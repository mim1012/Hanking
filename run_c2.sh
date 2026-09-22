#!/bin/bash
cd /home/kali/Projects/c2-project
export AGENT_TOKEN="lab-only-change-me"
export AUTH_TOKEN="REDACTED_ADMIN_TOKEN"
export ADMIN_TOKEN="REDACTED_ADMIN_TOKEN"
export BEACON_PATH="/jquery.min.js"
export BEACON_URIS="/cdn/analytics.js,/api/v2/telemetry"
export AGENT_WS_PATH="/socket.io"
export AGENT_WS_PATHS="/socket.io,/ws"
export STAGE_RETRY_WAIT=30
export FILE_MAX_BYTES=16777216
exec ./bin/c2server 2>&1