package utils

import (
	"sort"
	"strings"

	"github.com/ocyss/tg115-follow/internal/store"
)

// marker is a markdown token inserted at a rune boundary.
type marker struct {
	pos   int
	pre   bool // opening marker
	order int  // stable ordering for same-position markers
	token string
}

// RenderMarkdown renders plain text with Telegram entities as markdown:
// bold -> **text**, text_url -> [text](url). Plain url entities are left
// untouched (markdown autolinks bare URLs). Out-of-range entities are
// skipped. Entity offsets are UTF-16 code units.
func RenderMarkdown(text string, entities []store.Entity) string {
	if len(entities) == 0 {
		return text
	}
	runes := []rune(text)
	var markers []marker
	order := 0
	for _, e := range entities {
		var open, close string
		switch e.Type {
		case "bold":
			open, close = "**", "**"
		case "text_url":
			if e.URL == "" {
				continue
			}
			open, close = "[", "]("+e.URL+")"
		default:
			continue
		}
		start, end, ok := UTF16Bounds(runes, e.Offset, e.Length)
		if !ok || start >= end {
			continue
		}
		markers = append(
			markers,
			marker{pos: start, pre: true, order: order, token: open},
			marker{pos: end, pre: false, order: order, token: close},
		)
		order++
	}
	// Openers first at a position (stable by entity order); closers before
	// openers so ranges never interleave incorrectly.
	sort.SliceStable(markers, func(i, j int) bool {
		if markers[i].pos != markers[j].pos {
			return markers[i].pos < markers[j].pos
		}
		if markers[i].pre != markers[j].pre {
			return !markers[i].pre // closers first
		}
		return markers[i].order < markers[j].order
	})

	var sb strings.Builder
	mi := 0
	flush := func(pos int) {
		for mi < len(markers) && markers[mi].pos == pos {
			sb.WriteString(markers[mi].token)
			mi++
		}
	}
	for i, r := range runes {
		flush(i)
		sb.WriteRune(r)
	}
	flush(len(runes))
	return sb.String()
}
