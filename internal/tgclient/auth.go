package tgclient

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-faster/errors"
	"github.com/mdp/qrterminal/v3"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// qrAuth performs QR-code login: the code is printed to stdout as ASCII art
// so `docker compose logs -f` shows it and it can be scanned directly.
func qrAuth(ctx context.Context, client *telegram.Client, loggedIn qrlogin.LoggedIn, password string) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return errors.Wrap(err, "auth status")
	}
	if status.Authorized {
		return nil
	}

	fmt.Fprintln(os.Stdout,
		"\n>>> Scan this QR code with Telegram (Settings → Devices → Link Desktop Device):")
	show := func(ctx context.Context, token qrlogin.Token) error {
		qrterminal.GenerateHalfBlock(token.URL(), qrterminal.L, os.Stdout)
		fmt.Fprintf(os.Stdout, ">>> Or open: %s\n>>> Waiting for scan...\n\n", token.URL())
		return nil
	}

	if _, err := client.QR().Auth(ctx, loggedIn, show); err != nil {
		if !tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
			return errors.Wrap(err, "qr auth")
		}
		if err := handle2FA(ctx, client.Auth(), password); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stdout, ">>> QR login successful")
	return nil
}

// phoneAuth performs phone + code login with interactive prompts on stdin.
func phoneAuth(ctx context.Context, client *telegram.Client, phone, password string) error {
	t := &terminal{reader: bufio.NewReader(os.Stdin), phone: phone, password: password}
	flow := auth.NewFlow(t, auth.SendCodeOptions{})
	if err := client.Auth().IfNecessary(ctx, flow); err != nil {
		return errors.Wrap(err, "phone auth")
	}
	fmt.Fprintln(os.Stdout, ">>> Phone login successful")
	return nil
}

// handle2FA verifies the cloud password. When no password is provided and the
// process runs inside a container (no usable interactive terminal), it fails
// fast with instructions instead of blocking on stdin forever.
func handle2FA(ctx context.Context, a *auth.Client, password string) error {
	if password != "" {
		if _, err := a.Password(ctx, password); err != nil {
			return errors.Wrap(err, "2FA password (from -2fa flag / TG_2FA_PASSWORD)")
		}
		fmt.Fprintln(os.Stdout, ">>> 2FA OK. Tip: the session is saved, you should now "+
			"remove the -2fa flag / TG_2FA_PASSWORD env / telegram.password config entry.")
		return nil
	}
	if inDocker() {
		return errors.New("2FA password required but no interactive terminal is available in Docker. " +
			"Pass it once via `-2fa <password>` flag, TG_2FA_PASSWORD env or telegram.password config, " +
			"then remove it after the session file is created")
	}
	t := &terminal{reader: bufio.NewReader(os.Stdin)}
	for {
		fmt.Fprint(os.Stdout, ">>> Enter 2FA password: ")
		pwd, err := t.readLine()
		if err != nil {
			return errors.Wrap(err, "read 2FA password")
		}
		if _, err := a.Password(ctx, pwd); err != nil {
			if errors.Is(err, auth.ErrPasswordInvalid) {
				fmt.Fprintln(os.Stdout, ">>> Wrong password, try again.")
				continue
			}
			return errors.Wrap(err, "2FA password")
		}
		return nil
	}
}

// inDocker reports whether the process runs inside a container. The
// /.dockerenv marker file is the canonical Docker signal; the "container"
// env covers Podman/Kubernetes as a fallback.
func inDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return os.Getenv("container") != ""
}

// terminal implements auth.UserAuthenticator with stdin prompts.
type terminal struct {
	reader   *bufio.Reader
	phone    string
	password string
}

// SignUp implements [auth.UserAuthenticator].
func (t *terminal) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("not implemented")
}

func (t *terminal) readLine() (string, error) {
	line, err := t.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (t *terminal) Phone(ctx context.Context) (string, error) {
	if t.phone != "" {
		return t.phone, nil
	}
	fmt.Fprint(os.Stdout, ">>> Enter phone number (international format, e.g. +8613800138000): ")
	return t.readLine()
}

func (t *terminal) Password(ctx context.Context) (string, error) {
	if t.password != "" {
		fmt.Fprintln(os.Stdout, ">>> Using provided 2FA password. Tip: remove it after login, the session file persists.")
		return t.password, nil
	}
	fmt.Fprint(os.Stdout, ">>> Enter 2FA password: ")
	return t.readLine()
}

func (t *terminal) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Fprint(os.Stdout, ">>> Enter the code you received in Telegram: ")
	return t.readLine()
}

func (t *terminal) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return errors.New("account sign-up is not supported, please register the account first")
}

var _ auth.UserAuthenticator = (*terminal)(nil)
