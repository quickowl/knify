SHELL := /bin/bash

SESSION_MONITOR_DIR ?= apps/session-monitor
SESSION_MONITOR_BIN ?= $(SESSION_MONITOR_DIR)/session-monitor

.PHONY: session-monitor-test session-monitor-build session-monitor-status

session-monitor-test:
	cd "$(SESSION_MONITOR_DIR)" && go test ./...

session-monitor-build:
	cd "$(SESSION_MONITOR_DIR)" && go build -o session-monitor .

session-monitor-status: session-monitor-build
	"$(SESSION_MONITOR_BIN)" --status --recent "$${SESSION_MONITOR_RECENT:-12}" --out "$${SESSION_MONITOR_OUT:-/tmp/session-monitor-live-watch.json}" --hub-url "$${SESSION_MONITOR_HUB_URL:-http://127.0.0.1:8787}" --token "$${SESSION_MONITOR_HUB_TOKEN:-agentcanvas-dev-token}"
