package personal

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

func TestMaxMediaBytesIs100MB(t *testing.T) {
	if maxMediaBytes != 100*1024*1024 {
		t.Fatalf("maxMediaBytes = %d, want 100MB", maxMediaBytes)
	}
}

func TestAttachmentUnavailableText(t *testing.T) {
	tooLarge := fmt.Errorf("%w: 70646400 bytes (max %d)", errFileTooLarge, maxMediaBytes)
	transient := errors.New("download: context deadline exceeded")

	fileAtt := &protocol.Attachment{Title: "BÁO GIÁ TỔNG.xlsx", Href: "https://files.zalo.me/x"}
	imgAtt := &protocol.Attachment{Title: "", Href: "https://f20.zdn.vn/jpg/abc.jpg"}

	tests := []struct {
		name    string
		att     *protocol.Attachment
		err     error
		want    []string // substrings that must appear
		notWant []string
	}{
		{"too-large file names size limit + filename + workaround",
			fileAtt, tooLarge,
			[]string{`"BÁO GIÁ TỔNG.xlsx"`, "100 MB size limit", "too large", "download link", "screenshots"},
			[]string{"temporary error"}},
		{"transient file → temporary error, resend",
			fileAtt, transient,
			[]string{`"BÁO GIÁ TỔNG.xlsx"`, "temporary error", "resend"},
			[]string{"size limit"}},
		{"image kind labelled as image",
			imgAtt, tooLarge,
			[]string{"an image", "100 MB"},
			[]string{"a file"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := attachmentUnavailableText(tt.att, tt.err)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in: %s", w, got)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("unexpected %q in: %s", nw, got)
				}
			}
		})
	}
}
