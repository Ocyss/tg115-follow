package listen

import (
	"context"
	"log/slog"

	"github.com/go-faster/errors"
	"github.com/gotd/td/tg"
)

// Backfill incrementally fetches history messages from the target peer newer
// than afterID (the processed-message cursor) and feeds them through handle,
// which must be idempotent. When afterID is 0 (first run) at most limit
// recent messages are fetched; limit <= 0 means no cap. Returns the number of
// messages seen.
func Backfill(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, afterID, limit int, handle func(context.Context, tg.MessageClass) error) (int, error) {
	if peer == nil {
		return 0, errors.New("target peer not resolved")
	}
	fetched := 0
	offsetID := 0
	for {
		batch := 100
		if afterID == 0 && limit > 0 {
			remaining := limit - fetched
			if remaining <= 0 {
				break
			}
			if remaining < batch {
				batch = remaining
			}
		}
		res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			MinID:    afterID,
			Limit:    batch,
		})
		if err != nil {
			return fetched, errors.Wrap(err, "messages.getHistory")
		}
		msgs := historyMessages(res)
		if len(msgs) == 0 {
			break
		}
		for _, mc := range msgs {
			if err := handle(ctx, mc); err != nil {
				slog.WarnContext(ctx, "Backfill process failed", "error", err)
			}
		}
		fetched += len(msgs)
		oldest, ok := msgs[len(msgs)-1].(*tg.Message)
		if !ok || len(msgs) < batch {
			break
		}
		offsetID = oldest.ID
	}
	return fetched, nil
}

func historyMessages(res tg.MessagesMessagesClass) []tg.MessageClass {
	switch r := res.(type) {
	case *tg.MessagesMessages:
		return r.Messages
	case *tg.MessagesMessagesSlice:
		return r.Messages
	case *tg.MessagesChannelMessages:
		return r.Messages
	}
	return nil
}
