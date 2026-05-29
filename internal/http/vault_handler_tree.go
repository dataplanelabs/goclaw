package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// handleVaultTree returns immediate children (files + virtual folders) under
// a given path prefix for lazy-loading the vault sidebar tree.
func (h *VaultHandler) handleVaultTree(w http.ResponseWriter, r *http.Request) {
	tenantID := store.TenantIDFromContext(r.Context())
	path := r.URL.Query().Get("path")

	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		slog.Warn("security.vault_tree_traversal", "path", path)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path"})
		return
	}

	opts := store.VaultTreeOptions{
		Path:     path,
		AgentID:  r.URL.Query().Get("agent_id"),
		Scope:    r.URL.Query().Get("scope"),
		DocTypes: splitCSV(r.URL.Query().Get("doc_type")),
	}
	if teamID := r.URL.Query().Get("team_id"); teamID != "" {
		opts.TeamID = &teamID
	}

	if opts.TeamID == nil && !store.IsOwnerRole(r.Context()) {
		if ids := h.userAccessibleTeamIDs(r.Context()); len(ids) > 0 {
			opts.TeamIDs = ids
		} else {
			empty := ""
			opts.TeamID = &empty
		}
	}
	if opts.TeamID != nil && *opts.TeamID != "" {
		if !h.validateTeamMembership(r.Context(), w, *opts.TeamID) {
			return
		}
	}

	entries, err := h.store.ListTreeEntries(r.Context(), tenantID.String(), opts)
	if err != nil {
		slog.Warn("vault.tree failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tree"})
		return
	}
	if entries == nil {
		entries = []store.VaultTreeEntry{}
	}

	// Label id / group folders with their human display name (display-only;
	// stored paths unchanged). Mirrors the storage tree enrichment.
	if h.contacts != nil {
		ids := make([]string, 0, len(entries))
		for i := range entries {
			if entries[i].IsDir {
				if id := contactIDFromFolder(entries[i].Name); id != "" {
					ids = append(ids, id)
				}
			}
		}
		if len(ids) > 0 {
			if byID, err := h.contacts.GetContactsBySenderIDs(r.Context(), ids); err == nil {
				for i := range entries {
					c, ok := byID[contactIDFromFolder(entries[i].Name)]
					if !ok || c.DisplayName == nil || *c.DisplayName == "" {
						continue
					}
					entries[i].Label = *c.DisplayName
					if c.PeerKind != nil {
						entries[i].Kind = *c.PeerKind
					}
				}
			} else {
				slog.Debug("vault: contact label lookup failed", "error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
