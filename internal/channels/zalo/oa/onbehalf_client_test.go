package oa

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newOnBehalfClient(t *testing.T, srv *httptest.Server) *OnBehalfClient {
	t.Helper()
	c := NewClient(5 * time.Second)
	c.apiBase = srv.URL
	return NewOnBehalfClient(c, func() string { return "test-token" })
}

func TestOnBehalfClient_ListRecentChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/listrecentchat") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("access_token"); got != "test-token" {
			t.Errorf("missing access_token header: %q", got)
		}
		_, _ = io.WriteString(w, `{"error":0,"message":"ok","data":[
			{"message_id":"m100","from_id":"u1","to_id":"oa-self","type":"text","message":"hi","time":1735041000123,"from_display_name":"Alice"},
			{"message_id":"m200","from_id":"u2","to_id":"oa-self","type":"text","message":"ya","time":1735041000456}
		]}`)
	}))
	defer srv.Close()

	cl := newOnBehalfClient(t, srv)
	entries, err := cl.ListRecentChat(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("ListRecentChat: %v", err)
	}
	if len(entries) != 2 || entries[0].UID != "u1" || entries[1].LastMsgID != "m200" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestOnBehalfClient_GetConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/conversation") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"error":0,"data":[
			{"message_id":"m1","from_id":"oa-self","to_id":"u1","type":"text","message":"hi sir","time":1735041000000},
			{"message_id":"m2","from_id":"u1","to_id":"oa-self","type":"text","message":"who?","time":1735041001000}
		]}`)
	}))
	defer srv.Close()

	cl := newOnBehalfClient(t, srv)
	msgs, err := cl.GetConversation(context.Background(), "u1", 0, 20)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Text != "hi sir" || msgs[1].SrcID != "u1" {
		t.Fatalf("unexpected msgs: %+v", msgs)
	}
}

func TestOnBehalfClient_RefreshTokenDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":-118,"message":"invalid grant"}`)
	}))
	defer srv.Close()

	cl := newOnBehalfClient(t, srv)
	_, err := cl.ListRecentChat(context.Background(), 0, 10)
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
	}
}

func TestOnBehalfClient_EmptyToken(t *testing.T) {
	c := NewClient(5 * time.Second)
	cl := NewOnBehalfClient(c, func() string { return "" })
	_, err := cl.ListRecentChat(context.Background(), 0, 10)
	if !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected ErrInvalidRefreshToken for empty token, got %v", err)
	}
}

func TestOnBehalfClient_GetConversationEmptyUID(t *testing.T) {
	c := NewClient(5 * time.Second)
	cl := NewOnBehalfClient(c, func() string { return "tok" })
	_, err := cl.GetConversation(context.Background(), "", 0, 10)
	if err == nil {
		t.Fatal("expected error for empty uid")
	}
}
