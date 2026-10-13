package tgclient

import (
	"context"
	"log/slog"

	"github.com/go-faster/errors"
	"github.com/gotd/td/tg"

	"github.com/ocyss/tg115-follow/internal/config"
)

// applyPrivacy sets authorization-level toggles on the CURRENT session, the
// same switches as Telegram → Settings → Devices → this device:
// "Accept calls" and "Accept secret chats". Idempotent and cheap (two RPCs),
// so it runs on every connect.
func applyPrivacy(ctx context.Context, log *slog.Logger, api *tg.Client, p config.Privacy) error {
	if !p.DisableCalls && !p.DisableSecretChats {
		return nil
	}
	auths, err := api.AccountGetAuthorizations(ctx)
	if err != nil {
		return errors.Wrap(err, "get authorizations")
	}
	for _, a := range auths.GetAuthorizations() {
		if !a.Current {
			continue
		}
		_, err := api.AccountChangeAuthorizationSettings(ctx, &tg.AccountChangeAuthorizationSettingsRequest{
			Hash:                      a.Hash,
			EncryptedRequestsDisabled: p.DisableSecretChats,
			CallRequestsDisabled:      p.DisableCalls,
		})
		if err != nil {
			return errors.Wrap(err, "change authorization settings")
		}
		log.InfoContext(ctx, "Session privacy applied",
			"calls_disabled", p.DisableCalls,
			"secret_chats_disabled", p.DisableSecretChats)
		return nil
	}
	return errors.New("current authorization not found")
}
