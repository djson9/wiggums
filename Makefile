.PHONY: run build setup
.DEFAULT_GOAL := run

build:
	go build -o wiggums .

run: build

setup:
	@mkdir -p .obsidian
	@cp -n .obsidian.sample/community-plugins.json .obsidian/community-plugins.json 2>/dev/null || echo "community-plugins.json already exists, skipping"
	@cp -n .obsidian.sample/hotkeys.json .obsidian/hotkeys.json 2>/dev/null || echo "hotkeys.json already exists, skipping"
	@cp -n .obsidian.sample/app.json .obsidian/app.json 2>/dev/null || echo "app.json already exists, skipping"
	@cp -n templates/index.sample.md index.md 2>/dev/null || echo "index.md already exists, skipping"
	@echo "Setup complete"