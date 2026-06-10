package vieneu

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := NewProvider(Config{Endpoint: srv.URL, TimeoutMs: 5000})
	return p, srv
}

func readBody(t *testing.T, r *http.Request) synthRequest {
	t.Helper()
	var req synthRequest
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return req
}

func TestSynthesize_PresetVoiceHappyPath(t *testing.T) {
	var got synthRequest
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = readBody(t, r)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("FAKEMP3"))
	})
	out, err := p.Synthesize(context.Background(), "Xin chào", audio.TTSOptions{Voice: "binh", Format: "mp3"})
	if err != nil {
		t.Fatalf("synth: %v", err)
	}
	if string(out.Audio) != "FAKEMP3" {
		t.Errorf("audio = %q", out.Audio)
	}
	if out.Extension != "mp3" || out.MimeType != "audio/mpeg" {
		t.Errorf("ext/mime = %s/%s", out.Extension, out.MimeType)
	}
	if got.VoiceID != "binh" || got.Text != "Xin chào" {
		t.Errorf("request = %+v", got)
	}
	if got.Speed != 1.0 {
		t.Errorf("speed default = %v, want 1.0", got.Speed)
	}
	if got.Emotion != "natural" {
		t.Errorf("emotion default = %q, want natural", got.Emotion)
	}
}

func TestSynthesize_FallsBackToCfgVoice(t *testing.T) {
	var got synthRequest
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = readBody(t, r)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("x"))
	})
	p.cfg.VoiceID = "truc_ly"
	_, _ = p.Synthesize(context.Background(), "hi", audio.TTSOptions{})
	if got.VoiceID != "truc_ly" {
		t.Errorf("voice fallback = %q", got.VoiceID)
	}
}

func TestSynthesize_CloningPathSendsRefFields(t *testing.T) {
	var got synthRequest
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = readBody(t, r)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("y"))
	})
	opts := audio.TTSOptions{
		Params: map[string]any{
			"ref_audio_path": "/data/vieneu-refs/abc/voice.wav",
			"ref_text":       "Xin chào, đây là giọng tham chiếu",
		},
	}
	_, err := p.Synthesize(context.Background(), "Xin chào", opts)
	if err != nil {
		t.Fatalf("synth: %v", err)
	}
	if got.RefAudioPath == "" || got.RefText == "" {
		t.Errorf("clone fields missing: %+v", got)
	}
}

func TestSynthesize_RefAudioWithoutTextRejected(t *testing.T) {
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("daemon should not be called")
	})
	opts := audio.TTSOptions{Params: map[string]any{"ref_audio_path": "/x.wav"}}
	_, err := p.Synthesize(context.Background(), "hi", opts)
	if !errors.Is(err, ErrRefAudioInvalid) {
		t.Errorf("err = %v, want ErrRefAudioInvalid", err)
	}
}

func TestSynthesize_EmptyTextRejected(t *testing.T) {
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("daemon should not be called")
	})
	_, err := p.Synthesize(context.Background(), "   ", audio.TTSOptions{})
	if !errors.Is(err, ErrSynthFailed) {
		t.Errorf("err = %v, want ErrSynthFailed", err)
	}
}

func TestSynthesize_5xxRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ok"))
	})
	out, err := p.Synthesize(context.Background(), "x", audio.TTSOptions{Voice: "binh"})
	if err != nil {
		t.Fatalf("synth: %v", err)
	}
	if string(out.Audio) != "ok" || calls.Load() != 2 {
		t.Errorf("calls=%d, audio=%q", calls.Load(), out.Audio)
	}
}

func TestSynthesize_4xxNoRetry(t *testing.T) {
	var calls atomic.Int32
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad", http.StatusBadRequest)
	})
	_, err := p.Synthesize(context.Background(), "x", audio.TTSOptions{Voice: "binh"})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Errorf("4xx should not retry; got %d calls", calls.Load())
	}
}

func TestSynthesize_DaemonUnreachable(t *testing.T) {
	p := NewProvider(Config{Endpoint: "http://127.0.0.1:1", TimeoutMs: 500})
	_, err := p.Synthesize(context.Background(), "x", audio.TTSOptions{Voice: "binh"})
	if !errors.Is(err, ErrDaemonUnreachable) {
		t.Errorf("err = %v, want ErrDaemonUnreachable", err)
	}
}

func TestSynthesize_DoesNotMutateOptsParams(t *testing.T) {
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ok"))
	})
	params := map[string]any{"speed": 1.5, "emotion": "storytelling"}
	opts := audio.TTSOptions{Voice: "binh", Params: params}
	_, err := p.Synthesize(context.Background(), "x", opts)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := params["speed"]; !ok || v != 1.5 {
		t.Errorf("opts.Params.speed mutated: %v", params)
	}
	if v, ok := params["emotion"]; !ok || v != "storytelling" {
		t.Errorf("opts.Params.emotion mutated: %v", params)
	}
}

func TestSynthesize_ParamsAppliedToRequest(t *testing.T) {
	var got synthRequest
	p, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = readBody(t, r)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ok"))
	})
	opts := audio.TTSOptions{
		Voice:  "binh",
		Format: "opus",
		Params: map[string]any{"speed": 1.5, "emotion": "storytelling"},
	}
	out, err := p.Synthesize(context.Background(), "x", opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Speed != 1.5 || got.Emotion != "storytelling" {
		t.Errorf("params not applied: %+v", got)
	}
	if got.Format != "opus" {
		t.Errorf("format = %q", got.Format)
	}
	if out.Extension != "ogg" || !strings.Contains(out.MimeType, "audio") {
		t.Errorf("opus → ext=%q mime=%q", out.Extension, out.MimeType)
	}
}

func TestProvider_NameAndCapabilities(t *testing.T) {
	p := NewProvider(Config{})
	if p.Name() != "vieneu" {
		t.Errorf("Name() = %q", p.Name())
	}
	caps := p.Capabilities()
	if caps.Provider != "vieneu" || caps.RequiresAPIKey {
		t.Errorf("caps = %+v", caps)
	}
	if caps.Voices != nil {
		t.Error("voices should be nil (dynamic via ListVoices)")
	}
}
