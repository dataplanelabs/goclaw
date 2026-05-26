package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/audio"
	"github.com/nextlevelbuilder/goclaw/internal/audio/vieneu/refstore"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	vieneuMaxUploadBytes  = 5 * 1024 * 1024
	vieneuMaxClonedVoices = 10
)

// TTSVieneuVoicesHandler exposes the VieNeu cloned-voice CRUD surface.
type TTSVieneuVoicesHandler struct {
	clonedVoices  store.VieneuClonedVoicesStore
	refStore      *refstore.Store
	daemonBaseURL string
	tenants       store.TenantStore
	voiceCache    *audio.VoiceCache
}

func NewTTSVieneuVoicesHandler(s store.VieneuClonedVoicesStore, rs *refstore.Store, daemonBaseURL string, tenants store.TenantStore, voiceCache *audio.VoiceCache) *TTSVieneuVoicesHandler {
	return &TTSVieneuVoicesHandler{
		clonedVoices:  s,
		refStore:      rs,
		daemonBaseURL: daemonBaseURL,
		tenants:       tenants,
		voiceCache:    voiceCache,
	}
}

func (h *TTSVieneuVoicesHandler) invalidateCache(tenantID uuid.UUID) {
	if h.voiceCache != nil {
		h.voiceCache.Invalidate(tenantID)
	}
}

func (h *TTSVieneuVoicesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tts/vieneu/voices", requireAuth(permissions.RoleViewer, h.handleList))
	mux.HandleFunc("POST /v1/tts/vieneu/voices", requireAuth(permissions.RoleAdmin, h.requireTenantAdmin(h.handleUpload)))
	mux.HandleFunc("DELETE /v1/tts/vieneu/voices/{voiceID}", requireAuth(permissions.RoleAdmin, h.requireTenantAdmin(h.handleDelete)))
}

func (h *TTSVieneuVoicesHandler) requireTenantAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireTenantAdmin(w, r, h.tenants) {
			return
		}
		next(w, r)
	}
}

type vieneuVoiceResponse struct {
	VoiceID   string    `json:"voice_id"`
	Name      string    `json:"name"`
	RefText   string    `json:"ref_text"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *TTSVieneuVoicesHandler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		http.Error(w, `{"error":"tenant context required"}`, http.StatusBadRequest)
		return
	}
	voices, err := h.clonedVoices.List(ctx, tid)
	if err != nil {
		slog.Error("vieneu voices list", "err", err)
		http.Error(w, `{"error":"list failed"}`, http.StatusInternalServerError)
		return
	}
	out := make([]vieneuVoiceResponse, 0, len(voices))
	for _, v := range voices {
		out = append(out, vieneuVoiceResponse{
			VoiceID:   v.VoiceID,
			Name:      v.Name,
			RefText:   v.RefText,
			CreatedAt: v.CreatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"voices": out})
}

func (h *TTSVieneuVoicesHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		http.Error(w, `{"error":"tenant context required"}`, http.StatusBadRequest)
		return
	}
	locale := store.LocaleFromContext(ctx)

	// Enforce upload cap before reading the body.
	r.Body = http.MaxBytesReader(w, r.Body, vieneuMaxUploadBytes+8192)
	if err := r.ParseMultipartForm(vieneuMaxUploadBytes); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusBadRequest)
		return
	}
	refText := r.FormValue("ref_text")
	name := r.FormValue("name")
	if refText == "" {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, i18n.T(locale, i18n.MsgVieneuRefTextRequired)), http.StatusBadRequest)
		return
	}
	if name == "" {
		name = "Cloned voice"
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, `{"error":"audio file required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > vieneuMaxUploadBytes {
		http.Error(w, `{"error":"audio body > 5 MB"}`, http.StatusBadRequest)
		return
	}

	audioBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, `{"error":"read audio"}`, http.StatusBadRequest)
		return
	}

	// Quota check.
	count, err := h.clonedVoices.Count(ctx, tid)
	if err != nil {
		slog.Error("vieneu voices count", "err", err)
		http.Error(w, `{"error":"count failed"}`, http.StatusInternalServerError)
		return
	}
	if count >= vieneuMaxClonedVoices {
		msg := i18n.T(locale, i18n.MsgVieneuMaxClonedVoices, vieneuMaxClonedVoices)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
		return
	}

	// Validate via daemon /clone-preview before persisting.
	if h.daemonBaseURL != "" {
		if err := vieneuClonePreview(ctx, h.daemonBaseURL, audioBytes, refText); err != nil {
			msg := i18n.T(locale, i18n.MsgTtsVieneuRefAudioInvalid, err.Error())
			http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusBadRequest)
			return
		}
	}

	voiceUUID := uuid.New()
	voiceID := "cloned:" + voiceUUID.String()

	if _, err := h.refStore.Save(tid, voiceUUID.String(), bytes.NewReader(audioBytes)); err != nil {
		slog.Error("vieneu refstore save", "err", err)
		http.Error(w, `{"error":"refstore save failed"}`, http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	rec := store.VieneuClonedVoice{
		ID:        voiceUUID,
		TenantID:  tid,
		VoiceID:   voiceID,
		RefText:   refText,
		Name:      name,
		CreatedAt: now,
	}
	if err := h.clonedVoices.Insert(ctx, rec); err != nil {
		// Best-effort rollback of the on-disk file.
		_ = h.refStore.Delete(tid, voiceUUID.String())
		slog.Error("vieneu cloned voice insert", "err", err)
		http.Error(w, `{"error":"insert failed"}`, http.StatusInternalServerError)
		return
	}
	h.invalidateCache(tid)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(vieneuVoiceResponse{
		VoiceID:   voiceID,
		Name:      name,
		RefText:   refText,
		CreatedAt: now,
	})
}

func (h *TTSVieneuVoicesHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		http.Error(w, `{"error":"tenant context required"}`, http.StatusBadRequest)
		return
	}
	locale := store.LocaleFromContext(ctx)
	voiceID := r.PathValue("voiceID")
	if voiceID == "" {
		http.Error(w, `{"error":"voice_id required"}`, http.StatusBadRequest)
		return
	}

	row, err := h.clonedVoices.Get(ctx, tid, voiceID)
	if err != nil {
		slog.Error("vieneu cloned voice get", "err", err)
		http.Error(w, `{"error":"get failed"}`, http.StatusInternalServerError)
		return
	}
	if row == nil {
		msg := i18n.T(locale, i18n.MsgVieneuClonedVoiceNotFound, voiceID)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, msg), http.StatusNotFound)
		return
	}

	if err := h.clonedVoices.Delete(ctx, tid, voiceID); err != nil {
		slog.Error("vieneu cloned voice delete", "err", err)
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	_ = h.refStore.Delete(tid, row.ID.String())
	h.invalidateCache(tid)
	w.WriteHeader(http.StatusNoContent)
}

func vieneuClonePreview(ctx context.Context, baseURL string, audioBytes []byte, refText string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("ref_text", refText); err != nil {
		return err
	}
	part, err := mw.CreateFormFile("audio", "reference.wav")
	if err != nil {
		return err
	}
	if _, err := part.Write(audioBytes); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, baseURL+"/clone-preview", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		return errors.New(truncateBytes(msg, 200))
	}
	return nil
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
