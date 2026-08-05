package personal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

func TestIsUnreachable(t *testing.T) {
	// Shape the real outage produced: url.Error -> net.OpError -> net.DNSError.
	dnsFail := &url.Error{
		Op:  "Get",
		URL: "https://wpa.chat.zalo.me/api/login/getLoginInfo",
		Err: &net.OpError{
			Op:  "dial",
			Net: "tcp4",
			Err: &net.DNSError{Err: "server misbehaving", Name: "wpa.chat.zalo.me", IsTemporary: true},
		},
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"dns failure wrapped by channel layers", fmt.Errorf("re-auth: %w", fmt.Errorf("preloaded credentials failed: %w", dnsFail)), true},
		{"tls handshake timeout", fmt.Errorf("zalo_personal: server info: %w", &url.Error{Op: "Get", Err: errTimeout{}}), true},
		{"tls certificate verification", fmt.Errorf("re-auth: %w", &url.Error{Op: "Get", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}}), true},
		{"tls record header", fmt.Errorf("re-auth: %w", &url.Error{Op: "Get", Err: tls.RecordHeaderError{Msg: "bad record MAC"}}), true},
		{"connection refused", fmt.Errorf("re-auth: %w", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}), true},
		{"credential rejection", fmt.Errorf("re-auth: %w", errors.New("zalo_personal: invalid credentials")), false},
		{"decoded api error", fmt.Errorf("zalo_personal: login: %w", errors.New("error_code=100 session expired")), false},
	}

	for _, tt := range tests {
		if got := isUnreachable(tt.err); got != tt.want {
			t.Errorf("%s: isUnreachable = %v, want %v", tt.name, got, tt.want)
		}
	}
}

type errTimeout struct{}

func (errTimeout) Error() string   { return "net/http: TLS handshake timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }

func newRestartTestChannel() *Channel {
	return &Channel{
		BaseChannel: channels.NewBaseChannel(channels.TypeZaloPersonal, bus.New(), nil),
		stopCh:      make(chan struct{}),
	}
}

func TestRestartLoopBudget(t *testing.T) {
	netFail := &url.Error{Op: "Get", Err: &net.DNSError{Err: "server misbehaving", IsTemporary: true}}
	authFail := errors.New("zalo_personal: invalid credentials")
	noDelay := func(int) time.Duration { return 0 }

	t.Run("outage longer than the budget still recovers", func(t *testing.T) {
		c := newRestartTestChannel()
		rounds := 0
		restart := func(context.Context) error {
			rounds++
			if rounds <= maxChannelRestarts*5 {
				return netFail
			}
			return nil
		}
		if !c.restartLoop(context.Background(), restart, noDelay) {
			t.Fatal("channel gave up during a network outage; it must retry until connectivity returns")
		}
	})

	t.Run("credential rejections exhaust the budget", func(t *testing.T) {
		c := newRestartTestChannel()
		rounds := 0
		restart := func(context.Context) error {
			rounds++
			return authFail
		}
		if c.restartLoop(context.Background(), restart, noDelay) {
			t.Fatal("restartLoop reported success on persistent credential rejection")
		}
		if rounds != maxChannelRestarts {
			t.Errorf("restart attempts = %d, want %d", rounds, maxChannelRestarts)
		}
	})

	t.Run("network failures interleaved with auth failures do not shorten the budget", func(t *testing.T) {
		c := newRestartTestChannel()
		auth := 0
		restart := func(context.Context) error {
			if auth < maxChannelRestarts {
				auth++
				return authFail
			}
			return netFail
		}
		// 20 network failures up front must leave all 10 auth attempts intact.
		pre := 20
		wrapped := func(ctx context.Context) error {
			if pre > 0 {
				pre--
				return netFail
			}
			return restart(ctx)
		}
		if c.restartLoop(context.Background(), wrapped, noDelay) {
			t.Fatal("restartLoop reported success when credentials were never accepted")
		}
		if auth != maxChannelRestarts {
			t.Errorf("auth attempts consumed = %d, want %d", auth, maxChannelRestarts)
		}
	})

	t.Run("stopCh aborts the loop", func(t *testing.T) {
		c := newRestartTestChannel()
		close(c.stopCh)
		if c.restartLoop(context.Background(), func(context.Context) error { return netFail }, noDelay) {
			t.Fatal("restartLoop continued after stop")
		}
	})
}

func TestBackoffForPinsAtMax(t *testing.T) {
	if got := backoffFor(0); got != 2*time.Second {
		t.Errorf("backoffFor(0) = %v, want 2s", got)
	}
	for _, round := range []int{backoffRounds, 100, 1000} {
		if got := backoffFor(round); got != maxChannelBackoff {
			t.Errorf("backoffFor(%d) = %v, want %v", round, got, maxChannelBackoff)
		}
	}
}
