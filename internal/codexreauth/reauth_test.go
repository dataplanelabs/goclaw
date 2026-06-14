package codexreauth

import (
	"testing"
)

func TestParseDeviceAuth_BothPresent(t *testing.T) {
	output := `
Checking for updates...
Please visit the following URL on your device:
https://auth.openai.com/activate?user_code=ABCD-1234
User code: ABCD-1234
Enter code: ABCD-1234
Waiting for approval...
`
	info := parseDeviceAuth(output)
	if info == nil {
		t.Fatal("expected non-nil DeviceAuthInfo")
	}
	if info.VerificationURL == "" {
		t.Error("expected VerificationURL to be set")
	}
	if info.UserCode != "ABCD-1234" {
		t.Errorf("expected UserCode 'ABCD-1234', got %q", info.UserCode)
	}
}

func TestParseDeviceAuth_MissingCode(t *testing.T) {
	output := `
Visit: https://auth.openai.com/activate
Waiting...
`
	info := parseDeviceAuth(output)
	if info != nil {
		t.Error("expected nil when user code is missing")
	}
}

func TestParseDeviceAuth_MissingURL(t *testing.T) {
	output := `
User code: XYZW-9876
Enter code: XYZW-9876
`
	info := parseDeviceAuth(output)
	if info != nil {
		t.Error("expected nil when URL is missing")
	}
}

func TestParseDeviceAuth_Empty(t *testing.T) {
	if parseDeviceAuth("") != nil {
		t.Error("expected nil for empty output")
	}
}

func TestParseDeviceAuth_URLExclusion(t *testing.T) {
	// URLs that don't contain openai.com or auth should be excluded
	output := `
Visit: https://example.com/check-updates
User code: ABCD-1234
`
	info := parseDeviceAuth(output)
	// example.com URL should be excluded, so info should be nil (no valid URL)
	if info != nil {
		t.Error("expected nil when URL doesn't match openai/auth pattern")
	}
}

func TestParseDeviceAuth_CodeFormats(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantCode string
	}{
		{"enter_code", "Enter code: ABCD-1234", "ABCD-1234"},
		{"user_code", "User code: WXYZ-5678", "WXYZ-5678"},
		{"case_insensitive", "USER CODE: AAAA-BBBB", "AAAA-BBBB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := "https://auth.openai.com/device\n" + tc.line
			info := parseDeviceAuth(output)
			if info == nil {
				t.Fatal("expected non-nil")
			}
			if info.UserCode != tc.wantCode {
				t.Errorf("got %q, want %q", info.UserCode, tc.wantCode)
			}
		})
	}
}
