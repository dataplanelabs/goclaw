package whatsapp

import "testing"

func TestRemainingContentAfterMediaCaption(t *testing.T) {
	tests := []struct {
		name    string
		content string
		caption string
		mime    string
		want    string
	}{
		{name: "content used as caption", content: "response", mime: "image/png", want: ""},
		{name: "duplicate explicit caption", content: "response", caption: "response", mime: "application/pdf", want: ""},
		{name: "distinct response", content: "response", caption: "file caption", mime: "application/pdf", want: "response"},
		{name: "audio caption not supported", content: "response", caption: "response", mime: "audio/mpeg", want: "response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remainingContentAfterMediaCaption(tt.content, tt.caption, tt.mime); got != tt.want {
				t.Fatalf("remainingContentAfterMediaCaption() = %q, want %q", got, tt.want)
			}
		})
	}
}
