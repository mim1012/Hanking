// ecdhe.go — Ephemeral Elliptic-Curve Diffie-Hellman (X25519) session-key
// establishment + HKDF-SHA256 derivation (stdlib only, no external deps).
//
// WHY: prior builds baked the AEAD PSK (and its XOR mask) INSIDE the binary, so
// a static reverse-engineering pass could recover the C2 session key with a
// single XOR+sha256. With ECDHE the per-session key is derived at RUNTIME from
// an ephemeral keypair + peer public key; it never exists in the binary and is
// gone when the process exits. This removes the recoverable-secret weakness.
package main

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// eSessionInfo is the HKDF "info" domain-separation label. It is PUBLIC protocol
// metadata (like a version string) — NOT a secret.
const eSessionInfo = "c2lab-v2|ecdh-session"

// ecdheNewPrivate returns a fresh ephemeral X25519 private key.
func ecdheNewPrivate() (*ecdh.PrivateKey, error) {
	return ecdh.X25519().GenerateKey(rand.Reader)
}

// ecdhePubBytes returns the raw public key bytes for an ephemeral key.
func ecdhePubBytes(priv *ecdh.PrivateKey) []byte {
	return priv.PublicKey().Bytes()
}

// ecdhePubB64 returns the public key as base64 (register/reply wire format).
func ecdhePubB64(priv *ecdh.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(ecdhePubBytes(priv))
}

// ecdhePubFromB64 decodes a base64 public key from the wire.
func ecdhePubFromB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// ecdheDerive computes the 32-byte session key from our private key and the
// peer public key (X25519 ECDH -> HKDF-SHA256).
func ecdheDerive(priv *ecdh.PrivateKey, peerPubBytes []byte) ([]byte, error) {
	pub, err := ecdh.X25519().NewPublicKey(peerPubBytes)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, err
	}
	return hkdfSHA256(shared, nil, []byte(eSessionInfo), 32), nil
}

// hkdfSHA256 derives keyLen bytes with HKDF-SHA256 (RFC 5869).
func hkdfSHA256(secret, salt, info []byte, keyLen int) []byte {
	key, err := hkdf.Key(sha256.New, secret, salt, string(info), keyLen)
	if err != nil {
		return nil
	}
	return key
}
