# Vision 多模态骨架设计文档

> 更新时间：2026-05-04
>
> 在 lingma2api 代理层接入 OpenAI 多模态 `image_url` 与 Anthropic `image` content block 的入站解析、IR 表示、限额校验、日志记录与可配置错误/软兜底响应。
>
> 不在本骨架内：反向 Lingma 远端 `image_urls` 字段真实格式并真发图，将作为后续独立 plan 推进。

---

## 1. 背景

### 1.1 当前现状

1. `internal/proxy/types.go`：
   - `OpenAIChatRequest.Messages[].Content` 字段类型为 `string`，**完全不接受** OpenAI 多模态数组形式（`[{type:"text",...}, {type:"image_url",...}]`）。
   - IR 层已经定义 `CanonicalBlockImage` 与 `CanonicalBlockDocument`。
2. `internal/proxy/message_ir.go`：
   - **Anthropic 端入站已实现**：`convertAnthropicTurn` 中 `case "image"` 把 Anthropic content block 解析为 `CanonicalContentBlock{Type: CanonicalBlockImage}`。
   - **Anthropic 端出站已实现**：`projectCanonicalRequestForAnthropic` 把 `CanonicalBlockImage` 序列化回 Anthropic image block。
   - **OpenAI 端落库实现是软兜底**：`projectCanonicalRequestForOpenAI` 中 `case CanonicalBlockImage` 调用 `mediaBlockToText`，把图片信息压成 `[image: data:image/png;base64,...]` 字符串塞进 message content。
3. `internal/proxy/body.go`：
   - 第 81 行 `"image_urls": nil` 永远固定。也就是说不管 IR 中是否有图片块，发给 Lingma 远端的 `image_urls` 字段始终为 null。
4. `internal/db/settings.go`：
   - `defaultSettings` 已有 storage_mode/truncate_length/retention_days/polling_interval/theme/request_timeout 6 项，无 vision 相关项。

### 1.2 核心矛盾

当前实现等价于「Anthropic 端默认软兜底，OpenAI 端连入口都没开」。这与 Agent 工作流里「LLM 应该真正看到图，看不到就明确报错」的预期冲突，且消费端不知道图被吃成了文本。

### 1.3 决策依据

本设计基于 [grill-me 共识 (2026-05-04)](../../../../grill-me-vision-consensus.md)（如未导出，参见会话记录）：

| # | 决策项 | 取值 |
|---|---|---|
| 1 | 主线方向 | A. 代理层扩展 |
| 2 | 子方向 | A. 协议扩展 |
| 3 | 具体目标 | A. Vision 多模态 |
| 4 | 实施路径 | C. 先做代码骨架（反向独立后续） |
| 5 | 骨架范围 | B. 中等骨架（IR + 解析 + 限额 + 日志 + 明确错误） |
| 6 | 反向时机 | B. 骨架后顺序衔接 |
| 7 | 软兜底处理 | B. 默认错误 + Settings 开关可恢复 |

---

## 2. 目标

1. **OpenAI 端入站**支持多模态 `content` 数组形式，正确解析 `text` 和 `image_url` part。
2. **Anthropic 端入站**保持现有解析能力，无回归。
3. 入站后统一产出 `CanonicalContentBlock{Type: CanonicalBlockImage}` 携带元数据（`media_type`、`byte_size`、`source_type`、`index`）。
4. **校验层**对图片大小、张数、媒体类型做硬限制，超出限额时返回 400。
5. **运行时拦截**：当 Canonical 请求中含图片块且 `vision_fallback_enabled=false`（默认）时，直接返回 `501 Not Implemented` 风格的协议特定错误，不进入下游。
6. **软兜底兼容**：当 `vision_fallback_enabled=true` 时，复用现有 `mediaBlockToText` 把图片合并入文本，让旧消费端不被破坏。
7. **日志可见**：每个 image 块的元数据写入 `http_exchanges` / canonical record，前端 LogDetail 抽屉可看见。
8. **`body.go` 出站保持 `image_urls: nil`**，留 `// TODO(vision-reverse)` 注释指向反向任务。

## 3. 非目标（明确推迟）

1. 反向 Lingma 远端 `image_urls` 字段真实格式（独立后续 plan）。
2. 真实图片预览、缩略图、image hash 去重。
3. PDF / SVG / 视频 / 音频。
4. `/v1/models` 输出 `extra_capabilities` 字段。
5. 多模态模型映射表（哪些 Lingma 模型支持 vision）。
6. Policy 引擎扩展 `vision_handling` 多态规则（先用全局 settings）。
7. 调试聊天界面或新前端页面。
8. 完整 image base64 入库或缩略图渲染。

---

## 4. 设计输入与前置假设

1. `internal/proxy/types.go` 中 `Message.Content` 类型可以扩展（含 backwards-compat 的自定义 UnmarshalJSON）。
2. `internal/proxy/message_ir.go` 中 `CanonicalContentBlock` 已有 `Metadata map[string]any` 字段，可承载图片元数据。
3. `internal/db/settings.go` 中 `defaultSettings` 可新增键。
4. Anthropic 端现有 `imageToText` / `mediaBlockToText` 函数路径未来仍要复用，不删除。
5. Lingma 远端 `image_urls` 字段格式未知，骨架阶段保持 `nil`，反向任务负责回填。

---

## 5. 总体架构

```text
                          ┌──────────────────────────────────────────┐
   OpenAI multimodal ─────►│  Custom UnmarshalJSON                   │
   (string OR              │  → Message.Content (string fallback)    │
    [{type, text/image}])  │  → Message.Parts ([]OpenAIContentPart)  │
                           └──────────────┬───────────────────────────┘
                                          │
   Anthropic image block ─────►──────────┐│
                                         ▼▼
                        ┌──────────────────────────────────────┐
                        │  CanonicalizeOpenAIRequest          │
                        │  CanonicalizeAnthropicRequest       │
                        │  produce CanonicalContentBlock{     │
                        │    Type: CanonicalBlockImage,        │
                        │    Data: <ImageSource raw>,          │
                        │    Metadata: {media_type, byte_size, │
                        │               source_type, index}    │
                        │  }                                   │
                        └──────────────┬───────────────────────┘
                                       │
                              ┌────────▼────────┐
                              │ validateVision   │  (新增) limits + whitelist
                              │ Limits           │  超限 → 400 invalid_request
                              └────────┬────────┘
                                       │
                              ┌────────▼────────┐
                              │ visionGate       │  (新增) 检查是否含 image/document
                              │   - settings.    │
                              │     vision_fall  │
                              │     back_enabled │
                              └─┬───────┬───────┘
                                │       │
              fallback=false    │       │ fallback=true
                                │       │
                  ┌─────────────▼┐    ┌─▼────────────────┐
                  │ 501 vision_  │    │ 现有 mediaBlock   │
                  │ not_implem.  │    │ ToText 软兜底     │
                  │ (协议特定错误)│    │ 图 → [image: …]  │
                  └──────────────┘    │ 合并入 content    │
                                      └────┬─────────────┘
                                           │
                                ┌──────────▼──────────────┐
                                │ Existing body builder    │
                                │ image_urls: nil  // TODO │
                                │ (vision-reverse)         │
                                └──────────────────────────┘
```

---

## 6. 详细设计

### 6.1 OpenAI Message 多模态 Content 解析

#### 6.1.1 类型扩展

`internal/proxy/types.go`：

```go
// OpenAIContentPart represents a single part in a multimodal user/system message.
type OpenAIContentPart struct {
    Type     string                  `json:"type"`             // "text" | "image_url"
    Text     string                  `json:"text,omitempty"`
    ImageURL *OpenAIContentImageURL  `json:"image_url,omitempty"`
}

// OpenAIContentImageURL describes the image source. URL may be http(s) or a data URI.
type OpenAIContentImageURL struct {
    URL    string `json:"url"`
    Detail string `json:"detail,omitempty"` // "auto" | "low" | "high" - 仅记录，不参与限额
}

type Message struct {
    Role       string              `json:"role"`
    Content    string              `json:"content,omitempty"`
    Parts      []OpenAIContentPart `json:"-"`               // populated by UnmarshalJSON only
    Name       string              `json:"name,omitempty"`
    ToolCallID string              `json:"tool_call_id,omitempty"`
    ToolCalls  []ToolCall          `json:"tool_calls,omitempty"`
}
```

#### 6.1.2 自定义 UnmarshalJSON

新增 `Message.UnmarshalJSON`：

- 先用一个匿名结构 `aux` 抓住所有字段，把 `content` 暂存为 `json.RawMessage`。
- 复制非 content 字段到 `Message`。
- 判定 `aux.Content`：
  - 空（null/缺失） → `Message.Content = ""`，`Parts = nil`。
  - JSON string → 直接 `Unmarshal` 到 `Message.Content`，`Parts = nil`。
  - JSON array → `Unmarshal` 到 `[]OpenAIContentPart`，存进 `Message.Parts`；同时把所有 `Part.Text` 用 `\n` 拼接也写入 `Message.Content`（用于现有依赖 `Content` 的代码路径继续工作）。
- 其它形式 → 返回 error。

#### 6.1.3 Canonicalize 入站

`CanonicalizeOpenAIRequest` 中 `case "system", "user"`：

- 如果 `len(message.Parts) == 0`：保留现有 `CanonicalBlockText{Text: message.Content}` 路径。
- 否则遍历 `message.Parts`：
  - `type == "text"` → 追加 `CanonicalBlockText`。
  - `type == "image_url"` → 调用 `parseOpenAIImageURL(part.ImageURL.URL)` 拆分为 `(sourceType, mediaType, dataOrURL)`，构造 `ImageSource{Type, MediaType, Data}` 序列化为 `Data`，元数据存到 `Metadata`。

新增 helper `parseOpenAIImageURL(raw string) (ImageSource, error)`：

- `data:<media>;base64,<payload>` → `ImageSource{Type:"base64", MediaType:<media>, Data:<payload>}`。
- `http(s)://...` → `ImageSource{Type:"url", MediaType:"", Data:<URL>}`（media_type 由响应反推或留空，骨架阶段不去拉取，直接保留）。
- 其它形式 → error（用于校验拒绝）。

### 6.2 Anthropic Message 入站

无改动。现有 `case "image"` 已经把 `block.Source` 序列化到 `Data`。本骨架补充：构造 `Metadata`：

```go
turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
    Type: CanonicalBlockImage,
    Data: mustMarshalRaw(block.Source),
    Metadata: imageBlockMetadata(block.Source, len(turn.Blocks)),
})
```

`imageBlockMetadata(src *ImageSource, index int) map[string]any` 返回：

```go
map[string]any{
    "media_type":  src.MediaType,
    "source_type": src.Type,             // "base64" | "url"
    "byte_size":   approxByteSize(src),  // base64 长度估算原始字节数
    "index":       index,
}
```

### 6.3 限额校验

在 `validateChatRequest` 之外新增 `validateVisionLimits(req CanonicalRequest) error`，在 `CanonicalizeOpenAIRequest` / `CanonicalizeAnthropicRequest` 返回后立即调用。

校验项：

| 项 | 阈值 | 错误信息 |
|---|---|---|
| 单图字节大小 | 5 MB（5 * 1024 * 1024） | `image at index N exceeds 5 MB limit` |
| 单请求图片张数 | 4 | `image count exceeds 4 per request` |
| 单请求总字节大小 | 10 MB | `total image bytes exceed 10 MB per request` |
| 媒体类型白名单 | image/png, image/jpeg, image/gif, image/webp | `unsupported media type %q` |
| URL 形式 | http/https/data | `unsupported image url scheme` |
| data URI 解码失败 | base64 无效 | `invalid base64 image payload` |

校验失败 → server.go 返回 HTTP 400：

- OpenAI 端：`{"error":{"message":"...","type":"invalid_request_error","code":"invalid_image"}}`
- Anthropic 端：`{"type":"error","error":{"type":"invalid_request_error","message":"..."}}`

### 6.4 Vision Gate（错误 vs 软兜底）

在 `server.handleChatCompletions` 与 `server.handleAnthropicMessages` 中，`Canonicalize*Request` 之后、`validateChatRequest`（投影回 OpenAI Message 之后那次）之前，新增 `visionGate` 步骤。

`visionGate(ctx, store, canonical CanonicalRequest) (allowFallback bool, err error)`：

1. 若 canonical 中没有 `CanonicalBlockImage` / `CanonicalBlockDocument`，直接 `return false, nil`。
2. 调 `store.GetSettings(ctx)`，读 `vision_fallback_enabled`：
   - `"true"` → `return true, nil`，让现有 `mediaBlockToText` 软兜底跑。
   - `"false"`（默认）→ `return false, ErrVisionNotImplemented`。
3. 如果读 settings 失败，**安全地走 false**（保守，不允许软兜底意外开启）。

新增 sentinel：

```go
var ErrVisionNotImplemented = errors.New("vision_not_implemented")
```

server.go 错误响应：

- OpenAI 端：HTTP 501，body `{"error":{"message":"vision input is not implemented yet; set vision_fallback_enabled=true in settings to fall back to text representation","type":"not_implemented","code":"vision_not_implemented"}}`
- Anthropic 端：HTTP 501，body `{"type":"error","error":{"type":"not_supported_yet","message":"vision input is not implemented yet; set vision_fallback_enabled=true in settings to fall back to text representation"}}`

### 6.5 Body Builder（不动）

`internal/proxy/body.go` 中 `image_urls: nil` 保留，仅在该行上方加注释：

```go
// TODO(vision-reverse): Lingma 远端 image_urls 字段真实格式未反向。
// 骨架阶段保持 nil；反向任务参见
// docs/superpowers/specs/2026-05-XX-vision-image-urls-reverse.md
"image_urls": nil,
```

### 6.6 Settings 新增项

`internal/db/settings.go`：

```go
var defaultSettings = map[string]string{
    "storage_mode":            "full",
    "truncate_length":         "102400",
    "retention_days":          "30",
    "polling_interval":        "0",
    "theme":                   "light",
    "request_timeout":         "90",
    "vision_fallback_enabled": "false",
}
```

可通过现有 `PUT /admin/settings` 修改，无须新增 API。

### 6.7 日志元数据写入

`CanonicalContentBlock.Metadata` 已经在 `canonical_records` 中作为整个 `CanonicalRequest` 的一部分入库（现有逻辑），无需新增表。

`http_exchanges` 不直接知道图片块的存在，但 IR 序列化时图片元数据已经在 `Data` / `Metadata` 中，列表查询不会拉这些字段，详情抽屉打开时可读。

**截断策略**：base64 数据保留前 256 字节，后接 `...[image truncated, total: N bytes]`。新增 helper `truncateImageDataForLog(src ImageSource) ImageSource`，仅在持久化路径上调用。

### 6.8 前端 LogDetail 抽屉

`frontend/src/pages/LogDetail.tsx` 在 canonical view 中检测 block 类型为 `image` 时渲染：

```tsx
<div className="block-image-meta">
  <ImageIcon size={14} />
  <span>image · {block.metadata?.media_type ?? "unknown"} · {formatBytes(block.metadata?.byte_size)} · {block.metadata?.source_type}</span>
</div>
```

不渲染实际图像（base64 已被截断）。

---

## 7. 错误响应汇总

| 场景 | HTTP | OpenAI body | Anthropic body |
|---|---|---|---|
| 限额超出 / 媒体类型不允许 / URL scheme 错误 | 400 | `error.type=invalid_request_error, code=invalid_image` | `error.type=invalid_request_error` |
| 含图请求且 fallback 关闭 | 501 | `error.type=not_implemented, code=vision_not_implemented` | `error.type=not_supported_yet` |
| 含图请求且 fallback 开启（旧行为） | 200 / SSE | 正常聊天响应（图被压成文本） | 正常聊天响应（图被压成文本） |

---

## 8. 测试策略

### 8.1 单元测试

- `internal/proxy/types_test.go`（新增）：
  - `TestOpenAIMessage_UnmarshalString`
  - `TestOpenAIMessage_UnmarshalArray_TextOnly`
  - `TestOpenAIMessage_UnmarshalArray_ImageURL_HTTP`
  - `TestOpenAIMessage_UnmarshalArray_ImageURL_DataURI`
  - `TestOpenAIMessage_UnmarshalArray_Mixed`
  - `TestOpenAIMessage_UnmarshalNull`

- `internal/proxy/message_ir_test.go`（修改）：
  - `TestCanonicalizeOpenAIRequest_ImagePart_HTTP`
  - `TestCanonicalizeOpenAIRequest_ImagePart_DataURI`
  - `TestCanonicalizeOpenAIRequest_TextOnlyParts`
  - `TestCanonicalizeAnthropicRequest_ImageBlock_MetadataAttached`

- `internal/proxy/vision_limits_test.go`（新增）：
  - `TestValidateVisionLimits_OK`
  - `TestValidateVisionLimits_TooManyImages`
  - `TestValidateVisionLimits_SingleImageTooLarge`
  - `TestValidateVisionLimits_TotalBytesExceeded`
  - `TestValidateVisionLimits_InvalidMediaType`
  - `TestValidateVisionLimits_InvalidScheme`

- `internal/api/server_test.go`（修改）：
  - `TestHandleChatCompletions_VisionDefault_Returns501`
  - `TestHandleChatCompletions_VisionFallbackEnabled_Returns200`
  - `TestHandleAnthropicMessages_VisionDefault_Returns501`
  - `TestHandleAnthropicMessages_VisionFallbackEnabled_Returns200`
  - `TestHandleChatCompletions_VisionExceedsLimit_Returns400`

### 8.2 集成保护

- `body_test.go` 增补：`TestBuildChatBody_ImageUrlsAlwaysNil`，验证骨架不破坏现有 body 形态。

### 8.3 回归保护

- 所有现有测试用例必须继续通过。
- 新 settings 项加入 `defaultSettings`，必须在 `settings_test.go` 中确认 `GetSettings` 默认值含 `vision_fallback_enabled=false`。

---

## 9. 风险与回退

| 风险 | 缓解 |
|---|---|
| `Message.UnmarshalJSON` 影响所有现有 OpenAI 请求 | 在 string 形式上保持完全兼容，新增测试覆盖纯字符串路径 |
| Anthropic 端原本"装作能用"被改为 501，老消费端报错 | Settings `vision_fallback_enabled=true` 一键恢复旧行为 |
| `Metadata` 字段被序列化到 canonical record 增加体积 | 元数据本身体积小（< 200 字节/块），base64 已截断 |
| 限额不合理（5MB / 4 张过严） | 通过 settings 后续可调（如有需要再加 vision_max_size_mb / vision_max_count） |
| 反向 image_urls 失败导致骨架"永远只能 501" | 骨架本身已经是可独立交付的稳定状态，无副作用 |

---

## 10. 后续衔接

骨架合并后立即启动反向任务，独立 spec/plan：

- `docs/superpowers/specs/2026-05-XX-vision-image-urls-reverse.md`
- `docs/superpowers/plans/2026-05-XX-vision-image-urls-reverse.md`

反向任务输出后，仅需替换 `body.go:81` 的 `"image_urls": nil` 为真实序列化逻辑（在传入 IR 中遍历 `CanonicalBlockImage` 的 `Data`/`Metadata`，按反向出的格式生成），并把 `vision_fallback_enabled` 默认改为 `false` 即可声明 vision 真发功能 GA。
