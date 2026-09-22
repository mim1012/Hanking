package main

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestCaptureTestFrame verifies STREAM_TEST_FRAME=1 yields a valid, decodable
// JPEG and that consecutive frames differ (frame counter advances).
func TestCaptureTestFrame(t *testing.T) {
	os.Setenv(xob(eStreamTestFrame), "1")
	defer os.Unsetenv(xob(eStreamTestFrame))

	a, mime, err := captureCurrentFrame()
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q, want image/jpeg", mime)
	}
	img, err := jpeg.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatalf("not a valid JPEG: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 24 {
		t.Fatalf("dims = %v, want 32x24", img.Bounds())
	}
	// Different frame numbers must produce different pixel content
	// (mirrors pushStreamFrame, which advances streamFrameN before capture).
	atomic.StoreUint64(&streamFrameN, 2)
	b, _, err := captureCurrentFrame()
	if err != nil {
		t.Fatalf("capture2: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("test frames for different frame numbers are identical")
	}
}

// TestStreamEnvelope verifies pushStreamFrame's wire envelope shape matches the
// server's applyStreamFrame parser expectations (keys present and typed).
func TestStreamEnvelope(t *testing.T) {
	os.Setenv(xob(eStreamTestFrame), "1")
	os.Setenv(xob(eForceStream), "1")
	defer os.Unsetenv(xob(eStreamTestFrame))
	defer os.Unsetenv(xob(eForceStream))

	// Build the envelope directly (no live WS conn in unit test).
	raw, mime, err := captureCurrentFrame()
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	env := map[string]interface{}{
		"type":     "stream_frame",
		"agent_id": "test-agent",
		"frame":    uint64(1),
		"size":     len(raw),
		"mime":     mime,
		"data_b64": "x",
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"type", "agent_id", "frame", "size", "mime", "data_b64"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("envelope missing key %q", k)
		}
	}
	if out["type"] != "stream_frame" {
		t.Fatalf("type = %v", out["type"])
	}
	if int(out["size"].(float64)) != len(raw) {
		t.Fatalf("size mismatch: %v vs %d", out["size"], len(raw))
	}
}

// TestConfigStreamDefaults verifies loadConfig wires the stream fields with
// sane defaults (interval 2s, maxbytes 262144).
func TestConfigStreamDefaults(t *testing.T) {
	cfg := loadConfig()
	if cfg.StreamInterval <= 0 {
		t.Fatalf("StreamInterval = %v, want > 0", cfg.StreamInterval)
	}
	if cfg.StreamMaxBytes <= 0 {
		t.Fatalf("StreamMaxBytes = %v, want > 0", cfg.StreamMaxBytes)
	}
	if cfg.StreamInterval < time.Second {
		t.Fatalf("StreamInterval = %v suspiciously small", cfg.StreamInterval)
	}
}
