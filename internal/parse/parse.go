// Package parse extracts structured data from the target bot's messages.
// Parsing failures never error: the result falls back to MessageTypeUnknown
// and the raw message is still stored, so parsing can be replayed later.
package parse

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ocyss/tg115-follow/internal/store"
	"github.com/ocyss/tg115-follow/internal/utils"
)

var (
	reSubscribeTitle = regexp.MustCompile(`资源订阅\s*\n+\s*(.+)`)
	reMediaType      = regexp.MustCompile(`类型[:：]\s*(剧集|电影)`)
	reUpdateTitle    = regexp.MustCompile(`^\[(剧集|电影)·([^\]]+)\]\s*(.+)$`)
	reShareName      = regexp.MustCompile(`分享[:：]\s*(.+)`)
	reTMDB           = regexp.MustCompile(`TMDB[:：]\s*(\d+)`)
	reSize           = regexp.MustCompile(`(?m)^大小[:：][ \t]*(\S[^\n]*)`)
	reSubtitle       = regexp.MustCompile(`(?m)^字幕[:：][ \t]*(\S[^\n]*)`)
)

// SubscribeInfo is the parsed "订阅成功" payload.
type SubscribeInfo struct {
	Title     string          `json:"title"` // includes year, e.g. "择日飞升 (2026)"
	MediaType store.MediaType `json:"media_type"`
	DetailURL string          `json:"detail_url"`
}

// UpdateInfo is the parsed "已更新" payload.
type UpdateInfo struct {
	Title       string          `json:"title"` // includes year, e.g. "吞噬星空 (2020)"
	MediaType   store.MediaType `json:"media_type"`
	DiskType    string          `json:"disk_type,omitempty"` // 115 / ed2k / ...
	ShareName   string          `json:"share_name"`
	ShareURL    string          `json:"share_url"`
	ResourceURL string          `json:"resource_url"`
	TMDBID      int             `json:"tmdb_id"`
	Size        string          `json:"size,omitempty"`
	Subtitle    string          `json:"subtitle,omitempty"`
}

// Result of parsing one message.
type Result struct {
	Type      store.MessageType
	Subscribe *SubscribeInfo
	Update    *UpdateInfo
}

// Parse classifies and extracts a message from the target bot.
func Parse(text string, entities []store.Entity) Result {
	switch {
	case strings.Contains(text, "订阅成功"):
		return Result{Type: store.MessageTypeSubscribe, Subscribe: parseSubscribe(text, entities)}
	case strings.Contains(text, "已更新"):
		return Result{Type: store.MessageTypeUpdate, Update: parseUpdate(text, entities)}
	default:
		return Result{Type: store.MessageTypeUnknown}
	}
}

func parseSubscribe(text string, entities []store.Entity) *SubscribeInfo {
	info := &SubscribeInfo{MediaType: store.MediaTypeOther}
	if m := reSubscribeTitle.FindStringSubmatch(text); m != nil {
		info.Title = strings.TrimSpace(m[1])
	}
	if m := reMediaType.FindStringSubmatch(text); m != nil {
		info.MediaType = mediaType(m[1])
	}
	for _, e := range entities {
		if e.Type == "text_url" && strings.Contains(e.URL, "re0.me/") {
			info.DetailURL = e.URL
		}
	}
	return info
}

func parseUpdate(text string, entities []store.Entity) *UpdateInfo {
	info := &UpdateInfo{MediaType: store.MediaTypeOther}

	// The first bold entity without a URL is the structured title line, e.g.
	// "[剧集·115] 吞噬星空 (2020)".
	for _, e := range entities {
		if e.Type != "bold" || e.URL != "" {
			continue
		}
		line := utils.UTF16Slice(text, e.Offset, e.Length)
		if m := reUpdateTitle.FindStringSubmatch(line); m != nil {
			info.MediaType = mediaType(m[1])
			info.DiskType = m[2]
			info.Title = strings.TrimSpace(m[3])
			break
		}
	}
	if m := reShareName.FindStringSubmatch(text); m != nil {
		info.ShareName = strings.TrimSpace(m[1])
	}
	if m := reTMDB.FindStringSubmatch(text); m != nil {
		info.TMDBID, _ = strconv.Atoi(m[1])
	}
	if m := reSize.FindStringSubmatch(text); m != nil {
		info.Size = m[1]
	}
	if m := reSubtitle.FindStringSubmatch(text); m != nil {
		info.Subtitle = strings.TrimSpace(m[1])
	}
	for _, e := range entities {
		if e.Type != "text_url" {
			continue
		}
		switch {
		case strings.Contains(e.URL, "re0.me/user/"):
			info.ShareURL = e.URL
		case strings.Contains(e.URL, "/resource/"):
			info.ResourceURL = e.URL
		}
	}
	return info
}

func mediaType(s string) store.MediaType {
	switch s {
	case "剧集":
		return store.MediaTypeSeries
	case "电影":
		return store.MediaTypeMovie
	default:
		return store.MediaTypeOther
	}
}
