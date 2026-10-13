package config

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
)

// ComponentKey is the slog attribute used to route records to per-component
// levels (log.With("component", "tg")).
const ComponentKey = "component"

// Levels maps components to minimum levels, env_logger style:
//
//	level = "debug,tg=info,gaps=warn"
//
// The bare first token is the global default. The "tg" component (gotd
// internals) is special: it defaults to warn and does NOT inherit the global
// level, because it is extremely chatty at debug.
type Levels struct {
	Default slog.Level
	Per     map[string]slog.Level
}

// DefaultLevels returns the built-in configuration: info globally, tg at warn.
func DefaultLevels() Levels {
	return Levels{
		Default: slog.LevelInfo,
		Per: map[string]slog.Level{
			"tg":       slog.LevelWarn,
			"tg/gaps":  slog.LevelWarn,
			"tg/peers": slog.LevelWarn,
		},
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for TOML strings like
// "debug,tg=info". The tg=warn preset is kept unless explicitly overridden.
func (l *Levels) UnmarshalText(text []byte) error {
	*l = DefaultLevels()
	for tok := range strings.SplitSeq(string(text), ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if name, v, ok := strings.Cut(tok, "="); ok {
			var lv slog.Level
			if err := lv.UnmarshalText([]byte(strings.TrimSpace(v))); err != nil {
				return fmt.Errorf("invalid level for %q: %q", name, v)
			}
			l.Per[strings.TrimSpace(name)] = lv
			continue
		}
		if err := l.Default.UnmarshalText([]byte(tok)); err != nil {
			return fmt.Errorf("invalid default level %q", tok)
		}
	}
	return nil
}

// For returns the effective minimum level for a component.
func (l Levels) For(component string) slog.Level {
	if v, ok := l.Per[component]; ok {
		return v
	}
	return l.Default
}

func (c *Config) Logger(component string) *slog.Logger {
	return c.Log.Level.Logger(component)
}

func (l *Levels) Logger(component string) *slog.Logger {
	return l.LoggerWith(component, nil)
}

type CallerSkip int

const (
	CallerSkipNone CallerSkip = 0

	CallerSkipTD CallerSkip = 2
)

type LoggerOptions struct {
	CallerSkip CallerSkip
}

func (c *Config) LoggerWith(component string, opt *LoggerOptions) *slog.Logger {
	return c.Log.Level.LoggerWith(component, opt)
}

func (l *Levels) LoggerWith(component string, opt *LoggerOptions) *slog.Logger {
	return slog.New(NewLogHandler(slog.Default().Handler(), l, opt).
		WithAttrs([]slog.Attr{slog.String(ComponentKey, component)}))
}

// NewLogHandler wraps inner so each record is filtered against the level of
// its "component" attribute. The inner handler must accept everything (set
// its own level to debug).
func NewLogHandler(inner slog.Handler, levels *Levels, opt *LoggerOptions,
) slog.Handler {
	if opt == nil {
		opt = &LoggerOptions{CallerSkip: CallerSkipNone}
	}
	return &levelHandler{
		inner:      inner,
		levels:     levels,
		level:      levels.Default,
		callerSkip: opt.CallerSkip,
	}
}

type levelHandler struct {
	inner      slog.Handler
	levels     *Levels
	level      slog.Level // effective minimum for this chain
	callerSkip CallerSkip
}

func (h *levelHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.PC != 0 && h.callerSkip > 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})

		for i := CallerSkip(0); i < h.callerSkip; i++ {
			_, _ = frames.Next()
		}

		if f, _ := frames.Next(); f.File != "" {
			r.PC = 0
		}
	}

	return h.inner.Handle(ctx, r)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	level := h.level
	for _, a := range attrs {
		if a.Key == ComponentKey && a.Value.Kind() == slog.KindString {
			level = h.levels.For(a.Value.String())
		}
	}
	return &levelHandler{inner: h.inner.WithAttrs(attrs), levels: h.levels, level: level, callerSkip: h.callerSkip}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{inner: h.inner.WithGroup(name), levels: h.levels, level: h.level, callerSkip: h.callerSkip}
}
