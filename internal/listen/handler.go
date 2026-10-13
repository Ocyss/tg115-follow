// Package listen filters incoming messages to the configured bot and
// dispatches them to the processing pipeline.
package listen

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/ocyss/tg115-follow/internal/notify"
	"github.com/ocyss/tg115-follow/internal/store"
	"github.com/ocyss/tg115-follow/internal/utils"
)

var urlRe = regexp.MustCompile(`https?://[^\s<>()\[\]"'“”]+`)

// Message is a message received from the target bot.
type Message struct {
	ID       int
	Text     string
	URLs     []string
	Entities []store.Entity
	Date     time.Time
	// RawJSON is the JSON encoding of the original *tg.Message.
	RawJSON []byte
}

// Processor handles a message from the target bot. The 115 transfer / URL
// unlock crawler plugs in here later.
type Processor interface {
	Process(ctx context.Context, msg Message) error
}

// ProcessorFunc adapts a function to Processor.
type ProcessorFunc func(ctx context.Context, msg Message) error

// Process implements Processor.
func (f ProcessorFunc) Process(ctx context.Context, msg Message) error { return f(ctx, msg) }

// Handler listens for messages of the target bot only. Anything else is
// dropped in-memory and never stored.
type Handler struct {
	notifier  *notify.Multi
	processor Processor

	targetID int64
	peer     tg.InputPeerClass
}

func New(n *notify.Multi, p Processor) *Handler {
	return &Handler{notifier: n, processor: p}
}

// ResolveTarget resolves the bot username and caches its ID in memory.
func (h *Handler) ResolveTarget(ctx context.Context, pm *peers.Manager, username string) error {
	p, err := pm.Resolve(ctx, username)
	if err != nil {
		return errors.Wrapf(err, "resolve @%s", username)
	}
	user, ok := p.(peers.User)
	if !ok {
		return errors.Errorf("@%s is not a user/bot", username)
	}
	h.targetID = user.ID()
	h.peer = user.InputPeer()
	slog.InfoContext(ctx, "Listening target resolved",
		"username", username, "id", h.targetID)
	return nil
}

// Peer returns the input peer of the target bot (used to send check-in).
func (h *Handler) Peer() tg.InputPeerClass { return h.peer }

// Register installs update handlers on the dispatcher.
func (h *Handler) Register(d *tg.UpdateDispatcher) {
	d.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		return h.HandleMessage(ctx, u.Message)
	})
}

// HandleMessage filters and processes a raw Telegram message. It is used by
// both the live update dispatcher and the history backfill.
func (h *Handler) HandleMessage(ctx context.Context, msg tg.MessageClass) error {
	m, ok := msg.(*tg.Message)
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "Received message", "msg_id", m.ID, "msg", utils.MustJsonMarshal(m))
	}
	if !ok || h.targetID == 0 {
		return nil
	}

	from, fromOK := m.FromID.(*tg.PeerUser)
	peer, peerOK := m.PeerID.(*tg.PeerUser)
	if (!fromOK || from.UserID != h.targetID) &&
		(!peerOK || peer.UserID != h.targetID) {
		return nil
	}

	parsed := Message{
		ID:       m.ID,
		Text:     m.Message,
		URLs:     extractURLs(m),
		Entities: extractEntities(m),
		Date:     time.Unix(int64(m.Date), 0),
		RawJSON:  []byte(utils.MustJsonMarshal(m)),
	}
	slog.InfoContext(ctx, "Message from target bot",
		"msg_id", m.ID, "url_count", len(parsed.URLs))

	if h.processor != nil {
		if err := h.processor.Process(ctx, parsed); err != nil {
			slog.WarnContext(ctx, "Process message failed", "error", err)
			h.notifier.Notify(ctx, "tg115-follow 处理失败", err.Error())
		}
	}
	return nil
}

// extractEntities flattens the tg entities we care about.
func extractEntities(m *tg.Message) []store.Entity {
	out := make([]store.Entity, 0, len(m.Entities))
	for _, e := range m.Entities {
		switch te := e.(type) {
		case *tg.MessageEntityBold:
			out = append(out, store.Entity{Type: "bold", Offset: te.Offset, Length: te.Length})
		case *tg.MessageEntityTextURL:
			out = append(out, store.Entity{Type: "text_url", Offset: te.Offset, Length: te.Length, URL: te.URL})
		case *tg.MessageEntityURL:
			out = append(out, store.Entity{Type: "url", Offset: te.Offset, Length: te.Length})
		}
	}
	return out
}

// extractURLs collects URLs from both the raw text and message entities.
func extractURLs(m *tg.Message) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(u string) {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	for _, u := range urlRe.FindAllString(m.Message, -1) {
		add(u)
	}
	for _, e := range m.Entities {
		if tu, ok := e.(*tg.MessageEntityTextURL); ok {
			add(tu.URL)
		}
	}
	return out
}
