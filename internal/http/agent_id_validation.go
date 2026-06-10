package http

import (
	"net/http"

	"github.com/google/uuid"
)

func requireAgentUUID(w http.ResponseWriter, agentID string) bool {
	if _, err := uuid.Parse(agentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return false
	}
	return true
}
