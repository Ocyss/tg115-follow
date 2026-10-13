// Package notify fans out result messages to configured channels.
package notify

import (
	"context"
	"log/slog"
)

// Notifier sends a notification.
type Notifier interface {
	Notify(ctx context.Context, title, text string) error
}

// Multi dispatches to several notifiers, logging failures without stopping.
type Multi struct {
	notifiers []Notifier
	log       *slog.Logger
}

func NewMulti(log *slog.Logger, ns ...Notifier) *Multi {
	return &Multi{notifiers: ns, log: log}
}

func (m *Multi) Notify(ctx context.Context, title, text string) {
	for _, n := range m.notifiers {
		if err := n.Notify(ctx, title, text); err != nil {
			m.log.WarnContext(ctx, "Notify failed", "error", err)
		}
	}
}

// Empty reports whether no notifier is configured.
func (m *Multi) Empty() bool { return len(m.notifiers) == 0 }
