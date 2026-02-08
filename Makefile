.PHONY: run build
.DEFAULT_GOAL := run

build:
	go build -o wiggums .

run: build
	./wiggums run $(ARGS)

yolo:
	make run ARGS="--model opus --dangerously-skip-permissions"

index:
	@if [ -f index.md ]; then echo "Error: index.md already exists"; exit 1; fi
	@echo "Creating index.md"
	@cp templates/index.sample.md index.md