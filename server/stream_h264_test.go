package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyStreamFrameH264(t *testing.T) {
	streamMu.Lock()
	streamInit = map[string]*StreamInit{}
	streamSegs = map[string][]*StreamSeg{}
	streamFrames = map[string]*StreamFrame{}
	streamMu.Unlock()

	agent := "onpxe-TEST"
	initB64 := base64.StdEncoding.EncodeToString([]byte("INITDATA"))
	seg1B64 := base64.StdEncoding.EncodeToString([]byte("SEG1"))
	seg2B64 := base64.StdEncoding.EncodeToString([]byte("SEG2"))

	// init
	inner, _ := json.Marshal(map[string]interface{}{
		"type": "stream_frame", "agent_id": agent, "stream": 7, "seg_type": "init",
		"mime": `video/mp4; codecs="avc1.42C00B"`, "codec": "avc1.42C00B",
		"size": 8, "data_b64": initB64,
	})
	applyStreamFrame(agent, map[string]interface{}{"data": string(inner)})
	streamMu.RLock()
	init := streamInit[agent]
	streamMu.RUnlock()
	if init == nil || init.Stream != 7 {
		t.Fatalf("init not stored: %+v", init)
	}

	// media segs
	for i, b := range []string{seg1B64, seg2B64} {
		inner, _ := json.Marshal(map[string]interface{}{
			"type": "stream_frame", "agent_id": agent, "stream": 7, "seg_type": "media",
			"seq": i + 1, "size": 4, "mime": "video/mp4", "data_b64": b,
		})
		applyStreamFrame(agent, map[string]interface{}{"data": string(inner)})
	}
	streamMu.RLock()
	n := len(streamSegs[agent])
	streamMu.RUnlock()
	if n != 2 {
		t.Fatalf("expected 2 segs, got %d", n)
	}

	// stale generation media should be ignored
	inner, _ = json.Marshal(map[string]interface{}{
		"type": "stream_frame", "agent_id": agent, "stream": 999, "seg_type": "media",
		"seq": 99, "size": 4, "mime": "video/mp4", "data_b64": seg1B64,
	})
	applyStreamFrame(agent, map[string]interface{}{"data": string(inner)})
	streamMu.RLock()
	n = len(streamSegs[agent])
	streamMu.RUnlock()
	if n != 2 {
		t.Fatalf("stale gen media should be ignored, got %d", n)
	}

	// legacy JPEG
	inner, _ = json.Marshal(map[string]interface{}{
		"type": "stream_frame", "agent_id": agent, "frame": 3, "size": 4,
		"mime": "image/jpeg", "data_b64": seg1B64,
	})
	applyStreamFrame(agent, map[string]interface{}{"data": string(inner)})
	streamMu.RLock()
	jf := streamFrames[agent]
	streamMu.RUnlock()
	if jf == nil || jf.MIME != "image/jpeg" {
		t.Fatalf("legacy jpeg not stored: %+v", jf)
	}
}

func TestHandleAdminStreamH264(t *testing.T) {
	streamMu.Lock()
	streamInit = map[string]*StreamInit{}
	streamSegs = map[string][]*StreamSeg{}
	streamFrames = map[string]*StreamFrame{}
	streamMu.Unlock()

	agent := "onpxe-ADM"
	streamMu.Lock()
	streamInit[agent] = &StreamInit{AgentID: agent, Stream: 1, MIME: `video/mp4; codecs="avc1.42C00B"`, Codec: "avc1.42C00B", Raw: []byte{0x00}}
	streamSegs[agent] = []*StreamSeg{
		{AgentID: agent, Stream: 1, Seq: 1, Raw: []byte{0x01}},
		{AgentID: agent, Stream: 1, Seq: 2, Raw: []byte{0x02}},
		{AgentID: agent, Stream: 1, Seq: 3, Raw: []byte{0x03}},
	}
	streamMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/admin/stream?agent_id="+agent+"&since_seq=1", nil)
	rr := httptest.NewRecorder()
	handleAdminStream(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out struct {
		Mode      string `json:"mode"`
		LatestSeq int64  `json:"latest_seq"`
		Init      *StreamInit `json:"init"`
		Segments  []*StreamSeg `json:"segments"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rr.Body.String())
	}
	if out.Mode != "h264" {
		t.Fatalf("expected h264 mode, got %q", out.Mode)
	}
	if out.LatestSeq != 3 {
		t.Fatalf("latest_seq=%d want 3", out.LatestSeq)
	}
	if len(out.Segments) != 2 { // seq 2,3 > since_seq 1
		t.Fatalf("segments=%d want 2", len(out.Segments))
	}
	if out.Init == nil || out.Init.Codec != "avc1.42C00B" {
		t.Fatalf("init missing/wrong: %+v", out.Init)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "avc1.42C00B") {
		t.Fatalf("mime/codec not surfaced: %s", body)
	}
}
