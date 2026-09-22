package main

import (
	"bytes"
	"testing"
)

// TestEcdheBothSidesDeriveSameKey: two ephemeral keypairs must agree on the
// same 32-byte session key (X25519 ECDH is symmetric).
func TestEcdheBothSidesDeriveSameKey(t *testing.T) {
	a, err := ecdheNewPrivate()
	if err != nil {
		t.Fatalf("a keygen: %v", err)
	}
	b, err := ecdheNewPrivate()
	if err != nil {
		t.Fatalf("b keygen: %v", err)
	}
	ka, err := ecdheDerive(a, ecdhePubBytes(b))
	if err != nil {
		t.Fatalf("a derive: %v", err)
	}
	kb, err := ecdheDerive(b, ecdhePubBytes(a))
	if err != nil {
		t.Fatalf("b derive: %v", err)
	}
	if len(ka) != 32 || len(kb) != 32 {
		t.Fatalf("key length != 32: %d/%d", len(ka), len(kb))
	}
	if !bytes.Equal(ka, kb) {
		t.Fatalf("session keys mismatch")
	}
}

// TestEcdheDistinctKeys: different sessions must yield different keys.
func TestEcdheDistinctKeys(t *testing.T) {
	a, _ := ecdheNewPrivate()
	b1, _ := ecdheNewPrivate()
	b2, _ := ecdheNewPrivate()
	k1, _ := ecdheDerive(a, ecdhePubBytes(b1))
	k2, _ := ecdheDerive(a, ecdhePubBytes(b2))
	if bytes.Equal(k1, k2) {
		t.Fatalf("distinct peers produced identical keys")
	}
}

// TestEcdhePubRoundtrip: base64 wire format roundtrips cleanly.
func TestEcdhePubRoundtrip(t *testing.T) {
	a, _ := ecdheNewPrivate()
	enc := ecdhePubB64(a)
	dec, err := ecdhePubFromB64(enc)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}
	if !bytes.Equal(dec, ecdhePubBytes(a)) {
		t.Fatalf("pub roundtrip mismatch")
	}
}

// TestHkdfSHA256KnownVector: RFC 5869 test case 1 (SHA-256).
func TestHkdfSHA256KnownVector(t *testing.T) {
	ikm := bytes.Repeat([]byte{0x0b}, 22)
	salt := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
	info := []byte{0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9}
	okm := hkdfSHA256(ikm, salt, info, 42)
	want := []byte{
		0x3c, 0xb2, 0x5f, 0x25, 0xfa, 0xac, 0xd5, 0x7a, 0x90, 0x43, 0x4f,
		0x64, 0xd0, 0x36, 0x2f, 0x2a, 0x2d, 0x2d, 0x0a, 0x90, 0xcf, 0x1a,
		0x5a, 0x4c, 0x5d, 0xb0, 0x2d, 0x56, 0xec, 0xc4, 0xc5, 0xbf, 0x34,
		0x00, 0x72, 0x08, 0xd5, 0xb8, 0x87, 0x18, 0x58, 0x65,
	}
	if !bytes.Equal(okm, want) {
		t.Fatalf("HKDF RFC5869 case1 mismatch:\n got %x\nwant %x", okm, want)
	}
}
