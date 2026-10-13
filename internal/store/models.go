// Package store defines the SQLite (gorm) persistence layer: enums, models
// and the database handle. Non-filtered attributes live in datatypes JSON
// columns instead of dedicated columns.
package store

import (
	"time"

	"gorm.io/datatypes"
)

// MessageType classifies a message received from the target bot.
type MessageType string

const (
	// MessageTypeSubscribe is the "✅ 订阅成功" subscription confirmation.
	MessageTypeSubscribe MessageType = "subscribe"
	// MessageTypeUpdate is a "已更新" resource-update notification.
	MessageTypeUpdate MessageType = "update"
	// MessageTypeCheckinReply is a reply to the daily check-in command.
	MessageTypeCheckinReply MessageType = "checkin_reply"
	// MessageTypeUnknown is anything else (kept raw for later replay).
	MessageTypeUnknown MessageType = "unknown"
)

// MediaType of a resource.
type MediaType string

const (
	MediaTypeSeries MediaType = "series"
	MediaTypeMovie  MediaType = "movie"
	MediaTypeOther  MediaType = "other"
)

// UnlockStatus tracks whether the share link has been unlocked with points.
type UnlockStatus string

const (
	UnlockStatusLocked   UnlockStatus = "locked"
	UnlockStatusUnlocked UnlockStatus = "unlocked"
)

// CheckinStatus of a daily check-in.
type CheckinStatus string

const (
	CheckinStatusSent    CheckinStatus = "sent"
	CheckinStatusSuccess CheckinStatus = "success"
	CheckinStatusFailed  CheckinStatus = "failed"
)

// Entity is a flattened Telegram message entity. Offset and Length are in
// UTF-16 code units (Telegram semantics); use utils.UTF16Slice to extract.
type Entity struct {
	Type   string `json:"type"` // bold | url | text_url | other
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

// ResourceExtra holds non-filtered resource attributes.
type ResourceExtra struct {
	Size     string `json:"size,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
}

// Message is the raw log of every message from the target bot, including
// unrecognized ones so the parser can be replayed later.
type Message struct {
	ID       uint        `gorm:"primaryKey"`
	TgMsgID  int         `gorm:"uniqueIndex;not null"`
	Type     MessageType `gorm:"size:20;index;not null"`
	Date     time.Time   `gorm:"index"`
	RawJson  []byte
	Text     string
	Entities datatypes.JSONSlice[Entity]
	URLs     datatypes.JSONSlice[string]
	// Parsed is the raw JSON payload of SubscribeInfo / UpdateInfo;
	// null when unknown.
	Parsed    datatypes.JSON
	CreatedAt time.Time
}

// ShareUser is the uploader ("分享：xxx") behind a resource link. Future
// point-saving strategies score share users.
type ShareUser struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"index"`
	UserURL   string `gorm:"uniqueIndex"` // https://re0.me/user/<id>
	Extra     datatypes.JSON
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Resource is a subscribed / tracked media resource. Resources are never
// deduped by title: the same media can be shared on different disks or by
// different uploaders, each with its own link. Only ResourceURL is unique.
type Resource struct {
	ID            uint      `gorm:"primaryKey"`
	Title         string    `gorm:"index"` // includes year, e.g. "吞噬星空 (2020)"
	MediaType     MediaType `gorm:"size:10"`
	DiskType      string    `gorm:"size:20;index"` // 115 / ed2k / ...
	TMDBID        int       `gorm:"index"`
	DetailURL     string    `gorm:"index"`       // re0.me/tv|movie/<uuid> detail page
	ResourceURL   string    `gorm:"uniqueIndex"` // re0.me/resource/... share link
	LatestEpisode int       // reserved, no longer extracted
	LongTerm      bool      // reserved, no longer extracted
	ShareUserID   *uint
	ShareUser     *ShareUser
	MessageID     uint         `gorm:"index"` // the message this resource came from
	Message       Message      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	UnlockStatus  UnlockStatus `gorm:"size:10"`
	Extra         datatypes.JSONType[ResourceExtra]
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CheckinLog records the daily check-in result.
type CheckinLog struct {
	ID     uint   `gorm:"primaryKey"`
	Day    string `gorm:"uniqueIndex;size:10"` // YYYY-MM-DD, local
	SentAt time.Time
	Status CheckinStatus `gorm:"size:10"`
	Points int
	// Reply is the raw JSON of the reply message text(s).
	Reply     datatypes.JSON
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KV is a tiny key-value table (message cursor etc.).
type KV struct {
	Key   string `gorm:"primaryKey;size:64"`
	Value string
}

// KeyLastMsgID is the KV key of the processed-message cursor.
const KeyLastMsgID = "last_processed_msg_id"
