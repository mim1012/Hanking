package main

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// WebRTC viewer path. c2push keeps pushing fMP4 segments; the server extracts
// Annex-B access units and writes them to a per-agent static sample track.
// Viewers subscribe with a single offer/answer (no trickle).

var (
	rtcMu     sync.Mutex
	rtcTracks = map[string]*webrtc.TrackLocalStaticSample{}
)

func rtcPublicIP() string {
	if ip := os.Getenv("WEBRTC_PUBLIC_IP"); ip != "" {
		return ip
	}
	return "45.32.251.5"
}

func rtcTrack(agentID string) *webrtc.TrackLocalStaticSample {
	rtcMu.Lock()
	defer rtcMu.Unlock()
	if t, ok := rtcTracks[agentID]; ok {
		return t
	}
	t, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		"video",
		agentID,
	)
	if err != nil {
		logPrintf("webrtc track %s: %v", agentID, err)
		return nil
	}
	rtcTracks[agentID] = t
	return t
}

func newViewerPC() (*webrtc.PeerConnection, error) {
	se := webrtc.SettingEngine{}
	se.SetNAT1To1IPs([]string{rtcPublicIP()}, webrtc.ICECandidateTypeHost)
	if err := se.SetEphemeralUDPPortRange(50000, 50100); err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}},
	})
}

// feedWebRTC converts one fMP4 media segment into an Annex-B sample and
// pushes it to every subscriber of agentID. Safe to call with a copied payload.
func feedWebRTC(agentID string, fmp4 []byte) {
	annex := annexBFromFMP4(fmp4)
	if len(annex) == 0 {
		return
	}
	t := rtcTrack(agentID)
	if t == nil {
		return
	}
	_ = t.WriteSample(media.Sample{
		Data:     annex,
		Duration: 80 * time.Millisecond,
	})
}

func annexBFromFMP4(seg []byte) []byte {
	i := 0
	for i+8 <= len(seg) {
		size := int(binary.BigEndian.Uint32(seg[i : i+4]))
		if size < 8 || i+size > len(seg) {
			break
		}
		if string(seg[i+4:i+8]) == "mdat" {
			return avccToAnnexB(seg[i+8 : i+size])
		}
		i += size
	}
	return nil
}

func avccToAnnexB(b []byte) []byte {
	var out []byte
	start := []byte{0, 0, 0, 1}
	for len(b) >= 4 {
		n := int(binary.BigEndian.Uint32(b[:4]))
		b = b[4:]
		if n <= 0 || n > len(b) {
			break
		}
		out = append(out, start...)
		out = append(out, b[:n]...)
		b = b[n:]
	}
	return out
}

// POST /admin/webrtc/offer  {agent_id, sdp} → {sdp}
// Viewer is recvonly. Server answers with the agent's live H264 track.
func handleAdminWebRTCOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
		SDP     string `json:"sdp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" || req.SDP == "" {
		http.Error(w, "agent_id and sdp required", 400)
		return
	}
	pc, err := newViewerPC()
	if err != nil {
		logPrintf("webrtc pc: %v", err)
		http.Error(w, "pc", 500)
		return
	}
	track := rtcTrack(req.AgentID)
	if track == nil {
		pc.Close()
		http.Error(w, "track", 500)
		return
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  req.SDP,
	}); err != nil {
		pc.Close()
		http.Error(w, "offer: "+err.Error(), 400)
		return
	}
	if _, err := pc.AddTrack(track); err != nil {
		pc.Close()
		http.Error(w, "addtrack: "+err.Error(), 500)
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		http.Error(w, "answer: "+err.Error(), 500)
		return
	}
	gather := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		http.Error(w, "local", 500)
		return
	}
	select {
	case <-gather:
	case <-time.After(3 * time.Second):
	}
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			_ = pc.Close()
		}
	})
	writeJSON(w, 200, map[string]string{"sdp": pc.LocalDescription().SDP})
}
