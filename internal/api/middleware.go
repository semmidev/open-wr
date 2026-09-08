// Package api contains HTTP handlers and middleware for the open waiting room edge.
package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/cookie"
	"github.com/semmidev/wr/internal/room"
	"github.com/semmidev/wr/internal/waitingroom"
)

// OriginHandler abstracts the upstream target — either a real reverse proxy or a demo handler.
type OriginHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// WaitingRoomHandlerConfig configures the main waiting-room catch-all handler.
type WaitingRoomHandlerConfig struct {
	Rooms         []config.RoomConfig
	Service       *waitingroom.Service
	Store         room.Store
	Signer        *cookie.Signer
	OriginProxy   OriginHandler // nil falls back to DemoOrigin
	DemoOrigin    OriginHandler
	SecureCookies bool
}

// WaitingRoomHandler is the main HTTP handler that intercepts all requests,
// matches them against room patterns, and either queues or proxies them.
type WaitingRoomHandler struct {
	cfg     WaitingRoomHandlerConfig
	matcher *roomMatcher
}

// NewWaitingRoomHandler constructs and returns a WaitingRoomHandler.
func NewWaitingRoomHandler(cfg WaitingRoomHandlerConfig) *WaitingRoomHandler {
	return &WaitingRoomHandler{
		cfg:     cfg,
		matcher: newRoomMatcher(cfg.Rooms),
	}
}

func (h *WaitingRoomHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := h.matcher.Match(r.URL.Path, r.Host)
	if roomID == "" {
		h.serveOrigin(w, r)
		return
	}

	rcfg, ok := h.cfg.Service.GetRoom(roomID)
	if !ok {
		h.serveOrigin(w, r)
		return
	}

	cookieName := h.cfg.Signer.CookieName(rcfg.CookieName, roomID)
	var cookieVal string
	if c, err := r.Cookie(cookieName); err == nil {
		cookieVal = c.Value
	}

	ip := realIP(r)
	decision, err := h.cfg.Service.CheckRequest(r.Context(), roomID, cookieVal, ip, r.UserAgent())
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	switch decision.Action {
	case waitingroom.ActionAllow:
		h.handleAllow(w, r, rcfg, decision, cookieName)
	case waitingroom.ActionQueue:
		h.handleQueue(w, r, rcfg, decision, cookieName)
	default:
		// ActionBypass or unknown — proxy directly.
		h.serveOrigin(w, r)
	}
}

func (h *WaitingRoomHandler) handleAllow(
	w http.ResponseWriter, r *http.Request,
	rcfg config.RoomConfig, decision waitingroom.Decision, cookieName string,
) {
	if decision.CookiePayload != nil {
		payload := *decision.CookiePayload
		payload.Status = cookie.StatusActive
		if payload.IssuedAt == 0 {
			payload.IssuedAt = time.Now().Unix()
		}
		payload.ExpAt = time.Now().Add(time.Duration(rcfg.SessionDurationMinutes) * time.Minute).Unix()

		if signed, err := h.cfg.Signer.Sign(payload); err == nil {
			http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure flag dynamically controlled via h.cfg.SecureCookies
				Name:     cookieName,
				Value:    signed,
				Path:     "/",
				HttpOnly: true,
				Secure:   h.cfg.SecureCookies,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   rcfg.SessionDurationMinutes * 60,
			})
		}
	}
	h.serveOrigin(w, r)
}

func (h *WaitingRoomHandler) handleQueue(
	w http.ResponseWriter, r *http.Request,
	rcfg config.RoomConfig, decision waitingroom.Decision, cookieName string,
) {
	// Issue or refresh the queued cookie.
	if decision.IsNewQueue || decision.CookiePayload != nil {
		if signed, err := h.cfg.Signer.Sign(*decision.CookiePayload); err == nil {
			http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure flag dynamically controlled via h.cfg.SecureCookies
				Name:     cookieName,
				Value:    signed,
				Path:     "/",
				HttpOnly: true,
				Secure:   h.cfg.SecureCookies,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   1800, // 30 min queue cookie TTL
			})
		}
	}

	// JSON response for API clients.
	if rcfg.JSONResponseEnabled || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w,
			`{"status":"queued","room_id":%q,"position":%d,"total_queued":%d,"estimated_wait":%d}`,
			rcfg.ID, decision.Position, decision.TotalQueued, decision.EstimatedWait,
		)
		return
	}

	// HTML waiting page served entirely from edge — never touches origin.
	active, _ := h.cfg.Store.ActiveCount(r.Context(), rcfg.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = waitingroom.RenderDefault(w, rcfg, decision.Position, decision.TotalQueued, active, decision.EstimatedWait)
}

func (h *WaitingRoomHandler) serveOrigin(w http.ResponseWriter, r *http.Request) {
	if h.cfg.OriginProxy != nil {
		h.cfg.OriginProxy.ServeHTTP(w, r)
	} else {
		h.cfg.DemoOrigin.ServeHTTP(w, r)
	}
}

// realIP extracts the client IP from standard proxy headers, falling back to RemoteAddr.
func realIP(r *http.Request) string {
	// X-Forwarded-For may contain a list; take the first entry.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Strip port from RemoteAddr.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AdminAuthMiddleware rejects requests that do not carry the correct API key in the
// Authorization header (Bearer <key>) or X-Admin-Key header.
// If apiKey is empty, all requests are allowed (development mode).
func AdminAuthMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			// No key configured — development mode, allow all.
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := ""
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}
			if provided == "" {
				provided = r.Header.Get("X-Admin-Key")
			}
			if provided != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PoweredByMiddleware injects the X-Powered-By response header.
func PoweredByMiddleware(value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Powered-By", value)
			next.ServeHTTP(w, r)
		})
	}
}

// roomMatcher pre-compiles room path patterns into an efficient lookup structure.
type roomMatcher struct {
	exact    map[string]string       // exact path → roomID
	prefixes []prefixEntry           // sorted (longest-first) prefix → roomID
	byHost   map[string]*roomMatcher // optional: host-scoped matchers
}

type prefixEntry struct {
	prefix string
	roomID string
}

// newRoomMatcher builds the matcher at startup to avoid O(n) scan per request.
func newRoomMatcher(rooms []config.RoomConfig) *roomMatcher {
	m := &roomMatcher{
		exact:  make(map[string]string),
		byHost: make(map[string]*roomMatcher),
	}

	// Separate rooms by host.
	hostRooms := make(map[string][]config.RoomConfig)
	var globalRooms []config.RoomConfig
	for _, r := range rooms {
		if !r.Enabled {
			continue
		}
		if r.Host != "" {
			hostRooms[r.Host] = append(hostRooms[r.Host], r)
		} else {
			globalRooms = append(globalRooms, r)
		}
	}

	m.addRooms(globalRooms)
	for host, hrs := range hostRooms {
		hm := &roomMatcher{exact: make(map[string]string)}
		hm.addRooms(hrs)
		m.byHost[host] = hm
	}

	return m
}

func (m *roomMatcher) addRooms(rooms []config.RoomConfig) {
	for _, r := range rooms {
		if strings.HasSuffix(r.Path, "/*") {
			prefix := strings.TrimSuffix(r.Path, "/*")
			m.prefixes = append(m.prefixes, prefixEntry{prefix: prefix, roomID: r.ID})
		} else {
			m.exact[r.Path] = r.ID
		}
	}
	// Sort longest prefix first for most-specific match.
	for i := 0; i < len(m.prefixes)-1; i++ {
		for j := i + 1; j < len(m.prefixes); j++ {
			if len(m.prefixes[j].prefix) > len(m.prefixes[i].prefix) {
				m.prefixes[i], m.prefixes[j] = m.prefixes[j], m.prefixes[i]
			}
		}
	}
}

// Match returns the room ID that best matches path+host, or "" if no match.
func (m *roomMatcher) Match(path, host string) string {
	// Host-scoped rooms take priority.
	if host != "" {
		if hm, ok := m.byHost[host]; ok {
			if id := hm.matchPath(path); id != "" {
				return id
			}
		}
	}
	return m.matchPath(path)
}

func (m *roomMatcher) matchPath(path string) string {
	if id, ok := m.exact[path]; ok {
		return id
	}
	for _, pe := range m.prefixes {
		if strings.HasPrefix(path, pe.prefix) {
			return pe.roomID
		}
	}
	return ""
}
