package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/semmidev/wr/internal/waitingroom"
)

// AdminHandler exposes room management endpoints.
// Routes are protected by AdminAuthMiddleware when an API key is configured.
type AdminHandler struct {
	svc *waitingroom.Service
}

// NewAdminHandler creates an AdminHandler backed by the given service.
func NewAdminHandler(svc *waitingroom.Service) *AdminHandler {
	return &AdminHandler{svc: svc}
}

// Routes returns a chi.Router with all admin routes mounted.
func (h *AdminHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.listRooms)
	r.Get("/{id}/stats", h.roomStats)
	return r
}

// listRooms returns the list of all configured rooms as JSON.
func (h *AdminHandler) listRooms(w http.ResponseWriter, r *http.Request) {
	rooms := h.svc.ListRooms()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rooms); err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to encode response")
	}
}

// roomStats returns live stats for a single room.
func (h *AdminHandler) roomStats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	stats, err := h.svc.Stats(r.Context(), id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to encode response")
	}
}

func (h *AdminHandler) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
