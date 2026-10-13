// tg115-follow listens to @HDHiveBot messages, runs daily check-in and
// notifies results via Feishu / a Telegram bot.
//
// Usage:
//
//	tg115-follow serve  [-config config.toml]   (default)
//	tg115-follow login  [-config config.toml] [-method qr|phone]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lmittmann/tint"
	"github.com/ocyss/tg115-follow/internal/checkin"
	"github.com/ocyss/tg115-follow/internal/config"
	"github.com/ocyss/tg115-follow/internal/listen"
	"github.com/ocyss/tg115-follow/internal/notify"
	"github.com/ocyss/tg115-follow/internal/pipeline"
	"github.com/ocyss/tg115-follow/internal/store"
	"github.com/ocyss/tg115-follow/internal/tgclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	configPath := fs.String("config", "data/config.toml", "path to TOML config file")
	method := fs.String("method", "", "login method override: qr|phone")
	password := fs.String("2fa", "", "2FA cloud password (one-off; remove after login)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *method != "" {
		cfg.Telegram.LoginMethod = *method
	}
	if *password != "" {
		cfg.Telegram.Password = *password
	}

	err = newLogger(cfg.Log)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := tgclient.New(cfg)

	switch cmd {
	case "login":
		slog.InfoContext(ctx, "Login mode", "method", cfg.Telegram.LoginMethod)
		return client.Login(ctx)
	case "serve":
		return serve(ctx, cfg, client)
	default:
		return fmt.Errorf("unknown command %q (expected serve|login)", cmd)
	}
}

func serve(ctx context.Context, cfg *config.Config, client *tgclient.Client) error {
	notifier := buildNotifier(cfg)
	if notifier.Empty() {
		slog.WarnContext(ctx, "No notifier configured, results will only be logged")
	}

	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		return err
	}
	pipe := pipeline.New(db, cfg.Logger("pipeline"))
	handler := listen.New(notifier, listen.ProcessorFunc(pipe.Process))

	// Register handlers once; the dispatcher survives reconnects.
	handler.Register(client.Dispatcher())

	return client.Run(ctx, func(ctx context.Context, s *tgclient.Session) error {
		if err := handler.ResolveTarget(ctx, s.Peers, cfg.Listen.Username); err != nil {
			return err
		}

		// Incrementally replay history missed while offline.
		cursor := store.GetKVInt(db, store.KeyLastMsgID, 0)
		go func() {
			n, err := listen.Backfill(ctx, s.API, handler.Peer(), cursor, cfg.Database.HistoryLimit, handler.HandleMessage)
			if err != nil {
				slog.WarnContext(ctx, "History backfill failed", "error", err, "fetched", n)
				return
			}
			slog.InfoContext(ctx, "History backfill done", "fetched", n, "cursor_before", cursor)
		}()

		if cfg.Checkin.Enabled {
			sched := checkin.New(cfg.Checkin, cfg.Logger("checkin"), s.Sender, handler.Peer(), notifier)
			sched.OnSent = func(ctx context.Context, sentAt time.Time) {
				if err := pipe.RecordCheckinSent(ctx, sentAt); err != nil {
					slog.WarnContext(ctx, "Record check-in failed", "error", err)
				}
			}
			go func() {
				if err := sched.Run(ctx); err != nil && ctx.Err() == nil {
					slog.WarnContext(ctx, "Check-in scheduler stopped", "error", err)
				}
			}()
		}

		notifier.Notify(ctx, "tg115-follow 已启动",
			fmt.Sprintf("监听 @%s", cfg.Listen.Username))
		return nil
	})
}

func buildNotifier(cfg *config.Config) *notify.Multi {
	var ns []notify.Notifier
	if cfg.Notify.Feishu.Enabled {
		ns = append(ns, notify.NewFeishu(cfg.Notify.Feishu.Webhook))
	}
	if cfg.Notify.TGBot.Enabled {
		ns = append(ns, notify.NewTGBot(cfg.Notify.TGBot.BotToken, cfg.Notify.TGBot.ChatID))
	}
	return notify.NewMulti(cfg.Logger("notify"), ns...)
}

func newLogger(conf config.Log) error {
	var hand slog.Handler
	if conf.Formatter == "json" {
		// Inner handlers must accept everything; the wrapper filters.
		hand = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		hand = tint.NewTextHandler(os.Stderr, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: "06/01/02 15:04:05",
			AddSource:  true,
		})
	}
	slog.SetDefault(slog.New(config.NewLogHandler(hand, &conf.Level, nil)))
	return nil
}
