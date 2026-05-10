SHELL := /bin/bash

SESSION_MONITOR_DIR ?= apps/session-monitor
SESSION_MONITOR_BIN ?= $(SESSION_MONITOR_DIR)/session-monitor
SKILLS_CLI ?= npx --yes skills
SKILLS_PACKAGE ?= $(CURDIR)
SKILLS_SCOPE ?= -g
SKILL_AGENT ?= codex

.PHONY: session-monitor-test session-monitor-build session-monitor-status skill-session-monitor-validate skill-session-monitor-list skill-session-monitor-install skill-session-monitor-deploy

session-monitor-test:
	cd "$(SESSION_MONITOR_DIR)" && go test ./...

session-monitor-build:
	cd "$(SESSION_MONITOR_DIR)" && go build -o session-monitor .

session-monitor-status: session-monitor-build
	"$(SESSION_MONITOR_BIN)" --status --recent "$${SESSION_MONITOR_RECENT:-12}" --out "$${SESSION_MONITOR_OUT:-/tmp/session-monitor-live-watch.json}" --hub-url "$${SESSION_MONITOR_HUB_URL:-http://127.0.0.1:8787}" --token "$${SESSION_MONITOR_HUB_TOKEN:-agentcanvas-dev-token}"

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
