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
	// per-agent last sample write time — RTP pacing is measured, not fixed
	rtcLastWrite = map[string]time.Time{}
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
	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se))
	return api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs: []string{"stun:stun.l.google.com:19302"},
		}},
	})
}

// feedWebRTC splits one fMP4 media segment into access units (each VCL NAL is
// one frame — c2push feeds single-slice x264 output — with SPS/PPS/SEI NALs
// that precede it attached to that AU) and writes every AU as its own sample.
// Sample durations come from the measured inter-segment arrival gap split
// evenly across the AUs, so RTP pacing follows the real encoder cadence
// instead of the old fixed 80ms-per-fragment pacing (~24fps input played like
// 12.5fps).
func feedWebRTC(agentID string, fmp4 []byte) {
	aus := accessUnitsFromFMP4(fmp4)
	if len(aus) == 0 {
		return
	}
	t := rtcTrack(agentID)
	if t == nil {
		return
	}
	now := time.Now()
	total := time.Second / 24 // fallback content span when measurement is absent
	rtcMu.Lock()
	if prev, ok := rtcLastWrite[agentID]; ok {
		if d := now.Sub(prev); d > 20*time.Millisecond && d < 5*time.Second {
			total = d
		}
	}
	rtcLastWrite[agentID] = now
	rtcMu.Unlock()
	dur := total / time.Duration(max(len(aus), 1))
	// SPS/PPS 인밴드 주입 — fMP4 init은 RTP에 안 실려 디코더가 videoWidth를 못 만든다 (실측: mdat에 type7/8=0)
	var params []byte
	streamMu.RLock()
	if init := streamInit[agentID]; init != nil {
		params = init.Params
	}
	streamMu.RUnlock()
	for _, au := range aus {
		data := au
		if len(params) > 0 && !auStartsWithParamSet(au) {
			buf := make([]byte, 0, len(params)+len(au))
			buf = append(buf, params...)
			buf = append(buf, au...)
			data = buf
		}
		_ = t.WriteSample(media.Sample{Data: data, Duration: dur})
	}
}

// auStartsWithParamSet reports whether the AU already begins with an SPS NAL (type 7).
func auStartsWithParamSet(au []byte) bool {
	if len(au) < 5 {
		return false
	}
	l := int(binary.BigEndian.Uint32(au[:4]))
	if l <= 0 || 4+l > len(au) {
		return false
	}
	return au[4]&0x1f == 7
}

// accessUnitsFromFMP4 locates the mdat payload of one fMP4 media segment and
// converts it to Annex-B access units.
func accessUnitsFromFMP4(seg []byte) [][]byte {
	i := 0
	for i+8 <= len(seg) {
		size := int(binary.BigEndian.Uint32(seg[i : i+4]))
		if size < 8 || i+size > len(seg) {
			return nil
		}
		if string(seg[i+4:i+8]) == "mdat" {
			return annexBAccessUnits(seg[i+8 : i+size])
		}
		i += size
	}
	return nil
}

// annexBAccessUnits converts avcc length-prefixed NALs into Annex-B frames:
// each VCL NAL (type 1-5) starts a new AU; AUD/SPS/PPS/SEI NALs leading up to
// it stick to that AU.
func annexBAccessUnits(avcc []byte) [][]byte {
	var aus [][]byte
	cur := []byte{}
	for len(avcc) >= 4 {
		n := int(binary.BigEndian.Uint32(avcc[:4]))
		avcc = avcc[4:]
		if n <= 0 || n > len(avcc) {
			return aus
		}
		nalType := int(avcc[0] & 0x1f)
		if nalType >= 1 && nalType <= 5 && len(cur) > 0 {
			aus = append(aus, cur)
			cur = nil
		}
		cur = append(cur, 0, 0, 0, 1)
		cur = append(cur, avcc[:n]...)
		avcc = avcc[n:]
	}
	if len(cur) > 0 {
		aus = append(aus, cur)
	}
	return aus
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
