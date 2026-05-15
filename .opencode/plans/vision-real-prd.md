# 实现真多模态 Vision

## Goal

移除软降级图片兜底机制，实现真正的多模态视觉支持。

## Requirements

1. 图片不再被降级为 base64 文本塞入 `content`
2. 上游请求体包含 `parts` 字段（对齐 Python 客户端）
3. Vision gate 不再拦截多模态请求
4. `vision_fallback_enabled` 默认值改为 `"false"`
5. 保留 `vision_limits.go` 校验

## Acceptance Criteria

- [ ] `projectCanonicalUserLikeTurn` / `projectCanonicalTurn` 中跳过 `CanonicalBlockImage`
- [ ] `BuildCanonical` 在 `image_urls` 非空时添加 `parts` 到最后一条 user 消息
- [ ] `evaluateVisionGate` 不再返回 `ErrVisionNotImplemented`
- [ ] `vision_fallback_enabled` 默认值改为 `"false"`
- [ ] `go build ./...` 和 `go test ./...` 通过

## 改动文件

| File | Change |
|------|--------|
| `internal/proxy/message_ir.go` | 跳过 `CanonicalBlockImage` 不转文本 |
| `internal/proxy/body.go` | 有 `image_urls` 时添加 `parts` |
| `internal/api/vision_gate.go` | 移除阻塞逻辑 |
| `internal/db/settings.go` | 默认值 `"false"` |
| 测试文件 | 同步更新 |
