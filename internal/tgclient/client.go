// Package tgclient wraps a gotd Telegram client with auth, proxy dialing,
// gap-recovering updates and an outer reconnect loop.
package tgclient

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/log/logslog"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/ocyss/tg115-follow/internal/config"
)

// ReadyFunc is called after successful auth. It should register handlers and
// start background jobs, then return. The client keeps running afterwards.
type ReadyFunc func(ctx context.Context, s *Session) error

// Session exposes the authenticated runtime to business logic.
type Session struct {
	API    *tg.Client
	Peers  *peers.Manager
	Sender *message.Sender
	Self   peers.User
}

// Client manages the whole Telegram connection lifecycle.
type Client struct {
	cfg *config.Config

	dispatcher *tg.UpdateDispatcher
	loggedIn   qrlogin.LoggedIn

	// handler indirection: peers hook + gaps manager are built per connection,
	// after auth, while telegram.Options.UpdateHandler is fixed at creation.
	mux     sync.RWMutex
	handler telegram.UpdateHandler
}

// Dispatcher returns the update dispatcher for handler registration.
func (c *Client) Dispatcher() *tg.UpdateDispatcher { return c.dispatcher }

// New creates a Client from config.
func New(cfg *config.Config) *Client {
	dispatcher := tg.NewUpdateDispatcher()
	return &Client{
		cfg:        cfg,
		dispatcher: &dispatcher,
		loggedIn:   qrlogin.OnLoginToken(dispatcher),
		handler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return dispatcher.Handle(ctx, u)
		}),
	}
}

func (c *Client) dispatch(ctx context.Context, u tg.UpdatesClass) error {
	c.mux.RLock()
	h := c.handler
	c.mux.RUnlock()
	return h.Handle(ctx, u)
}

func (c *Client) newTelegramClient() (*telegram.Client, error) {
	resolver, err := buildResolver(c.cfg.Proxies)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(c.cfg.Telegram.SessionFile), 0o700); err != nil {
		return nil, errors.Wrap(err, "create session dir")
	}
	opts := telegram.Options{
		Logger: logslog.New(c.cfg.LoggerWith("tg", &config.LoggerOptions{
			CallerSkip: config.CallerSkipTD,
		})),
		SessionStorage: &session.FileStorage{Path: c.cfg.Telegram.SessionFile},
		UpdateHandler:  telegram.UpdateHandlerFunc(c.dispatch),
		// Client identity for initConnection. English values only, and only
		// what is required: nothing here leaks OS locale or host details.
		Device: telegram.DeviceConfig{
			DeviceModel:    "tg115-follow",
			SystemVersion:  runtime.GOOS,
			AppVersion:     config.Version,
			LangCode:       "en",
			SystemLangCode: "en-US",
		},
	}
	if resolver != nil {
		opts.Resolver = resolver
	}
	return telegram.NewClient(c.cfg.Telegram.AppID, c.cfg.Telegram.AppHash, opts), nil
}

func (c *Client) authenticate(ctx context.Context, client *telegram.Client) error {
	switch c.cfg.Telegram.LoginMethod {
	case "phone":
		return phoneAuth(ctx, client, c.cfg.Telegram.Phone, c.cfg.Telegram.Password)
	default:
		return qrAuth(ctx, client, c.loggedIn, c.cfg.Telegram.Password)
	}
}

// Login performs a single connection solely for authentication, then exits.
func (c *Client) Login(ctx context.Context) error {
	client, err := c.newTelegramClient()
	if err != nil {
		return err
	}
	return client.Run(ctx, func(ctx context.Context) error {
		return c.authenticate(ctx, client)
	})
}

// Run connects and keeps reconnecting (with backoff) until ctx is cancelled.
func (c *Client) Run(ctx context.Context, ready ReadyFunc) error {
	attempt := 0
	for {
		if err := c.runOnce(ctx, ready); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			attempt++
			delay := backoff(attempt)
			slog.WarnContext(ctx, "Connection lost, reconnecting",
				"error", err, "delay", delay, "attempt", attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return nil
	}
}

// backoff: 1s, 2s, 4s, ... capped at 5m, no jitter needed for a daemon.
func backoff(attempt int) time.Duration {
	d := time.Second << min(attempt-1, 9)
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}

func (c *Client) runOnce(ctx context.Context, ready ReadyFunc) error {
	client, err := c.newTelegramClient()
	if err != nil {
		return err
	}
	return client.Run(ctx, func(ctx context.Context) error {
		if err := c.authenticate(ctx, client); err != nil {
			return errors.Wrap(err, "auth")
		}
		if err := applyPrivacy(ctx, c.cfg.Logger("privacy"), client.API(), c.cfg.Privacy); err != nil {
			slog.WarnContext(ctx, "Failed to apply session privacy settings", "error", err)
		}

		// Peer manager with in-memory storage: peers are cached only in RAM,
		// nothing is written to disk besides the session file.
		pm := peers.Options{
			Logger: logslog.New(c.cfg.LoggerWith("tg/peers", &config.LoggerOptions{
				CallerSkip: config.CallerSkipTD,
			})),
		}.Build(client.API())
		if err := pm.Init(ctx); err != nil {
			return errors.Wrap(err, "init peers")
		}
		self, err := pm.Self(ctx)
		if err != nil {
			return errors.Wrap(err, "get self")
		}
		slog.InfoContext(ctx, "Logged in", "id", self.ID())

		// Gap recovery manager; state storage defaults to in-memory.
		gaps := updates.New(updates.Config{
			Handler:      c.dispatcher,
			AccessHasher: pm,
			Logger: logslog.New(c.cfg.LoggerWith("tg/gaps", &config.LoggerOptions{
				CallerSkip: config.CallerSkipTD,
			})),
		})
		c.mux.Lock()
		c.handler = pm.UpdateHook(gaps)
		c.mux.Unlock()

		s := &Session{
			API:    client.API(),
			Peers:  pm,
			Sender: message.NewSender(client.API()),
			Self:   self,
		}
		if err := ready(ctx, s); err != nil {
			return errors.Wrap(err, "ready")
		}

		// Blocks until ctx is done; handles ordering, gap recovery and
		// fetching missed updates after reconnects.
		return gaps.Run(ctx, client.API(), self.ID(), updates.AuthOptions{})
	})
}
