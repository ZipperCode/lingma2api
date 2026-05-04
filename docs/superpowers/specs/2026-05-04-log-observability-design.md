# v1.3 日志可观测性增强设计文档

> 增强 lingma2api 的请求日志系统，完整记录上下游 HTTP 交换信息，以时间线视图在管理后台展示，方便排查上游问题。

---

## 背景与问题

当前系统通过 `canonical_execution_records` 记录请求日志，但存在以下问题：

1. **HTTP 层信息缺失**：没有记录上游 URL、Headers、真实 HTTP 状态码
2. **`upstream_status` 被硬编码为 200**：`canonical_runtime.go:113` 硬编码 status，无论上游实际返回什么都显示 200
3. **排查困难**：遇到上游错误（如 401、429、502）时，只能看到聚合后的错误消息，无法看到原始响应内容
4. **SSE 流不可追溯**：流式请求的原始 SSE 行虽然记录在 sidecar 中，但展示方式不友好
5. **列表页加载冗余数据**：当前列表 API 返回了完整的 downstream_req/upstream_req body，数据量大

## 目标

1. 完整记录每次 HTTP 交换的请求/响应详情（Headers、Body、URL、Status、耗时）
2. 修复 `upstream_status` 硬编码问题
3. 前端列表页轻量加载，详情通过右侧抽屉以时间线视图展示
4. SSE 流式响应支持"聚合结果"与"原始 SSE 行"双轨查看
5. 旧数据可丢弃，无需迁移

## 架构

采用**双表互补**模型：

- `canonical_execution_records`（已有）：负责语义层——pre/post policy、session snapshot、canonical 转换过程
- `http_exchanges`（新增）：负责传输层——HTTP 的完整请求/响应，按时间顺序记录每次交换

两表通过 `log_id`（即 canonical record 的 UUID）关联。

## 数据模型

### `http_exchanges` 表

```sql
CREATE TABLE http_exchanges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    log_id TEXT NOT NULL,              -- 关联 canonical_execution_records.id
    direction TEXT NOT NULL,           -- 'downstream' | 'upstream'
    phase TEXT NOT NULL,               -- 'request' | 'response'
    timestamp DATETIME NOT NULL,       -- 精确到毫秒
    method TEXT,                       -- HTTP 方法
    url TEXT,                          -- 完整 URL（仅上游请求）
    path TEXT,                         -- 请求路径（仅下游请求）
    status_code INTEGER,               -- HTTP 状态码（仅 response）
    headers TEXT,                      -- JSON 序列化的 headers
    body TEXT,                         -- body 内容（截断策略见后）
    duration_ms INTEGER,               -- 从对应 request 到本 response 的耗时
    error TEXT,                        -- 错误信息（连接失败、超时等）
    raw_stream TEXT,                   -- 原始 SSE 行（仅上游流式响应）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_http_exchanges_log_id ON http_exchanges(log_id);
CREATE INDEX idx_http_exchanges_timestamp ON http_exchanges(timestamp);
```

### `request_logs` 表（废弃，保留兼容）

旧表不再写入新数据，但保留读取兼容。生产代码中 `InsertLog` 已停止调用。

## API 设计

### `GET /admin/logs`

**默认模式（summary）**：
返回精简字段，不包含 body、headers 等大数据：
```json
{
  "items": [
    {
      "id": "msg_uuid",
      "created_at": "2026-05-04T14:32:01Z",
      "model": "gpt-4",
      "mapped_model": "dashscope_qwen_max",
      "stream": true,
      "status": "success",
      "upstream_status": 200,
      "prompt_tokens": 120,
      "completion_tokens": 450,
      "total_tokens": 570,
      "ttft_ms": 234,
      "upstream_ms": 3200,
      "downstream_ms": 15,
      "ingress_protocol": "anthropic",
      "ingress_endpoint": "/v1/messages",
      "canonical_record": true
    }
  ],
  "total": 128,
  "page": 1,
  "limit": 50
}
```

**全量模式**：`GET /admin/logs?detail=full`
用于导出等场景，返回包含完整 exchange 数据。

### `GET /admin/logs/:id`

返回完整详情 + http_exchanges 数组：
```json
{
  "id": "msg_uuid",
  "created_at": "...",
  "model": "gpt-4",
  "mapped_model": "...",
  "stream": true,
  "status": "success",
  "upstream_status": 200,
  "prompt_tokens": 120,
  "completion_tokens": 450,
  "total_tokens": 570,
  "ttft_ms": 234,
  "upstream_ms": 3200,
  "downstream_ms": 15,
  "ingress_protocol": "anthropic",
  "ingress_endpoint": "/v1/messages",
  "canonical_record": true,
  "pre_policy_request": "{...}",
  "post_policy_request": "{...}",
  "session_snapshot": "{...}",
  "execution_sidecar": "{...}",
  "exchanges": [
    {
      "id": 1,
      "log_id": "msg_uuid",
      "direction": "downstream",
      "phase": "request",
      "timestamp": "2026-05-04T14:32:01.234Z",
      "method": "POST",
      "path": "/v1/messages",
      "headers": "{...}",
      "body": "{...}"
    },
    {
      "id": 2,
      "log_id": "msg_uuid",
      "direction": "upstream",
      "phase": "request",
      "timestamp": "2026-05-04T14:32:01.245Z",
      "method": "POST",
      "url": "https://lingma.aliyuncs.com/...",
      "headers": "{...}",
      "body": "{...}"
    },
    {
      "id": 3,
      "log_id": "msg_uuid",
      "direction": "upstream",
      "phase": "response",
      "timestamp": "2026-05-04T14:32:04.567Z",
      "status_code": 200,
      "headers": "{...}",
      "body": "{...}",
      "duration_ms": 3322,
      "raw_stream": "data: {...}\n\ndata: {...}\n\n..."
    },
    {
      "id": 4,
      "log_id": "msg_uuid",
      "direction": "downstream",
      "phase": "response",
      "timestamp": "2026-05-04T14:32:04.582Z",
      "status_code": 200,
      "headers": "{...}",
      "body": "{...}",
      "duration_ms": 15
    }
  ]
}
```

## 前端设计

### 列表页 (`/logs`)

- 保持现有表格布局
- 每行可点击，点击后右侧滑出抽屉
- 不再跳转到 `/logs/:id` 独立页面（保留路由兼容，可直接访问）

### 详情抽屉

**布局：**
```
┌────────────────────────────────────────────┐
│  [X] 请求详情                        msg_xxx │
├────────────────────────────────────────────┤
│  元数据网格（时间、模型、状态、Token、耗时）   │
├────────────────────────────────────────────┤
│  请求链路时间线                              │
│  ── 14:32:01.234 ── 下游请求 POST /v1/msg   │
│     [Headers] [Body JSON]                   │
│  ── 14:32:01.245 ── 上游请求 POST aliyun... │
│     [Headers] [Body JSON]                   │
│  ── 14:32:04.567 ── 上游响应 200 ● 3322ms   │
│     [Headers] [Body JSON] [原始SSE]          │
│  ── 14:32:04.582 ── 下游响应 200 ● 15ms     │
│     [Headers] [Body JSON]                   │
├────────────────────────────────────────────┤
│  [Canonical Data] 折叠面板                   │
│   Pre-Policy / Post-Policy / Session        │
└────────────────────────────────────────────┘
```

**时间线卡片交互：**
- 每个交换默认展开显示 Headers + Body
- Headers 区域如果超过 5 行，显示前 5 行 + "展开全部"
- Body 自动 JSON 格式化高亮
- SSE 响应的 Body 默认展示聚合后的 JSON，提供 "查看原始 SSE 流" 切换按钮
- 错误响应（status >= 400 或 error 字段非空）卡片边框变红色

### 响应式

- 抽屉宽度：桌面端 800px，平板 100%，移动端全屏
- 时间线卡片在小屏下改为垂直堆叠

## 后端写入流程

### Handler 中的写入点

以 OpenAI `/v1/chat/completions` 为例（Anthropic `/v1/messages` 类似）：

```go
func (server *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
    traceID := proxy.NewUUID()
    startTime := server.deps.Now()

    // 1. 记录下游请求
    server.recordExchange(traceID, "downstream", "request", ExchangeRecord{
        Timestamp: startTime,
        Method:    r.Method,
        Path:      r.URL.Path,
        Headers:   headersToJSON(r.Header),
        Body:      string(body),
    })

    // ... 处理逻辑 ...

    // 2. 记录上游请求
    upstreamReqStart := server.deps.Now()
    server.recordExchange(traceID, "upstream", "request", ExchangeRecord{
        Timestamp: upstreamReqStart,
        Method:    "POST",
        URL:       remoteRequest.URL,
        Headers:   headersToJSON(remoteHeaders),
        Body:      remoteRequest.BodyJSON,
    })

    // 3. 发送请求到上游，接收响应
    stream, err := server.deps.Transport.StreamChat(ctx, remoteRequest, credential)
    if err != nil {
        // 记录上游连接失败
        server.recordExchange(traceID, "upstream", "response", ExchangeRecord{
            Timestamp: server.deps.Now(),
            Error:     err.Error(),
        })
        writeMappedError(w, err)
        return
    }

    // ... 处理响应 ...

    // 4. 记录上游响应（含 raw SSE）
    upstreamReqEnd := server.deps.Now()
    server.recordExchange(traceID, "upstream", "response", ExchangeRecord{
        Timestamp:  upstreamReqEnd,
        StatusCode: upstreamStatus,  // 真实状态码！
        Headers:    headersToJSON(upstreamRespHeaders),
        Body:       aggregatedBody,
        DurationMs: int(upstreamReqEnd.Sub(upstreamReqStart).Milliseconds()),
        RawStream:  strings.Join(rawSSELines, "\n"),
    })

    // ... 发送下游响应 ...

    // 5. 记录下游响应
    server.recordExchange(traceID, "downstream", "response", ExchangeRecord{
        Timestamp:  server.deps.Now(),
        StatusCode: http.StatusOK,
        Headers:    headersToJSON(w.Header()),
        Body:       downstreamBody,
        DurationMs: int(server.deps.Now().Sub(downstreamStart).Milliseconds()),
    })

    // 6. 写入 canonical record
    server.persistCanonicalExecutionRecord(
        ctx, traceID,  // 使用同一 traceID
        // ... 其他参数 ...
    )
}
```

### 关键修正：`upstream_status` 不再硬编码

`persistCanonicalExecutionRecord` 移除 `Sidecar.Metadata["upstream_status"]: 200`，改为：
- 列表查询时从 `http_exchanges` 中读取真实的 `status_code`
- `admin_logs_view.go` 的 `projectCanonicalRecordToLog` 中通过 JOIN 获取真实状态码

## Body 截断策略

为避免存储爆炸，对 body 字段实施截断：

| 场景 | 截断阈值 | 说明 |
|------|---------|------|
| downstream request | 2MB | 与现有 LimitReader 一致 |
| downstream response | 1MB | 通常较小 |
| upstream request | 2MB | 包含图片等大内容时可能很大 |
| upstream response (非流式) | 2MB | |
| upstream response (流式) | body 存聚合结果（通常 < 1MB），`raw_stream` 存原始 SSE 行（限制 10MB） | SSE 行数多但每行不大 |

截断时保留前 N 字节，末尾添加 `"\n...[truncated, total: X bytes]"` 标记。

## 错误处理

### 上游连接失败
- `http_exchanges` 记录：phase=response, direction=upstream, error="连接超时/拒绝/etc"
- 没有 status_code、headers、body

### 上游返回非 200
- `Transport.StreamChat` 返回 `UpstreamHTTPError`
- Handler 中记录上游响应：status_code = 实际值（401/429/502 等），body = `UpstreamHTTPError.Body`
- 然后返回下游错误响应

### SSE 解析失败
- 上游响应已记录（含 raw_stream）
- 解析错误在下游响应中体现（非流式返回错误，流式在 SSE error 事件中返回）

## 性能考虑

1. **异步写入**：HTTP 交换记录和 canonical record 的写入应在请求处理完成后异步进行，不影响响应延迟
2. **数据库索引**：`http_exchanges.log_id` 建立索引，详情查询 JOIN 性能可控
3. **列表查询**：不 JOIN http_exchanges，只查 canonical_execution_records 的摘要字段

## 测试策略

1. **单元测试**：`http_exchanges` 的 CRUD、截断逻辑、JOIN 查询
2. **集成测试**：完整请求链路验证 exchanges 是否正确写入
3. **前端测试**：抽屉打开/关闭、时间线渲染、SSE 切换

## 风险与回退

1. **存储增长**：http_exchanges 每条请求产生 4 条记录，存储量约 4-5 倍于旧模型。通过 body 截断和日志保留策略（Settings 中的 retention_days）控制。
2. **性能影响**：异步写入可消除响应延迟影响。如果数据库写入成为瓶颈，可引入批量写入或内存队列。
3. **旧数据兼容**：旧 `request_logs` 数据保留但不再写入。`/admin/logs/:id` 对旧数据的详情查询可返回空 exchanges 数组。
