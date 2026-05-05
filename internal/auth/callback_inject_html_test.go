package auth

import (
	"strings"
	"testing"
)

func TestCallbackAutoInjectHTML_ContainsCriticalParts(t *testing.T) {
	html := CallbackAutoInjectHTML
	mustContain := []string{
		"<!DOCTYPE html>",
		"window.user_info",
		"window.login_url",
		"http://127.0.0.1:37510/submit-userinfo",
		"application/json",
		"提交成功",
	}
	for _, s := range mustContain {
		if !strings.Contains(html, s) {
			t.Errorf("CallbackAutoInjectHTML missing %q", s)
		}
	}
}
