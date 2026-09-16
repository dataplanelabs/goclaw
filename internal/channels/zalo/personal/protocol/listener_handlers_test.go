package protocol

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAnyToDecimalString(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "315077459047", "315077459047"},
		{"float64 large", float64(315077459047), "315077459047"},
		{"float64 small", float64(42), "42"},
		{"float64 zero", float64(0), "0"},
		{"json.Number", json.Number("315077459047"), "315077459047"},
		{"int", 42, "42"},
		{"nil", nil, "<nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := anyToDecimalString(tt.input)
			if got != tt.want {
				t.Errorf("anyToDecimalString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestAnyToDecimalString_JSONUnmarshal verifies the fix for the actual bug:
// JSON number unmarshalled into `any` becomes float64, which fmt.Sprint
// renders in scientific notation for large values.
func TestAnyToDecimalString_JSONUnmarshal(t *testing.T) {
	// Simulate what happens when Zalo sends fileId as a number in JSON
	raw := `{"fileId": 315077459047}`
	var parsed struct {
		FileID any `json:"fileId"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}

	got := anyToDecimalString(parsed.FileID)
	if got != "315077459047" {
		t.Errorf("JSON number → anyToDecimalString = %q, want %q", got, "315077459047")
	}
}

func TestHandleUserMessages_EmitsPageMsgsAsOA(t *testing.T) {
	ln := &Listener{
		sess:      &Session{UID: "me"},
		messageCh: make(chan Message, 2),
		errorCh:   make(chan error, 2),
	}
	payload := `{"data":{"msgs":[],"pageMsgs":[{"msgId":"m1","uidFrom":"oa-99","idTo":"0","dName":"Bank OA","content":"otp","msgType":"webchat"}]}}`
	ln.handleUserMessages(context.Background(), payload, 0)
	select {
	case m := <-ln.messageCh:
		um, ok := m.(UserMessage)
		if !ok {
			t.Fatalf("got %T", m)
		}
		if !um.FromOA() {
			t.Fatal("want FromOA")
		}
		if um.ThreadID() != "oa-99" {
			t.Fatalf("thread=%s want oa-99", um.ThreadID())
		}
		if um.Data.DName != "Bank OA" {
			t.Fatalf("dname=%s", um.Data.DName)
		}
	default:
		t.Fatal("no pageMsg emitted")
	}
}

func TestHandleUserMessages_EmptyPageMsgsNoEmit(t *testing.T) {
	ln := &Listener{
		sess:      &Session{UID: "me"},
		messageCh: make(chan Message, 1),
		errorCh:   make(chan error, 1),
	}
	ln.handleUserMessages(context.Background(), `{"data":{"msgs":[],"pageMsgs":[]}}`, 0)
	select {
	case m := <-ln.messageCh:
		t.Fatalf("unexpected emit: %+v", m)
	default:
	}
}
