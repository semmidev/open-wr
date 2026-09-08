package waitingroom_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/semmidev/wr/internal/config"
	"github.com/semmidev/wr/internal/waitingroom"
)

func TestRenderDefaultTemplate(t *testing.T) {
	rc := config.RoomConfig{
		ID:             "flash-sale",
		Name:           "Flash Sale Tiket",
		Description:    "Harap tunggu giliran untuk melakukan checkout.",
		QueueingMethod: "fifo",
	}

	var buf bytes.Buffer
	err := waitingroom.RenderDefault(&buf, rc, 5, 50, 10, 120)
	if err != nil {
		t.Fatalf("RenderDefault failed: %v", err)
	}

	html := buf.String()

	// Assert key components and text elements exist in rendered output
	expectedStrings := []string{
		"Waiting Room — Flash Sale Tiket",
		"Open Waiting Room",
		"Flash Sale Tiket",
		"Harap tunggu giliran untuk melakukan checkout.",
		"#6", // pos + 1
		"50", // TotalQueued
		"10", // ActiveCount
		"FIFO",
		"120 detik",
		"__owr_flash-sale",
		"var roomId = \"flash-sale\"",
	}

	for _, str := range expectedStrings {
		if !strings.Contains(html, str) {
			t.Errorf("expected HTML to contain %q, but it was missing", str)
		}
	}
}
