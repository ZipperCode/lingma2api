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
