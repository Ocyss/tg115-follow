package utils

import "testing"

func TestUTF16Slice(t *testing.T) {
	s := "您订阅的「吞噬星空」已更新\n\n[剧集·115] 吞噬星空 (2020)"
	got := UTF16Slice(s, 15, 20)
	if got != "[剧集·115] 吞噬星空 (2020)" {
		t.Errorf("got %q", got)
	}
}

func TestUTF16SliceEmoji(t *testing.T) {
	// Emoji is outside the BMP: 2 UTF-16 units each.
	s := "✅a😀bc"
	if got := UTF16Slice(s, 2, 1); got != "a" {
		t.Errorf("a: got %q", got)
	}
	if got := UTF16Slice(s, 3, 2); got != "😀" {
		t.Errorf("emoji: got %q", got)
	}
	if got := UTF16Slice(s, 5, 2); got != "bc" {
		t.Errorf("tail: got %q", got)
	}
}

func TestUTF16SliceOutOfRange(t *testing.T) {
	if got := UTF16Slice("abc", 5, 2); got != "" {
		t.Errorf("got %q", got)
	}
	if got := UTF16Slice("abc", -1, 2); got != "" {
		t.Errorf("got %q", got)
	}
}
