.PHONY: run
.DEFAULT_GOAL := run

run:
	@chmod +x wiggums.sh
	./wiggums.sh $(ARGS)
