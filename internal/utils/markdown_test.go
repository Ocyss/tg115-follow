package utils

import (
	"testing"

	"github.com/ocyss/tg115-follow/internal/store"
)

func TestRenderMarkdown(t *testing.T) {
	text := "✅ 订阅成功\n\n👉 查看资源详情"
	// ✅ = 2 utf16 units; offsets mirror the real bot message.
	entities := []store.Entity{
		{Type: "bold", Offset: 2, Length: 4},
		{Type: "text_url", Offset: 9, Length: 6, URL: "https://re0.me/tv/abc"},
	}
	got := RenderMarkdown(text, entities)
	want := "✅ **订阅成功**\n\n👉 [查看资源详情](https://re0.me/tv/abc)"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestRenderMarkdownNone(t *testing.T) {
	if got := RenderMarkdown("plain text", nil); got != "plain text" {
		t.Errorf("got %q", got)
	}
}

func TestRenderMarkdownOutOfRange(t *testing.T) {
	got := RenderMarkdown("abc", []store.Entity{{Type: "bold", Offset: 10, Length: 2}})
	if got != "abc" {
		t.Errorf("got %q", got)
	}
}
