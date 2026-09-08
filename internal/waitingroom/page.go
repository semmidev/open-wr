package waitingroom

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"os"
	"sync"

	"github.com/semmidev/wr/internal/config"
)

//go:embed templates
var tmplFS embed.FS

// defaultTmpl is the compiled default waiting-room template.
var defaultTmpl *template.Template

func init() {
	data, err := tmplFS.ReadFile("templates/waiting.html")
	if err != nil {
		panic(fmt.Sprintf("waitingroom: cannot read embedded template: %v", err))
	}
	defaultTmpl = template.Must(template.New("waiting").Parse(string(data)))
}

// PageData is the template data passed to the waiting room HTML template.
type PageData struct {
	RoomName         string
	RoomDescription  string
	RoomID           string
	Position         int64
	TotalQueued      int64
	ActiveCount      int64
	EstimatedWait    int64 // seconds
	EstimatedWaitMin int64 // ceiling minutes
	RefreshInterval  int   // seconds between meta-refresh (used as JS polling base too)
	QueueingMethod   string
}

// templateCache caches parsed custom templates per room to avoid re-parsing on every request.
var (
	tmplCacheMu sync.RWMutex
	tmplCache   = make(map[string]*template.Template) // key: CustomPageTemplate path
)

// RenderDefault renders the waiting room page to w.
// It uses a room's CustomPageTemplate if set (cached after first parse), or the embedded default.
func RenderDefault(w io.Writer, rc config.RoomConfig, pos, total, active, eta int64) error {
	data := PageData{
		RoomName:         rc.Name,
		RoomDescription:  rc.Description,
		RoomID:           rc.ID,
		Position:         pos + 1, // 1-indexed for display
		TotalQueued:      total,
		ActiveCount:      active,
		EstimatedWait:    eta,
		EstimatedWaitMin: (eta + 59) / 60,
		RefreshInterval:  5,
		QueueingMethod:   rc.QueueingMethod,
	}

	tmpl, err := resolveTemplate(rc.CustomPageTemplate)
	if err != nil {
		// Fallback to default if custom template fails.
		tmpl = defaultTmpl
	}
	return tmpl.Execute(w, data)
}

// resolveTemplate returns a compiled template for the given path.
// Results are cached; subsequent calls with the same path return the cached version.
// If path is empty, the embedded default template is returned.
func resolveTemplate(path string) (*template.Template, error) {
	if path == "" {
		return defaultTmpl, nil
	}

	// Fast path: already cached.
	tmplCacheMu.RLock()
	if t, ok := tmplCache[path]; ok {
		tmplCacheMu.RUnlock()
		return t, nil
	}
	tmplCacheMu.RUnlock()

	// Slow path: parse and cache.
	b, err := os.ReadFile(path) // #nosec G304 -- path is configured by operator in room config
	if err != nil {
		return nil, fmt.Errorf("custom template %q: %w", path, err)
	}
	t, err := template.New("custom").Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("custom template %q parse error: %w", path, err)
	}

	tmplCacheMu.Lock()
	tmplCache[path] = t
	tmplCacheMu.Unlock()

	return t, nil
}
