// Package checkin sends a daily check-in command to the target bot at a
// fixed local time plus a random jitter.
package checkin

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"

	"github.com/ocyss/tg115-follow/internal/config"
	"github.com/ocyss/tg115-follow/internal/notify"
)

// Scheduler runs the daily check-in job.
type Scheduler struct {
	cfg      config.Checkin
	log      *slog.Logger
	sender   *message.Sender
	peer     tg.InputPeerClass
	notifier *notify.Multi

	// OnSent is invoked after the check-in command is sent (e.g. to record
	// the attempt in the database). Optional.
	OnSent func(ctx context.Context, sentAt time.Time)
}

func New(cfg config.Checkin, log *slog.Logger, sender *message.Sender, peer tg.InputPeerClass, n *notify.Multi) *Scheduler {
	return &Scheduler{cfg: cfg, log: log, sender: sender, peer: peer, notifier: n}
}

// Run blocks until ctx is cancelled, sending the check-in command once a day.
func (s *Scheduler) Run(ctx context.Context) error {
	for {
		next := s.nextRun(time.Now())
		s.log.InfoContext(ctx, "Next check-in scheduled", "at", next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if err := s.checkin(ctx); err != nil {
			s.log.WarnContext(ctx, "Check-in failed", "error", err)
			s.notifier.Notify(ctx, "tg115-follow 签到失败", err.Error())
		}
	}
}

// nextRun computes today's configured time (or tomorrow's if passed) plus a
// random jitter of 0..jitter_minutes.
func (s *Scheduler) nextRun(now time.Time) time.Time {
	t, err := time.ParseInLocation("15:04", s.cfg.Time, time.Local)
	if err != nil {
		t = time.Date(0, 1, 1, 9, 0, 0, 0, time.Local)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	if s.cfg.JitterMinutes > 0 {
		next = next.Add(time.Duration(rand.Int64N(int64(s.cfg.JitterMinutes)+1)) * time.Minute)
	}
	return next
}

func (s *Scheduler) checkin(ctx context.Context) error {
	if s.peer == nil {
		return errors.New("target peer not resolved")
	}
	if _, err := s.sender.To(s.peer).Text(ctx, s.cfg.Command); err != nil {
		return errors.Wrap(err, "send check-in command")
	}
	s.log.InfoContext(ctx, "Check-in command sent", "command", s.cfg.Command)
	if s.OnSent != nil {
		s.OnSent(ctx, time.Now())
	}
	return nil
}
