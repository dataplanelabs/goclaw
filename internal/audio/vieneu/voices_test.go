package vieneu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestListVoices_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"voices":[
			{"id":"truc_ly","name":"Trúc Ly","language":"vi","gender":"female","accent":"north"},
			{"id":"binh","name":"Bình","language":"vi","gender":"male","accent":"north"}
		]}`))
	}))
	t.Cleanup(srv.Close)
	p := NewProvider(Config{Endpoint: srv.URL, TimeoutMs: 5000})
	voices, err := p.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("voices = %d, want 2", len(voices))
	}
	if voices[0].ID != "truc_ly" || voices[0].Category != "premade" {
		t.Errorf("voice[0] = %+v", voices[0])
	}
	if voices[0].Labels["language"] != "vi" || voices[0].Labels["gender"] != "female" {
		t.Errorf("labels = %v", voices[0].Labels)
	}
}

func TestListVoices_Caches(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"voices":[{"id":"x","name":"X","language":"vi"}]}`))
	}))
	t.Cleanup(srv.Close)
	p := NewProvider(Config{Endpoint: srv.URL, TimeoutMs: 5000})
	for range 3 {
		_, err := p.ListVoices(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (cached)", calls.Load())
	}
}

func TestListVoices_DaemonError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	p := NewProvider(Config{Endpoint: srv.URL, TimeoutMs: 5000})
	_, err := p.ListVoices(context.Background())
	if !errors.Is(err, ErrVoicesFetchFailed) {
		t.Errorf("err = %v, want ErrVoicesFetchFailed", err)
	}
}

func TestListVoices_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)
	p := NewProvider(Config{Endpoint: srv.URL, TimeoutMs: 5000})
	_, err := p.ListVoices(context.Background())
	if !errors.Is(err, ErrVoicesFetchFailed) {
		t.Errorf("err = %v", err)
	}
}
