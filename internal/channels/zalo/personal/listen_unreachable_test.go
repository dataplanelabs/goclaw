package personal

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
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
