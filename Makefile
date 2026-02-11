.PHONY: run build setup
.DEFAULT_GOAL := run

build:
	go build -o wiggums .

run: build

setup: build
	@mkdir -p .obsidian
	@cp -n .obsidian.sample/community-plugins.json .obsidian/community-plugins.json 2>/dev/null || echo "community-plugins.json already exists, skipping"
	@cp -n .obsidian.sample/hotkeys.json .obsidian/hotkeys.json 2>/dev/null || echo "hotkeys.json already exists, skipping"
	@cp -n .obsidian.sample/app.json .obsidian/app.json 2>/dev/null || echo "app.json already exists, skipping"
	@cp -rn .obsidian.sample/plugins .obsidian/plugins 2>/dev/null || echo "plugins already exists, skipping"
	@cp -n templates/index.sample.md index.md 2>/dev/null || echo "index.md already exists, skipping"
	@# Shell completion setup (idempotent)
	@grep -q '^wiggums()' ~/.zshrc 2>/dev/null || (printf '\nwiggums() {\n    $(CURDIR)/wiggums "$$@"\n}\n' >> ~/.zshrc && echo "Added wiggums shell function to ~/.zshrc")
	@grep -q 'wiggums completion zsh' ~/.zshrc 2>/dev/null || (echo 'source <(wiggums completion zsh)' >> ~/.zshrc && echo "Added wiggums completion to ~/.zshrc")
	@echo "Setup complete"