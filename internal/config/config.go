// Package config loads and validates the TOML configuration file.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration.
type Config struct {
	Telegram Telegram `toml:"telegram"`
	Proxies  []Proxy  `toml:"proxy"`
	Listen   Listen   `toml:"listen"`
	Checkin  Checkin  `toml:"checkin"`
	Notify   Notify   `toml:"notify"`
	Privacy  Privacy  `toml:"privacy"`
	Database Database `toml:"database"`
	Log      Log      `toml:"log"`
}

// Telegram holds MTProto client credentials and session settings.
type Telegram struct {
	AppID       int    `toml:"app_id"`
	AppHash     string `toml:"app_hash"`
	SessionFile string `toml:"session_file"`
	// LoginMethod: "qr" (default) or "phone". Env LOGIN_METHOD overrides.
	LoginMethod string `toml:"login_method"`
	// Phone is optional for login_method=phone; prompted when empty.
	Phone string `toml:"phone"`
	// Password is the 2FA cloud password. Prefer env TG_2FA_PASSWORD or the
	// -2fa flag over writing it to the config file; required for QR login in
	// Docker (no interactive terminal) when 2FA is enabled.
	Password string `toml:"password"`
}

// Proxy describes a single outbound proxy. The first entry is used for now;
// the array form leaves room for future rotation/selection strategies.
type Proxy struct {
	Type     string `toml:"type"` // socks5 | http | mtproxy
	Addr     string `toml:"addr"` // host:port
	Username string `toml:"username"`
	Password string `toml:"password"`
	Secret   string `toml:"secret"` // mtproxy only, hex (may start with "ee")
}

// Listen configures which bot to listen to.
type Listen struct {
	Username string `toml:"username"` // e.g. "HDHiveBot"
}

// Database configures the SQLite store.
type Database struct {
	Path string `toml:"path"`
	// HistoryLimit caps how many recent messages are fetched on the first
	// run (no cursor yet). <= 0 means unlimited.
	HistoryLimit int `toml:"history_limit"`
}

// Checkin configures the daily check-in job.
type Checkin struct {
	Enabled       bool   `toml:"enabled"`
	Time          string `toml:"time"`           // "HH:MM" local time
	JitterMinutes int    `toml:"jitter_minutes"` // random delay added to Time
	Command       string `toml:"command"`        // message sent to the bot
}

// Notify groups notification channels.
type Notify struct {
	Feishu Feishu `toml:"feishu"`
	TGBot  TGBot  `toml:"tgbot"`
}

// Feishu custom bot webhook.
type Feishu struct {
	Enabled bool   `toml:"enabled"`
	Webhook string `toml:"webhook"`
}

// TGBot uses a standalone bot (from @BotFather) to push results.
type TGBot struct {
	Enabled  bool   `toml:"enabled"`
	BotToken string `toml:"bot_token"`
	ChatID   int64  `toml:"chat_id"`
}

// Log settings.
type Log struct {
	// Level accepts env_logger-style specs, e.g. "debug,tg=info,gaps=warn".
	// The bare token is the global default; "tg" defaults to warn and does
	// not inherit the global level.
	Level     Levels `toml:"level"`
	Formatter string `toml:"formatter"` // text | json
}

// Privacy controls authorization-level toggles on the login session itself
// (Telegram → Settings → Devices → this device). MTProto has no OAuth-like
// scope system, so these are the only client-side switches available.
type Privacy struct {
	// DisableCalls rejects incoming voice/video calls to this session.
	DisableCalls bool `toml:"disable_calls"`
	// DisableSecretChats rejects secret-chat requests to this session.
	DisableSecretChats bool `toml:"disable_secret_chats"`
}

// Load reads and validates the config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{
		Telegram: Telegram{
			SessionFile: "data/session.json",
			LoginMethod: "qr",
		},
		Listen:   Listen{Username: "HDHiveBot"},
		Checkin:  Checkin{Time: "09:00", JitterMinutes: 30, Command: "签到"},
		Privacy:  Privacy{DisableCalls: true, DisableSecretChats: true},
		Database: Database{Path: "data/app.db", HistoryLimit: 50},
		Log:      Log{Level: DefaultLevels(), Formatter: "text"},
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if v := os.Getenv("LOGIN_METHOD"); v != "" {
		cfg.Telegram.LoginMethod = v
	}
	if v := os.Getenv("TG_2FA_PASSWORD"); v != "" {
		cfg.Telegram.Password = v
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Telegram.AppID == 0 || c.Telegram.AppHash == "" {
		return fmt.Errorf("telegram.app_id and telegram.app_hash are required (https://my.telegram.org/apps)")
	}
	switch c.Telegram.LoginMethod {
	case "qr", "phone":
	default:
		return fmt.Errorf("telegram.login_method must be \"qr\" or \"phone\", got %q", c.Telegram.LoginMethod)
	}
	for i, p := range c.Proxies {
		switch p.Type {
		case "socks5", "http", "mtproxy":
		default:
			return fmt.Errorf("proxy[%d].type must be socks5|http|mtproxy, got %q", i, p.Type)
		}
		if p.Addr == "" {
			return fmt.Errorf("proxy[%d].addr is required", i)
		}
		if p.Type == "mtproxy" && p.Secret == "" {
			return fmt.Errorf("proxy[%d].secret is required for mtproxy", i)
		}
	}
	if c.Listen.Username == "" {
		return fmt.Errorf("listen.username is required")
	}
	if c.Checkin.Enabled {
		if _, err := time.Parse("15:04", c.Checkin.Time); err != nil {
			return fmt.Errorf("checkin.time must be HH:MM, got %q", c.Checkin.Time)
		}
		if c.Checkin.Command == "" {
			return fmt.Errorf("checkin.command is required when checkin is enabled")
		}
	}
	if c.Notify.Feishu.Enabled && c.Notify.Feishu.Webhook == "" {
		return fmt.Errorf("notify.feishu.webhook is required when feishu is enabled")
	}
	if c.Notify.TGBot.Enabled && (c.Notify.TGBot.BotToken == "" || c.Notify.TGBot.ChatID == 0) {
		return fmt.Errorf("notify.tgbot.bot_token and chat_id are required when tgbot is enabled")
	}
	return nil
}
