SHELL := /bin/bash

# Local, machine-specific overrides. Copy .env.example to .env and edit.
# .env is gitignored; do not commit personal hostnames or tokens.
# Plain KEY=VALUE syntax so the same file works with `set -a; source .env`.
ifneq (,$(wildcard ./.env))
  include .env
  export
endif

SESSION_MONITOR_DIR ?= apps/session-monitor
SESSION_MONITOR_BIN ?= $(SESSION_MONITOR_DIR)/session-monitor
HUB_DIR ?= apps/hub
SITE_DIR ?= apps/site
SKILLS_CLI ?= npx --yes skills
SKILLS_PACKAGE ?= $(CURDIR)
SKILLS_SCOPE ?= -g
SKILL_AGENT ?= codex
GOLANGCI_LINT ?= golangci-lint
GOIMPORTS_LOCAL ?= github.com/quickowl/knify/apps/session-monitor
GITLEAKS ?= gitleaks

# Public dev default — matches the fallback in apps/hub/src/app.ts.
# Override in .env or via SESSION_MONITOR_HUB_TOKEN when targeting a non-dev hub.
SESSION_MONITOR_LIVE_HUB_TOKEN ?= agentcanvas-dev-token

.PHONY: gitleaks-scan gitleaks-scan-staged githooks-install session-monitor-test session-monitor-build session-monitor-status session-monitor-pulse session-monitor-ipad-link session-monitor-fmt session-monitor-fmt-check session-monitor-vet session-monitor-lint session-monitor-tidy session-monitor-check hub-install hub-typecheck hub-test hub-build hub-start-local hub-deploy hub-seed-dashboard hub-check-dashboard hub-smoke-naive-agents site-install site-dev site-build site-deploy skill-session-monitor-validate skill-session-monitor-list skill-session-monitor-install skill-session-monitor-deploy

gitleaks-scan:
	$(GITLEAKS) git --redact --verbose --no-banner .

gitleaks-scan-staged:
	$(GITLEAKS) git --pre-commit --staged --redact --no-banner .

githooks-install:
	git config core.hooksPath .githooks

session-monitor-test:
	cd "$(SESSION_MONITOR_DIR)" && go test ./...

session-monitor-build:
	cd "$(SESSION_MONITOR_DIR)" && go build -o session-monitor .

session-monitor-fmt:
	cd "$(SESSION_MONITOR_DIR)" && gofmt -s -w .
	cd "$(SESSION_MONITOR_DIR)" && go run golang.org/x/tools/cmd/goimports@latest -w -local "$(GOIMPORTS_LOCAL)" .

session-monitor-fmt-check:
	@cd "$(SESSION_MONITOR_DIR)" && out=$$(gofmt -s -l .); if [ -n "$$out" ]; then echo "gofmt -s needs to run on:"; echo "$$out"; exit 1; fi

session-monitor-vet:
	cd "$(SESSION_MONITOR_DIR)" && go vet ./...

session-monitor-lint:
	cd "$(SESSION_MONITOR_DIR)" && $(GOLANGCI_LINT) run ./...

session-monitor-tidy:
	cd "$(SESSION_MONITOR_DIR)" && go mod tidy

session-monitor-check: session-monitor-fmt-check session-monitor-vet session-monitor-lint session-monitor-test

session-monitor-status: session-monitor-build
	"$(SESSION_MONITOR_BIN)" --status --recent "$${SESSION_MONITOR_RECENT:-12}" --out "$${SESSION_MONITOR_OUT:-/tmp/session-monitor-live-watch.json}" --hub-url "$${SESSION_MONITOR_HUB_URL:-http://127.0.0.1:8787}" --token "$${SESSION_MONITOR_HUB_TOKEN:-agentcanvas-dev-token}"

session-monitor-pulse: session-monitor-build
	@while true; do \
		token="$${SESSION_MONITOR_HUB_TOKEN:-$(SESSION_MONITOR_LIVE_HUB_TOKEN)}"; \
		if [ -z "$$token" ]; then echo "Set SESSION_MONITOR_HUB_TOKEN before using session-monitor-pulse against the live hub." >&2; exit 1; fi; \
		clear; \
		"$(SESSION_MONITOR_BIN)" --status \
			--pretty \
			--recent "$${SESSION_MONITOR_RECENT:-5}" \
			--out "$${SESSION_MONITOR_OUT:-/tmp/session-monitor-live-watch.json}" \
			--hub-url "$${SESSION_MONITOR_HUB_URL:-$(SESSION_MONITOR_LIVE_HUB_URL)}" \
			--token "$$token"; \
		sleep "$${SESSION_MONITOR_PULSE_INTERVAL:-5}"; \
	done

session-monitor-ipad-link:
	@set -euo pipefail; \
	hub="$${SESSION_MONITOR_HUB_URL:-$(SESSION_MONITOR_LIVE_HUB_URL)}"; \
	token="$${SESSION_MONITOR_HUB_TOKEN:-$(SESSION_MONITOR_LIVE_HUB_TOKEN)}"; \
	if [ -z "$$token" ]; then echo "Set SESSION_MONITOR_HUB_TOKEN before creating a live viewer link." >&2; exit 1; fi; \
	canvas="$${SESSION_MONITOR_CANVAS_ID:-canvas.session-monitor.local}"; \
	run="$${SESSION_MONITOR_RUN_ID:-run.session-monitor.local}"; \
	agent="$${SESSION_MONITOR_AGENT_ID:-session-monitor}"; \
	req=$$(jq -n --arg canvas "$$canvas" --arg run "$$run" --arg agent "$$agent" --arg base "$$hub" \
		'{kind:"configuration",canvasId:$$canvas,runId:$$run,agentId:$$agent,linkBaseUrl:$$base,ttlSeconds:604800}'); \
	preflight=$$(curl -fsS -X POST "$$hub/v1/viewer-links/preflight" -H "Authorization: Bearer $$token" -H 'Content-Type: application/json' -d "$$req"); \
	if [ "$$(printf '%s' "$$preflight" | jq -r .status)" != "ready" ]; then \
		printf '%s\n' "$$preflight" | jq .; \
		exit 1; \
	fi; \
	created=$$(curl -fsS -X POST "$$hub/v1/viewer-links" -H "Authorization: Bearer $$token" -H 'Content-Type: application/json' -d "$$req"); \
	id=$$(printf '%s' "$$created" | jq -r .id); \
	code=$$(printf '%s' "$$created" | jq -r .code); \
	curl -fsS -X POST "$$hub/v1/viewer-links/$$id/self-test" -H "Authorization: Bearer $$token" -H 'Content-Type: application/json' -d "$$(jq -n --arg code "$$code" '{code:$$code}')" >/dev/null; \
	ipad=$$(python3 -c 'import sys, urllib.parse; print("agentcanvas://configure/" + sys.argv[1] + "?hub=" + urllib.parse.quote(sys.argv[2], safe=""))' "$$code" "$$hub"); \
	printf 'iPad configure link:\n%s\n\nHTTP fallback:\n%s/c/%s\n' "$$ipad" "$$hub" "$$code"

hub-install:
	cd "$(HUB_DIR)" && npm ci

hub-typecheck:
	cd "$(HUB_DIR)" && npm run typecheck

hub-test:
	cd "$(HUB_DIR)" && npm test

hub-build:
	cd "$(HUB_DIR)" && npm run build

hub-start-local:
	cd "$(HUB_DIR)" && npm run start-local

hub-deploy:
	cd "$(HUB_DIR)" && npm run deploy

hub-seed-dashboard:
	cd "$(HUB_DIR)" && npm run seed:dashboard

hub-check-dashboard:
	cd "$(HUB_DIR)" && npm run check:dashboard

hub-smoke-naive-agents:
	cd "$(HUB_DIR)" && npm run smoke:naive-agents

site-install:
	cd "$(SITE_DIR)" && pnpm install

site-dev:
	cd "$(SITE_DIR)" && pnpm run dev

site-build:
	cd "$(SITE_DIR)" && pnpm run build

site-deploy:
	cd "$(SITE_DIR)" && pnpm run deploy

skill-session-monitor-validate:
	test -f skills/session-monitor/SKILL.md
	test -f skills/session-monitor/agents/openai.yaml
	grep -q '^name: session-monitor$$' skills/session-monitor/SKILL.md
	grep -Eq '^description: .+' skills/session-monitor/SKILL.md
	grep -Eq '^  short_description: .+' skills/session-monitor/agents/openai.yaml
	$(SKILLS_CLI) add "$(SKILLS_PACKAGE)" --skill "session-monitor" --list

skill-session-monitor-list:
	$(SKILLS_CLI) add "$(SKILLS_PACKAGE)" --skill "session-monitor" --list

skill-session-monitor-install: skill-session-monitor-validate
	$(SKILLS_CLI) add "$(SKILLS_PACKAGE)" $(SKILLS_SCOPE) --skill "session-monitor" --agent "$(SKILL_AGENT)" -y

skill-session-monitor-deploy: skill-session-monitor-validate
	$(SKILLS_CLI) add "$(SKILLS_PACKAGE)" -g --skill "session-monitor" --agent "*" -y
