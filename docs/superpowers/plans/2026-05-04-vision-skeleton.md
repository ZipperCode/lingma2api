# Vision 多模态骨架 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 lingma2api 在 OpenAI/Anthropic 两侧都能接受多模态图片输入，统一进入 IR，校验限额，并通过 Settings 开关在「默认 501 错误」与「软兜底成文本」之间切换；发往 Lingma 远端的 `image_urls` 字段保持 nil 直到反向任务交付。

**Architecture:** 7 类改动按 IR-外到内顺序展开：(1) OpenAI Message 类型扩展 + 自定义 UnmarshalJSON 接受多模态数组；(2) Canonicalize 入站把图片块连同元数据写入 IR；(3) Anthropic 入站补充元数据；(4) 限额校验函数；(5) Settings 新增 `vision_fallback_enabled`；(6) VisionGate 在两个 handler 中拦截；(7) Body 注释 + 日志截断 + 前端 LogDetail 显示。每步 TDD：失败用例 → 实现 → 通过 → 提交。

**Tech Stack:** Go 1.24 + 标准库 / React 18 + TypeScript + lucide-react / SQLite (modernc.org/sqlite)

**Spec:** `docs/superpowers/specs/2026-05-04-vision-skeleton-design.md`

---

## 文件结构

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/proxy/types.go` | 修改 | 新增 `OpenAIContentPart`/`OpenAIContentImageURL`，`Message` 增 `Parts` 字段 |
| `internal/proxy/types_test.go` | 创建 | `Message.UnmarshalJSON` 单测 |
| `internal/proxy/content_unmarshal.go` | 创建 | `Message.UnmarshalJSON` 实现 |
| `internal/proxy/message_ir.go` | 修改 | OpenAI 入站解析 Parts；Anthropic 入站补充 Metadata |
| `internal/proxy/message_ir_test.go` | 修改 | 新增 OpenAI 多模态 / Anthropic 元数据测试 |
| `internal/proxy/vision_limits.go` | 创建 | `ValidateVisionLimits` + `approxByteSize` + `parseOpenAIImageURL` |
| `internal/proxy/vision_limits_test.go` | 创建 | 限额校验单测 |
| `internal/db/settings.go` | 修改 | 加入 `vision_fallback_enabled` 默认项 |
| `internal/db/settings_test.go` | 修改 | 默认项断言 |
| `internal/api/vision_gate.go` | 创建 | `ErrVisionNotImplemented` + `evaluateVisionGate` 决策函数 |
| `internal/api/vision_gate_test.go` | 创建 | gate 决策单测 |
| `internal/api/server.go` | 修改 | OpenAI handler 接入 vision gate / 限额校验 / 错误响应 |
| `internal/api/anthropic_handler.go` | 修改 | Anthropic handler 接入 vision gate / 限额校验 / 错误响应 |
| `internal/api/server_test.go` | 修改 | OpenAI 端 e2e 测试 |
| `internal/api/anthropic_handler_test.go` | 修改/创建 | Anthropic 端 e2e 测试（如已存在则修改） |
| `internal/proxy/body.go` | 修改 | 在 `image_urls: nil` 上方加 TODO 注释 |
| `internal/proxy/body_test.go` | 修改 | 新增 `TestBuildChatBody_ImageUrlsAlwaysNil` |
| `frontend/src/pages/LogDetail.tsx` | 修改 | 渲染 image block 元数据 |
| `frontend/src/types/index.ts` | 修改 | `CanonicalContentBlock` 类型补 `metadata` 字段（如缺） |
| `README.md` | 修改 | 新增「Vision 输入」一节，说明骨架行为与开关 |

---

## Task 1: OpenAI Message 类型扩展

**Files:**
- Modify: `internal/proxy/types.go`

- [ ] **Step 1: 添加 `OpenAIContentPart` / `OpenAIContentImageURL` 类型**

打开 `internal/proxy/types.go`，在 `type Message struct` 上方插入：

```go
// OpenAIContentPart represents a single part in a multimodal user/system
// message. When the request body provides `content` as a JSON array, each
// element is unmarshalled into one of these.
type OpenAIContentPart struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	ImageURL *OpenAIContentImageURL `json:"image_url,omitempty"`
}

// OpenAIContentImageURL describes the image source. URL may be an http(s)
// URL or a `data:<media>;base64,<payload>` data URI.
type OpenAIContentImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}
```

- [ ] **Step 2: 给 `Message` 增加 `Parts` 字段**

把现有 `Message` 结构体替换为：

```go
type Message struct {
	Role       string              `json:"role"`
	Content    string              `json:"content,omitempty"`
	Parts      []OpenAIContentPart `json:"-"`
	Name       string              `json:"name,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall          `json:"tool_calls,omitempty"`
}
```

`Parts` 标记为 `json:"-"`，因为它仅由自定义 `UnmarshalJSON` 填充；序列化回 OpenAI body 时仍由 `Content` 字段承担，避免破坏 `body.go`。

- [ ] **Step 3: 编译检查**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go build ./...
```
Expected: 通过（仅是字段添加，未触发现有调用方）。

- [ ] **Step 4: Commit**

```bash
git add internal/proxy/types.go
git commit -m "feat(proxy): add OpenAIContentPart types and Message.Parts field"
```

---

## Task 2: `Message.UnmarshalJSON` —— 失败用例

**Files:**
- Create: `internal/proxy/types_test.go`

- [ ] **Step 1: 新建 `internal/proxy/types_test.go`**

```go
package proxy

import (
	"encoding/json"
	"testing"
)

func TestMessage_UnmarshalString(t *testing.T) {
	raw := `{"role":"user","content":"hello"}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Content != "hello" {
		t.Fatalf("Content = %q, want hello", m.Content)
	}
	if len(m.Parts) != 0 {
		t.Fatalf("Parts = %v, want empty", m.Parts)
	}
}

func TestMessage_UnmarshalArrayTextOnly(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"text","text":"hi"},{"type":"text","text":"there"}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Content != "hi\nthere" {
		t.Fatalf("Content = %q, want %q", m.Content, "hi\nthere")
	}
	if len(m.Parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(m.Parts))
	}
}

func TestMessage_UnmarshalArrayImageURLHTTP(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"text","text":"see"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Content != "see" {
		t.Fatalf("Content = %q, want see", m.Content)
	}
	if len(m.Parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(m.Parts))
	}
	if m.Parts[1].ImageURL == nil || m.Parts[1].ImageURL.URL != "https://example.com/a.png" {
		t.Fatalf("ImageURL not parsed: %#v", m.Parts[1].ImageURL)
	}
}

func TestMessage_UnmarshalArrayImageURLDataURI(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(m.Parts))
	}
	if m.Parts[0].ImageURL.URL != "data:image/png;base64,AAAA" {
		t.Fatalf("URL not preserved")
	}
}

func TestMessage_UnmarshalNullContent(t *testing.T) {
	raw := `{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"x","arguments":"{}"}}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Content != "" {
		t.Fatalf("Content = %q, want empty", m.Content)
	}
	if len(m.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(m.ToolCalls))
	}
}

func TestMessage_UnmarshalArrayInvalidType(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"unknown","text":"x"}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		t.Fatalf("expected error for unknown content part type")
	}
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestMessage_Unmarshal -v
```
Expected: 多个测试 FAIL（因为还没有自定义 UnmarshalJSON，数组形式会触发 json: cannot unmarshal array into Go struct field 错误）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/proxy/types_test.go
git commit -m "test(proxy): add failing tests for multimodal Message unmarshalling"
```

---

## Task 3: `Message.UnmarshalJSON` 实现

**Files:**
- Create: `internal/proxy/content_unmarshal.go`

- [ ] **Step 1: 实现自定义 UnmarshalJSON**

新建 `internal/proxy/content_unmarshal.go`：

```go
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// UnmarshalJSON accepts both legacy string form and OpenAI-style multimodal
// array form for the `content` field. Array form populates Parts and also
// concatenates text parts into Content for downstream code that still reads
// Content directly.
func (m *Message) UnmarshalJSON(data []byte) error {
	type messageAux struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		Name       string          `json:"name,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	}
	var aux messageAux
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Role = aux.Role
	m.Name = aux.Name
	m.ToolCallID = aux.ToolCallID
	m.ToolCalls = aux.ToolCalls
	m.Content = ""
	m.Parts = nil

	raw := bytes.TrimSpace(aux.Content)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("content as string: %w", err)
		}
		m.Content = s
		return nil
	}
	if raw[0] == '[' {
		var parts []OpenAIContentPart
		if err := json.Unmarshal(raw, &parts); err != nil {
			return fmt.Errorf("content as array: %w", err)
		}
		texts := make([]string, 0, len(parts))
		for index, part := range parts {
			switch part.Type {
			case "text":
				texts = append(texts, part.Text)
			case "image_url":
				if part.ImageURL == nil || part.ImageURL.URL == "" {
					return fmt.Errorf("content[%d]: image_url.url is required", index)
				}
			default:
				return fmt.Errorf("content[%d]: unsupported part type %q", index, part.Type)
			}
		}
		m.Parts = parts
		m.Content = strings.Join(texts, "\n")
		return nil
	}
	return fmt.Errorf("content must be string or array, got %s", string(raw[:1]))
}
```

- [ ] **Step 2: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestMessage_Unmarshal -v
```
Expected: 全部 PASS。

- [ ] **Step 3: 跑完整 proxy 测试，确认无回归**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -v
```
Expected: 现有所有用例继续 PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/proxy/content_unmarshal.go
git commit -m "feat(proxy): support OpenAI multimodal content array via Message.UnmarshalJSON"
```

---

## Task 4: 限额校验 + URL 解析 helper —— 失败用例

**Files:**
- Create: `internal/proxy/vision_limits_test.go`

- [ ] **Step 1: 新建 `internal/proxy/vision_limits_test.go`**

```go
package proxy

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func makeBase64(size int) string {
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = byte(i % 256)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func makeImageBlock(t *testing.T, sourceType, mediaType, data string) CanonicalContentBlock {
	t.Helper()
	src := ImageSource{Type: sourceType, MediaType: mediaType, Data: data}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return CanonicalContentBlock{
		Type: CanonicalBlockImage,
		Data: raw,
		Metadata: map[string]any{
			"media_type":  mediaType,
			"source_type": sourceType,
			"byte_size":   approxByteSize(src),
			"index":       0,
		},
	}
}

func TestParseOpenAIImageURL_DataURI(t *testing.T) {
	src, err := parseOpenAIImageURL("data:image/png;base64,QUFB")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if src.Type != "base64" || src.MediaType != "image/png" || src.Data != "QUFB" {
		t.Fatalf("unexpected: %+v", src)
	}
}

func TestParseOpenAIImageURL_HTTP(t *testing.T) {
	src, err := parseOpenAIImageURL("https://example.com/x.png")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if src.Type != "url" || src.Data != "https://example.com/x.png" {
		t.Fatalf("unexpected: %+v", src)
	}
}

func TestParseOpenAIImageURL_Invalid(t *testing.T) {
	if _, err := parseOpenAIImageURL("ftp://x"); err == nil {
		t.Fatalf("expected error for ftp scheme")
	}
	if _, err := parseOpenAIImageURL("data:image/png;base64,!!"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if _, err := parseOpenAIImageURL(""); err == nil {
		t.Fatalf("expected error for empty url")
	}
}

func TestValidateVisionLimits_OK(t *testing.T) {
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{
			makeImageBlock(t, "base64", "image/png", makeBase64(1024)),
		},
	}}}
	if err := ValidateVisionLimits(req); err != nil {
		t.Fatalf("ValidateVisionLimits: %v", err)
	}
}

func TestValidateVisionLimits_TooManyImages(t *testing.T) {
	blocks := make([]CanonicalContentBlock, 5)
	for i := range blocks {
		blocks[i] = makeImageBlock(t, "url", "image/png", "https://example.com/")
	}
	req := CanonicalRequest{Turns: []CanonicalTurn{{Blocks: blocks}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "image count") {
		t.Fatalf("expected too-many-images error, got %v", err)
	}
}

func TestValidateVisionLimits_SingleImageTooLarge(t *testing.T) {
	tooLarge := makeBase64(6 * 1024 * 1024)
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{makeImageBlock(t, "base64", "image/png", tooLarge)},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "exceeds 5 MB") {
		t.Fatalf("expected single-image limit error, got %v", err)
	}
}

func TestValidateVisionLimits_TotalBytesExceeded(t *testing.T) {
	big := makeBase64(4 * 1024 * 1024)
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{
			makeImageBlock(t, "base64", "image/png", big),
			makeImageBlock(t, "base64", "image/png", big),
			makeImageBlock(t, "base64", "image/png", big),
		},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "total image bytes") {
		t.Fatalf("expected total-bytes error, got %v", err)
	}
}

func TestValidateVisionLimits_InvalidMediaType(t *testing.T) {
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{makeImageBlock(t, "base64", "image/bmp", makeBase64(64))},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "unsupported media type") {
		t.Fatalf("expected media-type error, got %v", err)
	}
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run "TestParseOpenAIImageURL|TestValidateVisionLimits" -v
```
Expected: FAIL（`parseOpenAIImageURL` / `ValidateVisionLimits` / `approxByteSize` 都还未定义）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/proxy/vision_limits_test.go
git commit -m "test(proxy): add failing tests for vision limit validation"
```

---

## Task 5: 限额校验 + URL 解析 helper —— 实现

**Files:**
- Create: `internal/proxy/vision_limits.go`

- [ ] **Step 1: 新建 `internal/proxy/vision_limits.go`**

```go
package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Vision skeleton limits. Values intentionally conservative; tune via Settings
// once reverse-engineering of the upstream image_urls field is complete.
const (
	visionMaxImagesPerRequest = 4
	visionMaxImageBytes       = 5 * 1024 * 1024  // 5 MB
	visionMaxTotalImageBytes  = 10 * 1024 * 1024 // 10 MB
)

var visionAllowedMediaTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
}

// parseOpenAIImageURL turns an OpenAI image_url.url into the canonical
// ImageSource shape used in IR. Supports `https?://...` and
// `data:<media>;base64,<payload>`.
func parseOpenAIImageURL(raw string) (ImageSource, error) {
	if raw == "" {
		return ImageSource{}, fmt.Errorf("image_url.url must not be empty")
	}
	if strings.HasPrefix(raw, "data:") {
		comma := strings.Index(raw, ",")
		if comma < 0 {
			return ImageSource{}, fmt.Errorf("invalid data uri")
		}
		header := raw[len("data:"):comma]
		payload := raw[comma+1:]
		// header expected like "image/png;base64"
		semi := strings.Index(header, ";")
		if semi < 0 || header[semi+1:] != "base64" {
			return ImageSource{}, fmt.Errorf("data uri must use base64 encoding")
		}
		mediaType := header[:semi]
		if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
			return ImageSource{}, fmt.Errorf("invalid base64 image payload: %w", err)
		}
		return ImageSource{Type: "base64", MediaType: mediaType, Data: payload}, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ImageSource{}, fmt.Errorf("invalid image url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ImageSource{}, fmt.Errorf("unsupported image url scheme %q", parsed.Scheme)
	}
	return ImageSource{Type: "url", MediaType: "", Data: raw}, nil
}

// approxByteSize returns the approximate raw byte count for an image source.
// For base64 it inverts standard base64 padding; for url it returns the URL
// string length as a proxy (only used for log metadata, not for limit checks).
func approxByteSize(src ImageSource) int {
	if src.Type == "base64" {
		stripped := strings.TrimRight(src.Data, "=")
		return len(stripped) * 3 / 4
	}
	return len(src.Data)
}

// ValidateVisionLimits scans every block of every turn and enforces the
// vision skeleton limits. Returns the first violation as a user-facing error.
func ValidateVisionLimits(req CanonicalRequest) error {
	totalBytes := 0
	imageCount := 0
	for turnIdx, turn := range req.Turns {
		for blockIdx, block := range turn.Blocks {
			if block.Type != CanonicalBlockImage && block.Type != CanonicalBlockDocument {
				continue
			}
			imageCount++
			if imageCount > visionMaxImagesPerRequest {
				return fmt.Errorf("image count exceeds %d per request", visionMaxImagesPerRequest)
			}
			var src ImageSource
			if err := json.Unmarshal(block.Data, &src); err != nil {
				return fmt.Errorf("turn[%d].block[%d]: invalid image source: %w", turnIdx, blockIdx, err)
			}
			if src.Type == "base64" {
				if _, ok := visionAllowedMediaTypes[src.MediaType]; !ok {
					return fmt.Errorf("turn[%d].block[%d]: unsupported media type %q", turnIdx, blockIdx, src.MediaType)
				}
				size := approxByteSize(src)
				if size > visionMaxImageBytes {
					return fmt.Errorf("turn[%d].block[%d]: image exceeds 5 MB limit (%d bytes)", turnIdx, blockIdx, size)
				}
				totalBytes += size
			} else if src.Type == "url" {
				// http(s) URL: do not fetch in the skeleton stage. media_type
				// may be empty and is allowed; size budget not consumed.
				if src.Data == "" {
					return fmt.Errorf("turn[%d].block[%d]: empty image url", turnIdx, blockIdx)
				}
			} else {
				return fmt.Errorf("turn[%d].block[%d]: unsupported image source type %q", turnIdx, blockIdx, src.Type)
			}
			if totalBytes > visionMaxTotalImageBytes {
				return fmt.Errorf("total image bytes exceed 10 MB per request")
			}
		}
	}
	return nil
}
```

- [ ] **Step 2: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run "TestParseOpenAIImageURL|TestValidateVisionLimits" -v
```
Expected: 全部 PASS。

- [ ] **Step 3: Commit**

```bash
git add internal/proxy/vision_limits.go
git commit -m "feat(proxy): add ValidateVisionLimits and parseOpenAIImageURL helpers"
```

---

## Task 6: OpenAI Canonicalize 中接入图片解析 —— 失败用例

**Files:**
- Modify: `internal/proxy/message_ir_test.go`

- [ ] **Step 1: 在 `internal/proxy/message_ir_test.go` 末尾追加**

```go
func TestCanonicalizeOpenAIRequest_ImagePartHTTP(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "text", Text: "see this:"},
				{Type: "image_url", ImageURL: &OpenAIContentImageURL{URL: "https://example.com/x.png"}},
			},
			Content: "see this:",
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if len(canonical.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(canonical.Turns))
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != CanonicalBlockText || blocks[0].Text != "see this:" {
		t.Fatalf("first block: %+v", blocks[0])
	}
	if blocks[1].Type != CanonicalBlockImage {
		t.Fatalf("second block type = %v, want image", blocks[1].Type)
	}
	if blocks[1].Metadata["source_type"] != "url" {
		t.Fatalf("source_type = %v, want url", blocks[1].Metadata["source_type"])
	}
}

func TestCanonicalizeOpenAIRequest_ImagePartDataURI(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "image_url", ImageURL: &OpenAIContentImageURL{URL: "data:image/png;base64,QUFB"}},
			},
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 1 || blocks[0].Type != CanonicalBlockImage {
		t.Fatalf("blocks: %+v", blocks)
	}
	if blocks[0].Metadata["media_type"] != "image/png" {
		t.Fatalf("media_type = %v", blocks[0].Metadata["media_type"])
	}
	if blocks[0].Metadata["source_type"] != "base64" {
		t.Fatalf("source_type = %v", blocks[0].Metadata["source_type"])
	}
}

func TestCanonicalizeOpenAIRequest_TextOnlyParts(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "text", Text: "hi"},
				{Type: "text", Text: "again"},
			},
			Content: "hi\nagain",
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != CanonicalBlockText || blocks[0].Text != "hi" {
		t.Fatalf("block[0]: %+v", blocks[0])
	}
	if blocks[1].Type != CanonicalBlockText || blocks[1].Text != "again" {
		t.Fatalf("block[1]: %+v", blocks[1])
	}
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestCanonicalizeOpenAIRequest_ImagePart -v
```
Expected: FAIL（当前 `CanonicalizeOpenAIRequest` 完全忽略 `Parts`，只生成一个 text block）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/proxy/message_ir_test.go
git commit -m "test(proxy): add failing tests for OpenAI multimodal canonicalization"
```

---

## Task 7: OpenAI Canonicalize 中接入图片解析 —— 实现

**Files:**
- Modify: `internal/proxy/message_ir.go`

- [ ] **Step 1: 找到 `CanonicalizeOpenAIRequest` 函数**

它当前在 `internal/proxy/message_ir.go:86`。其中 `case "system", "user":` 分支只看 `message.Content`。

- [ ] **Step 2: 替换 `case "system", "user":` 分支**

把：

```go
case "system", "user":
    if message.Content != "" {
        turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
            Type: CanonicalBlockText,
            Text: message.Content,
        })
    }
```

替换为：

```go
case "system", "user":
    if len(message.Parts) > 0 {
        for index, part := range message.Parts {
            switch part.Type {
            case "text":
                if part.Text == "" {
                    continue
                }
                turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
                    Type: CanonicalBlockText,
                    Text: part.Text,
                })
            case "image_url":
                if part.ImageURL == nil {
                    return CanonicalRequest{}, fmt.Errorf("turn[%d].part[%d]: image_url is nil", len(turns), index)
                }
                src, err := parseOpenAIImageURL(part.ImageURL.URL)
                if err != nil {
                    return CanonicalRequest{}, fmt.Errorf("turn[%d].part[%d]: %w", len(turns), index, err)
                }
                rawSrc, err := json.Marshal(src)
                if err != nil {
                    return CanonicalRequest{}, fmt.Errorf("turn[%d].part[%d]: marshal source: %w", len(turns), index, err)
                }
                turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
                    Type:     CanonicalBlockImage,
                    Data:     rawSrc,
                    Metadata: imageBlockMetadata(src, index),
                })
            }
        }
    } else if message.Content != "" {
        turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
            Type: CanonicalBlockText,
            Text: message.Content,
        })
    }
```

- [ ] **Step 3: 在文件末尾添加 `imageBlockMetadata` helper**

```go
// imageBlockMetadata produces the metadata map attached to every image
// CanonicalContentBlock. It captures enough context for log views without
// embedding the raw payload.
func imageBlockMetadata(src ImageSource, index int) map[string]any {
	return map[string]any{
		"media_type":  src.MediaType,
		"source_type": src.Type,
		"byte_size":   approxByteSize(src),
		"index":       index,
	}
}
```

- [ ] **Step 4: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestCanonicalizeOpenAIRequest -v
```
Expected: PASS（包含新增的三个用例和现有用例）。

- [ ] **Step 5: 跑全 proxy 测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -v
```
Expected: 全部 PASS，无回归。

- [ ] **Step 6: Commit**

```bash
git add internal/proxy/message_ir.go
git commit -m "feat(proxy): canonicalize OpenAI multimodal parts into image blocks"
```

---

## Task 8: Anthropic Canonicalize 中补充 Metadata —— 失败用例

**Files:**
- Modify: `internal/proxy/message_ir_test.go`

- [ ] **Step 1: 在 `message_ir_test.go` 追加**

```go
func TestCanonicalizeAnthropicRequest_ImageBlockHasMetadata(t *testing.T) {
	req := AnthropicMessagesRequest{
		Model:     "claude-3-7-sonnet-20250219",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"text","text":"see"},
				{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"QUFB"}}
			]`),
		}},
	}
	canonical, err := CanonicalizeAnthropicRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if len(canonical.Turns) != 1 {
		t.Fatalf("turns = %d", len(canonical.Turns))
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[1].Type != CanonicalBlockImage {
		t.Fatalf("blocks[1].Type = %v", blocks[1].Type)
	}
	if blocks[1].Metadata == nil {
		t.Fatalf("blocks[1].Metadata is nil; want populated")
	}
	if blocks[1].Metadata["media_type"] != "image/jpeg" {
		t.Fatalf("media_type = %v", blocks[1].Metadata["media_type"])
	}
	if blocks[1].Metadata["source_type"] != "base64" {
		t.Fatalf("source_type = %v", blocks[1].Metadata["source_type"])
	}
}
```

确保 import 中已含 `"encoding/json"`（如果还没有，加上）。

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestCanonicalizeAnthropicRequest_ImageBlockHasMetadata -v
```
Expected: FAIL（`Metadata` 字段当前没被填充）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/proxy/message_ir_test.go
git commit -m "test(proxy): assert Anthropic image block carries metadata"
```

---

## Task 9: Anthropic Canonicalize 中补充 Metadata —— 实现

**Files:**
- Modify: `internal/proxy/message_ir.go`

- [ ] **Step 1: 定位 Anthropic 的 image case**

在 `internal/proxy/message_ir.go` 中找到 `case "image":`（位于 `convertAnthropicTurn` 中，行号约 312）：

```go
case "image":
    turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
        Type: CanonicalBlockImage,
        Data: mustMarshalRaw(block.Source),
    })
```

- [ ] **Step 2: 替换为带 metadata 的版本**

```go
case "image":
    src := ImageSource{}
    if block.Source != nil {
        src = *block.Source
    }
    turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
        Type:     CanonicalBlockImage,
        Data:     mustMarshalRaw(block.Source),
        Metadata: imageBlockMetadata(src, len(turn.Blocks)),
    })
case "document":
    src := ImageSource{}
    if block.Source != nil {
        src = *block.Source
    }
    turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
        Type:     CanonicalBlockDocument,
        Data:     mustMarshalRaw(block.Source),
        Metadata: imageBlockMetadata(src, len(turn.Blocks)),
    })
```

注意：原本的 `case "document":` 也对应替换，使其同样携带 metadata。

- [ ] **Step 3: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestCanonicalizeAnthropicRequest_ImageBlockHasMetadata -v
```
Expected: PASS。

- [ ] **Step 4: 跑全 proxy 测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -v
```
Expected: 全部 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/message_ir.go
git commit -m "feat(proxy): attach metadata to Anthropic image/document IR blocks"
```

---

## Task 10: Settings 新增 `vision_fallback_enabled` —— 失败用例

**Files:**
- Create: `internal/db/settings_test.go`

`internal/db/settings_test.go` 不存在，需要新建。`Store.Migrate()` 不接受 context（参考 `store.go:32`）。`Open` 接受路径，传 `:memory:` 用 SQLite 内存数据库。

- [ ] **Step 1: 新建 `internal/db/settings_test.go`**

```go
package db

import (
	"context"
	"testing"
)

func newSettingsTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestGetSettings_DefaultsIncludeVisionFallback(t *testing.T) {
	store := newSettingsTestStore(t)

	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	got, ok := settings["vision_fallback_enabled"]
	if !ok {
		t.Fatalf("vision_fallback_enabled missing; defaults: %v", settings)
	}
	if got != "false" {
		t.Fatalf("vision_fallback_enabled = %q, want %q", got, "false")
	}
}

func TestUpdateSettings_AcceptsVisionFallback(t *testing.T) {
	store := newSettingsTestStore(t)

	if err := store.UpdateSettings(context.Background(), map[string]string{"vision_fallback_enabled": "true"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings["vision_fallback_enabled"] != "true" {
		t.Fatalf("vision_fallback_enabled = %q, want true", settings["vision_fallback_enabled"])
	}
}

func TestUpdateSettings_RejectsUnknownKey(t *testing.T) {
	store := newSettingsTestStore(t)

	err := store.UpdateSettings(context.Background(), map[string]string{"vision_unknown_key": "x"})
	if err == nil {
		t.Fatalf("expected error for unknown key")
	}
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/db -run "TestGetSettings_DefaultsIncludeVisionFallback|TestUpdateSettings_AcceptsVisionFallback" -v
```
Expected: FAIL（默认项还没加）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/db/settings_test.go
git commit -m "test(db): assert vision_fallback_enabled exists and is updatable"
```

---

## Task 11: Settings 新增 `vision_fallback_enabled` —— 实现

**Files:**
- Modify: `internal/db/settings.go`

- [ ] **Step 1: 添加默认项**

打开 `internal/db/settings.go`，把 `defaultSettings` map 改为：

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

- [ ] **Step 2: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/db -run "TestGetSettings_DefaultsIncludeVisionFallback|TestUpdateSettings_AcceptsVisionFallback" -v
```
Expected: PASS。

- [ ] **Step 3: 跑全 db 测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/db -v
```
Expected: 全部 PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/db/settings.go
git commit -m "feat(db): add vision_fallback_enabled default setting"
```

---

## Task 12: Vision Gate —— 失败用例

**Files:**
- Create: `internal/api/vision_gate_test.go`

- [ ] **Step 1: 新建 `internal/api/vision_gate_test.go`**

```go
package api

import (
	"context"
	"errors"
	"testing"

	"lingma2api/internal/proxy"
)

type fakeSettingsStore struct {
	settings map[string]string
	err      error
}

func (f *fakeSettingsStore) GetSettings(ctx context.Context) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]string, len(f.settings))
	for k, v := range f.settings {
		out[k] = v
	}
	return out, nil
}

func TestEvaluateVisionGate_NoImage(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockText, Text: "hello"},
	}}}}
	allow, err := evaluateVisionGate(context.Background(), store, req)
	if err != nil {
		t.Fatalf("evaluateVisionGate: %v", err)
	}
	if allow {
		t.Fatalf("allowFallback = true, want false (no image)")
	}
}

func TestEvaluateVisionGate_ImageDefaultDenies(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented", err)
	}
}

func TestEvaluateVisionGate_ImageFallbackAllows(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "true"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	allow, err := evaluateVisionGate(context.Background(), store, req)
	if err != nil {
		t.Fatalf("evaluateVisionGate: %v", err)
	}
	if !allow {
		t.Fatalf("allowFallback = false, want true")
	}
}

func TestEvaluateVisionGate_StoreErrorIsConservative(t *testing.T) {
	store := &fakeSettingsStore{err: errors.New("db down")}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented (conservative path)", err)
	}
}

func TestEvaluateVisionGate_DocumentTreatedAsVision(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockDocument},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented for document block", err)
	}
}
```

- [ ] **Step 2: 跑测试，确认失败**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -run TestEvaluateVisionGate -v
```
Expected: FAIL（`evaluateVisionGate` / `ErrVisionNotImplemented` 还未定义；`SettingsStore` 接口若不存在也会报错）。

- [ ] **Step 3: 提交失败用例**

```bash
git add internal/api/vision_gate_test.go
git commit -m "test(api): add failing tests for vision gate evaluation"
```

---

## Task 13: Vision Gate —— 实现

**Files:**
- Create: `internal/api/vision_gate.go`

- [ ] **Step 1: 新建 `internal/api/vision_gate.go`**

```go
package api

import (
	"context"
	"errors"

	"lingma2api/internal/proxy"
)

// ErrVisionNotImplemented is returned by evaluateVisionGate when a request
// contains image/document blocks and the vision_fallback_enabled setting is
// false (the default).
var ErrVisionNotImplemented = errors.New("vision_not_implemented")

// SettingsStore is the minimal subset of *db.Store required by the gate.
// Defined here so tests can inject a fake without depending on the full DB.
type SettingsStore interface {
	GetSettings(ctx context.Context) (map[string]string, error)
}

// evaluateVisionGate decides whether a canonical request that contains
// image/document blocks should proceed via the soft-fallback path or be
// rejected with ErrVisionNotImplemented.
//
// Returns:
//   - (false, nil) when the request has no vision content; caller proceeds normally.
//   - (true, nil)  when fallback is enabled; caller proceeds and the existing
//     mediaBlockToText projection compresses images into text.
//   - (false, ErrVisionNotImplemented) when fallback is disabled OR when the
//     settings store fails (conservative).
func evaluateVisionGate(ctx context.Context, store SettingsStore, req proxy.CanonicalRequest) (bool, error) {
	if !canonicalRequestHasVisionContent(req) {
		return false, nil
	}
	if store == nil {
		return false, ErrVisionNotImplemented
	}
	settings, err := store.GetSettings(ctx)
	if err != nil {
		return false, ErrVisionNotImplemented
	}
	if settings["vision_fallback_enabled"] == "true" {
		return true, nil
	}
	return false, ErrVisionNotImplemented
}

func canonicalRequestHasVisionContent(req proxy.CanonicalRequest) bool {
	for _, turn := range req.Turns {
		for _, block := range turn.Blocks {
			if block.Type == proxy.CanonicalBlockImage || block.Type == proxy.CanonicalBlockDocument {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 2: 跑测试，确认通过**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -run TestEvaluateVisionGate -v
```
Expected: 全部 PASS。

- [ ] **Step 3: Commit**

```bash
git add internal/api/vision_gate.go
git commit -m "feat(api): add evaluateVisionGate decision function"
```

---

## Task 14: OpenAI handler 接入 vision gate / 限额校验

**Files:**
- Modify: `internal/api/server.go`

- [ ] **Step 1: 找到 `handleChatCompletions` 中 `Canonicalize` 之后的位置**

`internal/api/server.go` `handleChatCompletions` 中第一次调用 `proxy.CanonicalizeOpenAIRequest` 的下方（约第 241 行）紧接 `attachCanonicalRequestMetadata` 行之后。

- [ ] **Step 2: 在 `attachCanonicalRequestMetadata` 之后插入 limits + gate**

```go
attachCanonicalRequestMetadata(&canonicalRequest, request.Header)

if err := proxy.ValidateVisionLimits(canonicalRequest); err != nil {
    writeOpenAIInvalidImage(writer, err.Error())
    return
}
fallbackAllowed, err := evaluateVisionGate(request.Context(), server.db, canonicalRequest)
if err != nil {
    if errors.Is(err, ErrVisionNotImplemented) {
        writeOpenAIVisionNotImplemented(writer)
        return
    }
    writeMappedError(writer, err)
    return
}
_ = fallbackAllowed
```

`fallbackAllowed` 在骨架阶段不影响后续路径（软兜底走的是已有 `mediaBlockToText` 投影），但保留赋值以便后续 plan 接入更精细控制。

- [ ] **Step 3: 在 `server.go` 末尾增加两个 helper**

```go
func writeOpenAIVisionNotImplemented(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusNotImplemented)
	body := map[string]any{
		"error": map[string]any{
			"message": "vision input is not implemented yet; set vision_fallback_enabled=true in settings to fall back to text representation",
			"type":    "not_implemented",
			"code":    "vision_not_implemented",
		},
	}
	_ = json.NewEncoder(writer).Encode(body)
}

func writeOpenAIInvalidImage(writer http.ResponseWriter, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusBadRequest)
	body := map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    "invalid_image",
		},
	}
	_ = json.NewEncoder(writer).Encode(body)
}
```

确认文件已 import `"errors"`、`"encoding/json"`、`"net/http"`，否则补全。

- [ ] **Step 4: 编译检查**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go build ./...
```
Expected: 通过。

- [ ] **Step 5: Commit**

```bash
git add internal/api/server.go
git commit -m "feat(api): wire vision gate and limits into OpenAI handler"
```

---

## Task 15: Anthropic handler 接入 vision gate / 限额校验

**Files:**
- Modify: `internal/api/anthropic_handler.go`

- [ ] **Step 1: 找到 `handleAnthropicMessages` 中 `Canonicalize` 之后的位置**

`internal/api/anthropic_handler.go` 中 `proxy.CanonicalizeAnthropicRequest` 调用下方紧接 `attachCanonicalRequestMetadata` 行之后。

- [ ] **Step 2: 插入 limits + gate**

在 `attachCanonicalRequestMetadata(&canonicalRequest, request.Header)` 之后追加：

```go
if err := proxy.ValidateVisionLimits(canonicalRequest); err != nil {
    writeAnthropicInvalidImage(writer, err.Error())
    return
}
if _, err := evaluateVisionGate(request.Context(), server.db, canonicalRequest); err != nil {
    if errors.Is(err, ErrVisionNotImplemented) {
        writeAnthropicVisionNotImplemented(writer)
        return
    }
    writeAnthropicError(writer, http.StatusInternalServerError, err.Error())
    return
}
```

- [ ] **Step 3: 在 `anthropic_handler.go` 末尾增加 helper**

```go
func writeAnthropicVisionNotImplemented(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusNotImplemented)
	body := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "not_supported_yet",
			"message": "vision input is not implemented yet; set vision_fallback_enabled=true in settings to fall back to text representation",
		},
	}
	_ = json.NewEncoder(writer).Encode(body)
}

func writeAnthropicInvalidImage(writer http.ResponseWriter, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusBadRequest)
	body := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": message,
		},
	}
	_ = json.NewEncoder(writer).Encode(body)
}
```

确认 import 含 `"errors"`、`"encoding/json"`、`"net/http"`。

- [ ] **Step 4: 编译检查**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go build ./...
```
Expected: 通过。

- [ ] **Step 5: Commit**

```bash
git add internal/api/anthropic_handler.go
git commit -m "feat(api): wire vision gate and limits into Anthropic handler"
```

---

## Task 16: e2e 测试 —— OpenAI 端 501 与 fallback

**Files:**
- Modify: `internal/api/server_test.go`

`server_test.go` 现有测试模式：直接 `NewServer(Dependencies{...}, store)` 拿到 `http.Handler`，配合 `httptest.NewRequest` + `httptest.NewRecorder` 手动 ServeHTTP。**不使用** `httptest.NewServer`。新测试沿用此模式，但需要传一个真实的内存 db.Store（之前测试都传 nil）以便 vision gate 能读 settings。

- [ ] **Step 1: 在 `server_test.go` 顶部 import 区追加**

如果还没有，import 中加上：

```go
import (
    // ... existing
    "lingma2api/internal/db"
    "fmt"
)
```

- [ ] **Step 2: 在文件末尾追加新 test setup helper**

```go
func newVisionTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newVisionHandler(t *testing.T, store *db.Store) http.Handler {
	t.Helper()
	return NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      fakeModels{},
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: fakeBuilder{},
		Now:     func() time.Time { return time.Unix(1, 0) },
	}, store)
}
```

- [ ] **Step 3: 追加三个 vision 测试**

```go
func TestChatCompletionsVisionDefaultReturns501(t *testing.T) {
	store := newVisionTestStore(t)
	handler := newVisionHandler(t, store)

	body := `{"model":"auto","stream":false,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "vision_not_implemented") {
		t.Fatalf("body = %s, want vision_not_implemented", recorder.Body.String())
	}
}

func TestChatCompletionsVisionFallbackEnabledBypassesGate(t *testing.T) {
	store := newVisionTestStore(t)
	if err := store.UpdateSettings(context.Background(), map[string]string{"vision_fallback_enabled": "true"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	handler := newVisionHandler(t, store)

	body := `{"model":"auto","stream":false,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusNotImplemented {
		t.Fatalf("status = 501; expected gate to be bypassed when fallback enabled. body=%s", recorder.Body.String())
	}
	// fallback 开启后走 fakeTransport，应该返回 200
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestChatCompletionsVisionExceedsLimitReturns400(t *testing.T) {
	store := newVisionTestStore(t)
	handler := newVisionHandler(t, store)

	// 7 MB raw payload → > 5 MB approx after base64 decode
	bigPayload := strings.Repeat("A", 7*1024*1024)
	body := fmt.Sprintf(`{"model":"auto","stream":false,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,%s"}}]}]}`, bigPayload)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "invalid_image") {
		t.Fatalf("body = %s, want invalid_image", recorder.Body.String())
	}
}
```

注意：现有的多个 `TestChatCompletions*` 测试目前传 `nil` 作为第二个 store 参数；新 helper `newVisionHandler` 传真实 store。两组测试共存即可，`vision gate` 在没有图片输入时不会触发对 store 的读取。

- [ ] **Step 4: 跑测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -run TestChatCompletionsVision -v
```
Expected: 三个用例全部 PASS。

- [ ] **Step 5: 跑全 api 测试，确认无回归**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -v
```
Expected: 全部 PASS（含原有 `TestChatCompletions*` 系列）。

- [ ] **Step 6: Commit**

```bash
git add internal/api/server_test.go
git commit -m "test(api): cover OpenAI vision 501/400/fallback paths"
```

---

## Task 17: e2e 测试 —— Anthropic 端 501 与 fallback

**Files:**
- Modify or Create: `internal/api/anthropic_handler_test.go`

- [ ] **Step 1: 检查现有文件**

```bash
ls /home/zipper/Projects/lingma2api/internal/api/anthropic_handler_test.go 2>/dev/null || echo "not present"
```

无论是否存在，把以下用例追加进去（如不存在则需要带上 package + import 头）。

- [ ] **Step 2: 写测试用例**

如果 `anthropic_handler_test.go` 不存在，新建并先写头部：

```go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lingma2api/internal/db"
)
```

然后追加用例（复用 `server_test.go` 中的 `newVisionTestStore` / `newVisionHandler`，因为同一 package 内可见）：

```go
func TestAnthropicMessagesVisionDefaultReturns501(t *testing.T) {
	store := newVisionTestStore(t)
	handler := newVisionHandler(t, store)

	body := `{"model":"claude-3-7-sonnet-20250219","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"see"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "not_supported_yet") {
		t.Fatalf("body = %s, want not_supported_yet", recorder.Body.String())
	}
}

func TestAnthropicMessagesVisionFallbackEnabledBypassesGate(t *testing.T) {
	store := newVisionTestStore(t)
	if err := store.UpdateSettings(context.Background(), map[string]string{"vision_fallback_enabled": "true"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	handler := newVisionHandler(t, store)

	body := `{"model":"claude-3-7-sonnet-20250219","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusNotImplemented {
		t.Fatalf("status = 501; expected fallback bypass. body=%s", recorder.Body.String())
	}
}
```

`time` / `db` import 实际在文件头声明，但 `newVisionHandler` 已经引用了它们（位于 `server_test.go`），同 package 共享。如果这里的新文件 lint 报告未使用的 import，删掉即可。

- [ ] **Step 3: 跑测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -run TestAnthropicMessagesVision -v
```
Expected: PASS。

- [ ] **Step 4: 跑全 api 测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/api -v
```
Expected: 现有所有用例继续 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/api/anthropic_handler_test.go
git commit -m "test(api): cover Anthropic vision 501 and fallback paths"
```

---

## Task 18: Body 注释 + body_test 守护

**Files:**
- Modify: `internal/proxy/body.go`
- Modify: `internal/proxy/body_test.go`

- [ ] **Step 1: 在 `body.go:81` 上方加注释**

打开 `internal/proxy/body.go`，把：

```go
		"image_urls":       nil,
```

替换为：

```go
		// TODO(vision-reverse): Lingma 远端 image_urls 字段真实格式未反向。
		// 骨架阶段保持 nil；反向任务参见
		// docs/superpowers/specs/2026-05-XX-vision-image-urls-reverse.md
		"image_urls":       nil,
```

- [ ] **Step 2: 在 `body_test.go` 末尾追加守护测试**

`internal/proxy/body.go` 中真实入口是 `(*BodyBuilder).BuildCanonical(request CanonicalRequest, modelKey string) (RemoteChatRequest, error)`（见 `body.go:37`）。返回的 `RemoteChatRequest.BodyJSON` 是 `[]byte`。测试结构：

```go
func TestBuildChatBody_ImageUrlsAlwaysNil(t *testing.T) {
	builder := NewBodyBuilder("2.11.2", func() time.Time { return time.Unix(1, 0) }, func() string { return "uuid-1" }, func() string { return "hex-1" })
	canonical := CanonicalRequest{
		Model:  "auto",
		Stream: false,
		Turns: []CanonicalTurn{{
			Role: "user",
			Blocks: []CanonicalContentBlock{
				{Type: CanonicalBlockText, Text: "hi"},
			},
		}},
	}
	remote, err := builder.BuildCanonical(canonical, "qwen-plus")
	if err != nil {
		t.Fatalf("BuildCanonical: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(remote.BodyJSON, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed["image_urls"] != nil {
		t.Fatalf("image_urls = %v, want nil (skeleton stage)", parsed["image_urls"])
	}
}
```

import 中需要 `"encoding/json"`、`"testing"`、`"time"`。如果文件已 import 了它们则不必重复。如果某个 helper 名（例如 `NewBodyBuilder` 实参顺序）与 `body.go` 当前签名不一致，按 `head -50 internal/proxy/body.go` 显示的真实签名调整。

- [ ] **Step 3: 跑测试**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./internal/proxy -run TestBuildChatBody_ImageUrlsAlwaysNil -v
```
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add internal/proxy/body.go internal/proxy/body_test.go
git commit -m "chore(proxy): mark image_urls as vision-reverse TODO and add guard test"
```

---

## Task 19: 前端 LogDetail 显示图片块摘要

**Files:**
- Modify: `frontend/src/pages/LogDetail.tsx`
- Modify: `frontend/src/styles/global.css`

`LogDetail.tsx` 当前把 canonical request 作为 raw JSON 字符串通过 `CodeViewer` 直接渲染（参见 `CANONICAL_TABS` 部分）。元数据已经在 JSON 中天然可见，本任务只新增一个**摘要 Banner**：在 canonical tab 内容上方显示「含 N 张图片，总 X KB」的快速概览，便于排查。

不新增 `CanonicalContentBlock` 类型；改为在 `LogDetail.tsx` 内做 best-effort JSON 解析。

- [ ] **Step 1: 在 `LogDetail.tsx` import 区追加**

```tsx
import { ImageIcon } from 'lucide-react';
```

- [ ] **Step 2: 在 `LogDetail` 组件内部，`return` 上方新增 helper**

```tsx
function summarizeVisionBlocks(rawCanonical: string): { count: number; totalBytes: number } | null {
  if (!rawCanonical) return null;
  try {
    const parsed = JSON.parse(rawCanonical) as { turns?: Array<{ blocks?: Array<{ type?: string; metadata?: Record<string, unknown> }> }> };
    let count = 0;
    let totalBytes = 0;
    for (const turn of parsed.turns ?? []) {
      for (const block of turn.blocks ?? []) {
        if (block.type === 'image' || block.type === 'document') {
          count += 1;
          const size = Number(block.metadata?.byte_size ?? 0);
          if (Number.isFinite(size)) totalBytes += size;
        }
      }
    }
    if (count === 0) return null;
    return { count, totalBytes };
  } catch {
    return null;
  }
}

function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i += 1;
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[i]}`;
}
```

- [ ] **Step 3: 在 canonical tab 内容渲染处插入摘要 Banner**

找到 `CodeViewer` 渲染 `tabValue` 的位置（紧接 `<div className="tabs">` 区块之后）。在它前面追加：

```tsx
{(['pre_policy_request', 'post_policy_request', 'session_snapshot'] as const).includes(tab as 'pre_policy_request' | 'post_policy_request' | 'session_snapshot') && (() => {
  const summary = summarizeVisionBlocks(tabValue);
  if (!summary) return null;
  return (
    <div className="vision-summary">
      <ImageIcon size={14} />
      <span>含 {summary.count} 张图片 · 总 {formatBytes(summary.totalBytes)}</span>
    </div>
  );
})()}
```

注意：`tab` 变量在文件顶部已定义（参考现有 `tabs.find(...)` 等用法）。如果当前文件用的是 `state.tab` 等不同命名，按现有写法对齐。

- [ ] **Step 4: 在 `frontend/src/styles/global.css` 末尾追加样式**

```css
.vision-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  margin: 8px 0;
  border-radius: 6px;
  background: rgba(89, 169, 255, 0.08);
  color: var(--text-secondary, #888);
  font-size: 12px;
}
```

- [ ] **Step 5: 启动开发模式验证**

Run:
```bash
cd /home/zipper/Projects/lingma2api && ./dev.sh
```

打开 http://127.0.0.1:3000，先进 `/admin/settings` 把 `vision_fallback_enabled` 改为 `true`，然后通过 curl 触发一个含图请求：

```bash
curl -s -X POST http://127.0.0.1:3000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"auto","stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"see"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUFB"}}]}]}' >/dev/null
```

回到 `/admin/logs`，打开最新一条记录，切到 `Pre-Policy Canonical` tab，确认顶部出现「含 1 张图片」摘要。

按 `Ctrl+C` 停开发服务。

- [ ] **Step 6: 构建前端**

Run:
```bash
cd /home/zipper/Projects/lingma2api/frontend && npm run build
cd ..
```

确认 `frontend-dist/` 有更新。

- [ ] **Step 7: Commit**

```bash
git add frontend/src/pages/LogDetail.tsx frontend/src/styles/global.css frontend-dist/
git commit -m "feat(frontend): show vision block summary on canonical log tabs"
```

---

## Task 20: README 同步

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在 README.md 「当前能力」一节末尾追加 Vision 子节**

在「当前能力」列表项之后追加：

```markdown
### Vision 输入（骨架）

- 接受 OpenAI `image_url` 与 Anthropic `image` content block 的入站解析与限额校验。
- **默认行为**：含图请求返回 `501 vision_not_implemented`（OpenAI）/ `not_supported_yet`（Anthropic）。
- **软兜底**：在 `/admin/settings` 中设置 `vision_fallback_enabled=true`，含图请求会把图片元数据合并入文本继续处理（模型不会真正看见图）。
- **限额**：单图 5 MB / 单请求 4 张 / 单请求总 10 MB；白名单 `image/png|jpeg|gif|webp`。
- **真发图能力**：尚未实现，待后续反向 Lingma 远端 `image_urls` 字段格式后接线。
```

- [ ] **Step 2: 在 README.md 「限制」一节追加项**

```markdown
- Vision：当前仅有协议骨架。真发图（`image_urls` 字段）需等待反向任务交付。
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(readme): document vision skeleton behavior and limits"
```

---

## Task 21: 全量回归

**Files:**
- 无新增

- [ ] **Step 1: 全单测**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go test ./... -v
```
Expected: ALL PASS。如有失败，记录失败用例并修复（不要绕过测试）。

- [ ] **Step 2: `go vet`**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go vet ./...
```
Expected: 无输出。

- [ ] **Step 3: 编译二进制**

Run:
```bash
cd /home/zipper/Projects/lingma2api && go build -o /tmp/lingma2api-vision-skeleton .
```
Expected: 编译成功。

- [ ] **Step 4: 启动服务并冒烟**

Run:
```bash
cd /home/zipper/Projects/lingma2api && /tmp/lingma2api-vision-skeleton -config ./config.yaml &
sleep 2
curl -s -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"auto","stream":false,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUFB"}}]}]}' | tee /tmp/vision-resp.json
kill %1
cat /tmp/vision-resp.json
```
Expected: 返回 `{"error":{"message":"vision input is not implemented yet...","type":"not_implemented","code":"vision_not_implemented"}}`。

- [ ] **Step 5: 更新 frontend-dist（再次确保打包同步）**

Run:
```bash
cd /home/zipper/Projects/lingma2api/frontend && npm run build
cd ..
git status
```

如果 `frontend-dist/` 有新 diff，commit：

```bash
git add frontend-dist/
git commit -m "chore(frontend-dist): sync embedded build"
```

- [ ] **Step 6: 标记里程碑**

```bash
git tag -a vision-skeleton -m "Vision input protocol skeleton (image_urls reverse pending)"
git log --oneline -10
```

骨架交付完成。下一阶段开始反向任务（独立 plan：`docs/superpowers/specs/2026-05-XX-vision-image-urls-reverse.md`）。

---

## Self-Review 备忘

在 plan 全部 task 完成后，回到 spec 核对：

1. **Spec § 6.1 OpenAI Message 多模态 Content 解析** ⇒ Task 1-3, 6-7 覆盖。
2. **Spec § 6.2 Anthropic Message 入站补强** ⇒ Task 8-9 覆盖。
3. **Spec § 6.3 限额校验** ⇒ Task 4-5 覆盖。
4. **Spec § 6.4 Vision Gate** ⇒ Task 12-15 覆盖。
5. **Spec § 6.5 Body 注释** ⇒ Task 18 覆盖。
6. **Spec § 6.6 Settings** ⇒ Task 10-11 覆盖。
7. **Spec § 6.7 日志元数据** ⇒ Task 7、9 中 `imageBlockMetadata` 已写入 `Metadata`，由现有 `canonical_records` 持久化路径承接。
8. **Spec § 6.8 前端 LogDetail** ⇒ Task 19 覆盖。
9. **Spec § 7 错误响应汇总** ⇒ Task 14-17 覆盖。
10. **Spec § 8 测试策略** ⇒ Task 2、4、6、8、12、16、17、18 共 8 个测试 task 覆盖。
11. **Spec § 10 后续衔接** ⇒ Task 21 末尾打 tag，反向 plan 作为独立后续。
