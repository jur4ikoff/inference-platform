BUF := docker run --rm -v "$(CURDIR):/workspace" -w /workspace bufbuild/buf:1.61.0

.PHONY: generate lint check fmt test

generate:
	$(BUF) generate

lint:
	$(BUF) lint

check:
	$(BUF) format --diff --exit-code
	$(BUF) lint
	$(BUF) build

fmt:
	$(BUF) format -w

test:
	go test ./...
