package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/semmidev/wr/internal/proxy"
)

func TestDemoOriginHandler(t *testing.T) {
	handler := proxy.DemoOriginHandler()
	req := httptest.NewRequest(http.MethodGet, "/flash/sale", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Origin Demo Server") {
		t.Errorf("expected body to contain title, got %s", body)
	}
	if !strings.Contains(body, "/flash/sale") {
		t.Errorf("expected body to contain path /flash/sale, got %s", body)
	}
}
