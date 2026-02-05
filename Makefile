.PHONY: run
.DEFAULT_GOAL := run

run:
	@chmod +x wiggums.sh
	./wiggums.sh $(ARGS)

yolo:
	make run ARGS="--model opus --dangerously-skip-permissions"

index:
	@if [ -f index.md ]; then echo "Error: index.md already exists"; exit 1; fi
	@echo "Creating index.md"
	@cp templates/index.sample.md index.md