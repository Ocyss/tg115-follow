package tgclient

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"

	"github.com/gotd/td/telegram/dcs"

	"github.com/ocyss/tg115-follow/internal/config"
)

// buildResolver creates a DC resolver from the proxy list.
// The first proxy entry wins; without proxies a direct resolver is used
// (nil return means "use gotd default").
func buildResolver(proxies []config.Proxy) (dcs.Resolver, error) {
	if len(proxies) == 0 {
		return nil, nil
	}
	p := proxies[0]
	switch p.Type {
	case "mtproxy":
		secret, err := parseMTProxySecret(p.Secret)
		if err != nil {
			return nil, fmt.Errorf("mtproxy secret: %w", err)
		}
		return dcs.MTProxy(p.Addr, secret, dcs.MTProxyOptions{})
	case "socks5", "http":
		dial, err := buildDial(p)
		if err != nil {
			return nil, err
		}
		return dcs.Plain(dcs.PlainOptions{Dial: dial}), nil
	default:
		return nil, fmt.Errorf("unsupported proxy type %q", p.Type)
	}
}

// buildDial builds a DialFunc through a socks5 or HTTP CONNECT proxy.
func buildDial(p config.Proxy) (dcs.DialFunc, error) {
	switch p.Type {
	case "socks5":
		var auth *proxy.Auth
		if p.Username != "" {
			auth = &proxy.Auth{User: p.Username, Password: p.Password}
		}
		d, err := proxy.SOCKS5("tcp", p.Addr, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		if cd, ok := d.(proxy.ContextDialer); ok {
			return cd.DialContext, nil
		}
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				c, err := d.Dial(network, addr)
				ch <- result{c, err}
			}()
			select {
			case r := <-ch:
				return r.conn, r.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, nil
	case "http":
		u := &url.URL{Scheme: "http", Host: p.Addr}
		if p.Username != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		}
		d, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("http proxy dialer: %w", err)
		}
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				c, err := d.Dial(network, addr)
				ch <- result{c, err}
			}()
			select {
			case r := <-ch:
				return r.conn, r.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, nil
	}
	return nil, fmt.Errorf("unsupported dial proxy type %q", p.Type)
}

// parseMTProxySecret accepts hex secrets, with an optional "ee"/"dd" prefix
// as seen in tg://proxy links.
func parseMTProxySecret(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "ee") || strings.HasPrefix(s, "dd") {
		if _, err := hex.DecodeString(s); err == nil {
			return hex.DecodeString(s)
		}
	}
	return hex.DecodeString(s)
}
