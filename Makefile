dump:
	@printf '%s\n' "--- ./go.mod ---" > dump.txt
	@cat go.mod >> dump.txt
	@printf '\n%s\n' "--- ./go.sum ---" >> dump.txt
	@cat go.sum >> dump.txt
	@for f in $$(find . -name '*.go' | sort); do printf '\n%s\n' "--- $$f ---" >> dump.txt && cat "$$f" >> dump.txt; done