package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/semmidev/wr/internal/config"
)

func TestLoadConfig_ValidYAML(t *testing.T) {
	content := `
server:
  listen: ":9090"
  origin: "http://backend:3000"
  cookie_secret: "12345678901234567890123456789012"
  redis_enabled: false
rooms:
  - id: "test-room"
    name: "Test Room"
    path: "/test/*"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write tmp config: %v", err)
	}

	cfg, err := config.Load(tmpFile)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.Server.Listen != ":9090" {
		t.Errorf("expected listen :9090, got %s", cfg.Server.Listen)
	}
	if cfg.Server.Origin != "http://backend:3000" {
		t.Errorf("expected origin http://backend:3000, got %s", cfg.Server.Origin)
	}
	if len(cfg.Rooms) != 1 || cfg.Rooms[0].ID != "test-room" {
		t.Errorf("expected 1 room with ID test-room, got %v", cfg.Rooms)
	}
}

func TestLoadConfig_ShortSecretFails(t *testing.T) {
	content := `
server:
  cookie_secret: "short"
rooms:
  - id: "test"
    path: "/test/*"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")
	_ = os.WriteFile(tmpFile, []byte(content), 0600)

	_, err := config.Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for short cookie secret, got nil")
	}
}
