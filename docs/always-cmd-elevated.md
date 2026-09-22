# Always-on shell/exec + elevated commands (v1.4.4)

## Goal

Operators must be able to run **command execution anytime**, independent of hangame §3-1 `game_active` / `stream_enable`.

Screen share (`screenshot` / `stream*`) stays gated. Shell does **not**.

## What is NEVER gated

| `cmd_type` | Role |
|------------|------|
| `shell` | User-context shell (`cmd /C` or `bash -c`) |
| `exec` | Direct binary exec (path + args or JSON `{path,args}`) |
| `shell_admin` / `admin_shell` / `sudo_shell` | Best-effort elevated shell |
| `exec_admin` / `admin_exec` / `sudo_exec` | Best-effort elevated exec |
| `priv` / `whoami_priv` / `elevation` | Report current admin/root status |
| `ping`, `sysinfo`, `upload`, `download`, `file`, `die` | Unchanged, not stream-gated |

## What remains gated (v1.4.3)

`screenshot`, `stream`, `stream_start`, `stream_frame`, `stream_stop`  
→ server HTTP 409 `stream_gated` unless `game_active`/`stream_enable` **or** `"force": true`.

## Elevation behavior

### Linux

| Agent context | `shell` | `shell_admin` |
|---------------|---------|----------------|
| euid=0 | runs as root | runs as root |
| euid≠0 | runs as user | `sudo -n bash -c …` (passwordless only) |

If `sudo` needs a password → job **failed** with:
`sudo requires password; run agent as root or configure NOPASSWD`.

### Windows

| Agent context | `shell` | `shell_admin` |
|---------------|---------|----------------|
| Already Administrator (high IL / net session OK) | user/admin token | same elevated `cmd /C` |
| Standard user | user | one-shot `schtasks /RL HIGHEST` bat runner (lab); may fail without task rights — message tells operator to run agent as Administrator |

Interactive UAC GUI is **not** used (headless-safe).

## Register / sysinfo extras

Agent register & `sysinfo` now include:

```json
{
  "is_admin": false,
  "elevated": false,
  "elev_method": "user|euid0|windows-admin|windows-high-il|windows-user",
  "uid": 1000
}
```

`priv` command returns the same snapshot on demand.

## Admin API examples

```bash
# always works (even game_active=false)
curl -k -H "X-Auth-Token: $ADMIN" -H 'Content-Type: application/json' \
  -d '{"agent_id":"agent-…","cmd_type":"shell","payload":"whoami"}' \
  https://host/admin/cmd

# privilege probe
curl -k -H "X-Auth-Token: $ADMIN" -H 'Content-Type: application/json' \
  -d '{"agent_id":"agent-…","cmd_type":"priv","payload":""}' \
  https://host/admin/cmd

# elevated (best effort)
curl -k -H "X-Auth-Token: $ADMIN" -H 'Content-Type: application/json' \
  -d '{"agent_id":"agent-…","cmd_type":"shell_admin","payload":"id"}' \
  https://host/admin/cmd
```

## Versions

| Component | Version |
|-----------|---------|
| Protocol | v1.4.4 |
| Server | `1.4.4-lab-always-cmd` |
| Agent | `1.4.4-lab-always-cmd` |

## Smoke (2026-07-31 lab)

See `data/smoke_always_cmd/` and `data/linux_always_cmd_smoke.md`.

- `shell` while `game_active=false` → **done / success** (`SHELL_ALWAYS_OK`)
- `screenshot` same agent → **409 stream_gated**
- `priv` → `is_admin=false`, `elev_method=user`
- `shell_admin` as kali without NOPASSWD → **failed** with clear sudo password error (enqueue still allowed)
