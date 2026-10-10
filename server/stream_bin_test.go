package main

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
)

// binFrame builds one cleartext binary stream frame as applyStreamFrameBinary
// expects it AFTER decryption:
//
//	kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payload
func binFrame(kind byte, stream, seq int64, codec string, payload []byte) []byte {
	f := make([]byte, 0, 22+len(codec)+len(payload))
	f = append(f, kind)
	var b8 [8]byte
	binary.LittleEndian.PutUint64(b8[:], uint64(stream))
	f = append(f, b8[:]...)
	binary.LittleEndian.PutUint64(b8[:], uint64(seq))
	f = append(f, b8[:]...)
	f = append(f, byte(len(codec)))
	f = append(f, codec...)
	f = append(f, payload...)
	return f
}

func resetStreamMaps() {
	streamMu.Lock()
	streamInit = map[string]*StreamInit{}
	streamSegs = map[string][]*StreamSeg{}
	streamFrames = map[string]*StreamFrame{}
	streamMu.Unlock()
}

func TestApplyStreamFrameBinary(t *testing.T) {
	resetStreamMaps()
	agent := "onpxe-BIN"

	// init frame stores Raw and resets the ring
	applyStreamFrameBinary(agent, binFrame(1, 5, 0, "avc1.42C00B", []byte("INIT")))
	streamMu.RLock()
	init := streamInit[agent]
	nSeg := len(streamSegs[agent])
	streamMu.RUnlock()
	if init == nil || init.Stream != 5 || init.Codec != "avc1.42C00B" || !bytes.Equal(init.Raw, []byte("INIT")) {
		t.Fatalf("init not stored: %+v", init)
	}
	if nSeg != 0 {
		t.Fatalf("ring should start empty, got %d", nSeg)
	}

	// media frames append
	applyStreamFrameBinary(agent, binFrame(2, 5, 1, "", []byte("S1")))
	applyStreamFrameBinary(agent, binFrame(2, 5, 2, "", []byte("S2")))
	streamMu.RLock()
	segs := streamSegs[agent]
	streamMu.RUnlock()
	if len(segs) != 2 || segs[0].Seq != 1 || !bytes.Equal(segs[1].Raw, []byte("S2")) {
		t.Fatalf("media not appended: %+v", segs)
	}

	// media for an unknown generation is ignored
	applyStreamFrameBinary(agent, binFrame(2, 999, 3, "", []byte("S3")))
	streamMu.RLock()
	n := len(streamSegs[agent])
	streamMu.RUnlock()
	if n != 2 {
		t.Fatalf("stale gen media should be ignored, got %d", n)
	}

	// short/garbage frames must not panic or corrupt state
	applyStreamFrameBinary(agent, []byte{1, 2, 3})
	applyStreamFrameBinary(agent, nil)

	// a new init generation resets the media ring
	applyStreamFrameBinary(agent, binFrame(1, 6, 0, "avc1.42C00B", []byte("INIT2")))
	streamMu.RLock()
	init = streamInit[agent]
	n = len(streamSegs[agent])
	streamMu.RUnlock()
	if init == nil || init.Stream != 6 {
		t.Fatalf("new generation not stored: %+v", init)
	}
	if n != 0 {
		t.Fatalf("new generation should reset ring, got %d", n)
	}

	// ring is trimmed to streamSegMax, keeping the newest segments
	total := streamSegMax + 3
	for i := 1; i <= total; i++ {
		applyStreamFrameBinary(agent, binFrame(2, 6, int64(i), "", []byte{byte(i)}))
	}
	streamMu.RLock()
	segs = streamSegs[agent]
	streamMu.RUnlock()
	if len(segs) != streamSegMax {
		t.Fatalf("ring len=%d want %d", len(segs), streamSegMax)
	}
	if segs[len(segs)-1].Seq != int64(total) {
		t.Fatalf("newest seq=%d want %d", segs[len(segs)-1].Seq, total)
	}
	if segs[0].Seq != int64(total-streamSegMax+1) {
		t.Fatalf("oldest seq=%d want %d", segs[0].Seq, total-streamSegMax+1)
	}
}

func TestAeadRawRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	plain := []byte("hello binary stream")

	sealed, err := aeadSealRaw(key, plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	// nonce(12) + ct + gcm tag(16), and no JSON/base64 envelope
	if len(sealed) != 12+len(plain)+16 {
		t.Fatalf("sealed len=%d want %d", len(sealed), 12+len(plain)+16)
	}
	opened, err := aeadOpenRaw(key, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(opened, plain) {
		t.Fatalf("roundtrip mismatch: %q", opened)
	}

	// tampered ciphertext must fail authentication
	sealed[len(sealed)-1] ^= 0xFF
	if _, err := aeadOpenRaw(key, sealed); err == nil {
		t.Fatal("tampered frame should not decrypt")
	}
	// truncated frame must error, not panic
	if _, err := aeadOpenRaw(key, []byte{1, 2, 3}); err == nil {
		t.Fatal("short frame should error")
	}
	// wrong key must fail
	if _, err := aeadOpenRaw(bytes.Repeat([]byte{0x43}, 32), sealed[:len(sealed)-1]); err == nil {
		t.Fatal("wrong key should not decrypt")
	}
}

func TestHandleAdminStreamRaw(t *testing.T) {
	resetStreamMaps()
	agent := "onpxe-RAW"
	streamMu.Lock()
	streamInit[agent] = &StreamInit{AgentID: agent, Stream: 1, MIME: "video/mp4", Codec: "avc1.42C00B", Raw: []byte("INIT")}
	streamSegs[agent] = []*StreamSeg{
		{AgentID: agent, Stream: 1, Seq: 1, Raw: []byte("A")},
		{AgentID: agent, Stream: 1, Seq: 2, Raw: []byte("B")},
		{AgentID: agent, Stream: 1, Seq: 3, Raw: []byte("C")},
	}
	streamMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/admin/stream?agent_id="+agent+"&since_seq=1&format=raw", nil)
	rr := httptest.NewRecorder()
	handleAdminStream(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("content-type=%q", ct)
	}

	// parse concatenated records:
	// kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payloadLen(4,LE) || payload
	type rec struct {
		kind    byte
		stream  int64
		seq     int64
		codec   string
		payload []byte
	}
	body := rr.Body.Bytes()
	var recs []rec
	for off := 0; off < len(body); {
		if len(body)-off < 18 {
			t.Fatalf("trailing short record at %d (body %d bytes)", off, len(body))
		}
		r := rec{
			kind:   body[off],
			stream: int64(binary.LittleEndian.Uint64(body[off+1:])),
			seq:    int64(binary.LittleEndian.Uint64(body[off+9:])),
		}
		cl := int(body[off+17])
		if len(body)-off < 22+cl {
			t.Fatalf("short record header at %d", off)
		}
		r.codec = string(body[off+18 : off+18+cl])
		pl := int(binary.LittleEndian.Uint32(body[off+18+cl:]))
		if len(body)-off < 22+cl+pl {
			t.Fatalf("short record payload at %d", off)
		}
		r.payload = body[off+22+cl : off+22+cl+pl]
		recs = append(recs, r)
		off += 22 + cl + pl
	}

	// expect: init record + segs 2,3 (seq 1 filtered by since_seq=1)
	if len(recs) != 3 {
		t.Fatalf("records=%d want 3", len(recs))
	}
	if recs[0].kind != 1 || recs[0].seq != 0 || recs[0].codec != "avc1.42C00B" || !bytes.Equal(recs[0].payload, []byte("INIT")) {
		t.Fatalf("bad init record: %+v", recs[0])
	}
	if recs[1].kind != 2 || recs[1].seq != 2 || !bytes.Equal(recs[1].payload, []byte("B")) {
		t.Fatalf("bad seg record: %+v", recs[1])
	}
	if recs[2].kind != 2 || recs[2].seq != 3 || !bytes.Equal(recs[2].payload, []byte("C")) {
		t.Fatalf("bad seg record: %+v", recs[2])
	}
}
