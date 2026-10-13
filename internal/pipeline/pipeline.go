// Package pipeline persists and processes messages from the target bot:
// idempotent raw logging, resource/share-user upserts and check-in results.
package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ocyss/tg115-follow/internal/listen"
	"github.com/ocyss/tg115-follow/internal/parse"
	"github.com/ocyss/tg115-follow/internal/store"
	"github.com/ocyss/tg115-follow/internal/utils"
)

var rePoints = regexp.MustCompile(`积分[^\d]*\+?\s*(\d+)`)

// Pipeline implements listen.Processor backed by the database.
type Pipeline struct {
	db  *gorm.DB
	log *slog.Logger
}

// New creates a Pipeline.
func New(db *gorm.DB, log *slog.Logger) *Pipeline {
	return &Pipeline{db: db, log: log}
}

// Process parses and stores one message. Safe to call concurrently and to
// re-call with the same message (idempotent on tg_msg_id).
func (p *Pipeline) Process(ctx context.Context, msg listen.Message) error {
	res := parse.Parse(msg.Text, msg.Entities)
	p.advanceCursor(msg.ID)

	// Idempotency: already logged messages are skipped entirely.
	var count int64
	if err := p.db.WithContext(ctx).Model(&store.Message{}).
		Where("tg_msg_id = ?", msg.ID).Count(&count).Error; err != nil {
		return errors.Wrap(err, "check message")
	}
	if count > 0 {
		return nil
	}

	row := &store.Message{
		TgMsgID:  msg.ID,
		Type:     res.Type,
		Date:     msg.Date,
		RawJson:  msg.RawJSON,
		Text:     utils.RenderMarkdown(msg.Text, msg.Entities),
		Entities: datatypes.NewJSONSlice(msg.Entities),
		URLs:     datatypes.NewJSONSlice(msg.URLs),
	}
	if res.Subscribe != nil {
		row.Parsed = mustJSON(res.Subscribe)
	} else if res.Update != nil {
		row.Parsed = mustJSON(res.Update)
	}
	if err := p.db.WithContext(ctx).Create(row).Error; err != nil {
		return errors.Wrap(err, "store message")
	}

	switch res.Type {
	case store.MessageTypeUpdate:
		// Subscribe confirmations are recorded only; nothing to act on yet.
		return p.applyUpdate(ctx, row.ID, res.Update)
	default:
		p.maybeCheckinReply(ctx, msg)
		return nil
	}
}

func (p *Pipeline) applyUpdate(ctx context.Context, msgID uint, u *parse.UpdateInfo) error {
	if u == nil {
		return nil
	}

	// Share user: append-or-update keyed by user URL.
	var shareID *uint
	if u.ShareURL != "" {
		su := store.ShareUser{Name: u.ShareName, UserURL: u.ShareURL}
		err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_url"}},
			DoUpdates: clause.Assignments(map[string]any{
				"name":       gorm.Expr("excluded.name"),
				"updated_at": gorm.Expr("excluded.updated_at"),
				"extra": gorm.Expr(`
							CASE
								WHEN share_users.name IS NOT excluded.name THEN
									json_patch(
										COALESCE(share_users.extra, json('{}')),
										json_object(
											'old_names',
											json_patch(
												COALESCE(
													json_extract(share_users.extra, '$.old_names'),
													json('{}')
												),
												json_object(
													share_users.name,
													excluded.updated_at
												)
											)
										)
									)
								ELSE
									share_users.extra
							END
            `),
			}),
		}).Create(&su).Error
		if err != nil {
			return errors.Wrap(err, "upsert share user")
		}
		shareID = &su.ID
	}

	// Resources are never deduped by title: the same media may be shared on
	// different disks or by different uploaders. Only the share link is
	// unique: existing link → update in place, otherwise append a new row.
	r := store.Resource{
		Title:        u.Title,
		MediaType:    u.MediaType,
		DiskType:     u.DiskType,
		TMDBID:       u.TMDBID,
		ResourceURL:  u.ResourceURL,
		ShareUserID:  shareID,
		MessageID:    msgID,
		UnlockStatus: store.UnlockStatusLocked,
		Extra:        datatypes.NewJSONType(store.ResourceExtra{Size: u.Size, Subtitle: u.Subtitle}),
	}
	if u.ResourceURL == "" {
		if err := p.db.WithContext(ctx).Create(&r).Error; err != nil {
			return errors.Wrap(err, "create resource")
		}
		return nil
	}
	err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "resource_url"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "media_type", "disk_type", "tmdb_id",
			"share_user_id", "message_id", "extra", "updated_at",
		}),
	}).Create(&r).Error
	if err != nil {
		return errors.Wrap(err, "upsert resource")
	}
	return nil
}

// RecordCheckinSent upserts today's check-in log after the command was sent.
func (p *Pipeline) RecordCheckinSent(ctx context.Context, sentAt time.Time) error {
	day := store.Day(sentAt)
	var log store.CheckinLog
	err := p.db.WithContext(ctx).Where("day = ?", day).First(&log).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrap(err, "find checkin log")
	}
	log.Day = day
	log.SentAt = sentAt
	if log.Status == "" {
		log.Status = store.CheckinStatusSent
	}
	return p.db.WithContext(ctx).Save(&log).Error
}

// maybeCheckinReply applies simple keyword matching to unrecognized messages
// that look like a check-in reply. Deeper (AI-assisted) parsing plugs in
// later; unmatched replies stay stored as unknown for replay.
func (p *Pipeline) maybeCheckinReply(ctx context.Context, msg listen.Message) {
	if !strings.Contains(msg.Text, "签到") {
		return
	}
	day := store.Day(time.Now())
	var log store.CheckinLog
	err := p.db.WithContext(ctx).Where("day = ?", day).First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log = store.CheckinLog{Day: day, Status: store.CheckinStatusSent}
	} else if err != nil {
		p.log.WarnContext(ctx, "Find checkin log failed", "error", err)
		return
	}
	log.Reply = mustJSON(msg.Text)
	if strings.Contains(msg.Text, "成功") {
		log.Status = store.CheckinStatusSuccess
		if m := rePoints.FindStringSubmatch(msg.Text); m != nil {
			log.Points, _ = strconv.Atoi(m[1])
		}
	} else if strings.Contains(msg.Text, "失败") || strings.Contains(msg.Text, "已签到") {
		log.Status = store.CheckinStatusFailed
	}
	if err := p.db.WithContext(ctx).Save(&log).Error; err != nil {
		p.log.WarnContext(ctx, "Save checkin log failed", "error", err)
		return
	}
	p.db.WithContext(ctx).Model(&store.Message{}).
		Where("tg_msg_id = ?", msg.ID).
		Update("type", store.MessageTypeCheckinReply)
	p.log.InfoContext(ctx, "Check-in reply recorded", "status", log.Status, "points", log.Points)
}

// advanceCursor monotonically moves the processed-message cursor.
func (p *Pipeline) advanceCursor(id int) {
	if store.GetKVInt(p.db, store.KeyLastMsgID, 0) >= id {
		return
	}
	if err := store.SetKVInt(p.db, store.KeyLastMsgID, id); err != nil {
		p.log.Warn("Advance cursor failed", "error", err)
	}
}

func mustJSON(v any) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}
