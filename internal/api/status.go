package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/semmidev/wr/internal/cookie"
	"github.com/semmidev/wr/internal/waitingroom"
)

// StatusHandler serves the Open WR status polling endpoint.
// Clients (waiting-room JS) poll this endpoint to check if they have been admitted.
type StatusHandler struct {
	svc        *waitingroom.Service
	signer     *cookie.Signer
	corsOrigin string // value of Access-Control-Allow-Origin header
}

// NewStatusHandler creates a StatusHandler.
// corsOrigin should be a specific origin string or "*" (never empty).
func NewStatusHandler(svc *waitingroom.Service, signer *cookie.Signer, corsOrigin string) *StatusHandler {
	if corsOrigin == "" {
		corsOrigin = "*"
	}
	return &StatusHandler{svc: svc, signer: signer, corsOrigin: corsOrigin}
}

// ServeHTTP handles GET /__owr/waiting-room/status?room_id=xxx
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight.
	h.setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		roomID = r.URL.Query().Get("roomId") // alternate casing for compatibility
	}
	if roomID == "" {
		h.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	rcfg, ok := h.svc.GetRoom(roomID)
	if !ok {
		h.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	cookieName := h.signer.CookieName(rcfg.CookieName, roomID)
	c, err := r.Cookie(cookieName)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "missing session cookie")
		return
	}

	payload, verifyErr := h.signer.Verify(c.Value)
	if verifyErr != nil {
		if errors.Is(verifyErr, cookie.ErrExpiredToken) {
			h.writeError(w, http.StatusUnauthorized, "session expired")
		} else {
			h.writeError(w, http.StatusForbidden, "invalid session cookie")
		}
		return
	}

	pos, total, active, eta, isActive, err := h.svc.GetStatus(r.Context(), roomID, payload.ID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to get status")
		return
	}

	resp := map[string]interface{}{
		"room_id":        roomID,
		"is_active":      isActive,
		"position":       pos,
		"total_queued":   total,
		"active_count":   active,
		"estimated_wait": eta,
		"status":         payload.Status,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// Response partially written; nothing to do.
		_ = err
	}
}

func (h *StatusHandler) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", h.corsOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func (h *StatusHandler) writeError(w http.ResponseWriter, code int, msg string) {
	h.setCORSHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
