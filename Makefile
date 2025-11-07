.DEFAULT_GOAL := build
PREFIX ?= /usr/local
VERSION := $(shell git describe --tags 2>/dev/null)
STATICCHECK := $(shell command -v staticcheck 2> /dev/null)

# Make sure this target stays first!
.PHONY: help
help: ## print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: setup
setup: ## install statickcheck
ifndef STATICCHECK
	go install honnef.co/go/tools/cmd/staticcheck@latest
endif

.PHONY: build
build: ## build the binary
	@go build -ldflags "-X main.KEAI_VERSION=$(VERSION)"

.PHONY: install
install: ## install keai to your system
	@mkdir -p $(PREFIX)/bin
	@cp -f keai $(PREFIX)/bin/keai
	@chmod 755 $(PREFIX)/bin/keai

.PHONY: clean
clean: ## clean the repo
	@rm -f keai coverage.out

.PHONY: cover
cover: ## test with coverage
	@go test -coverprofile=coverage.out ./...

.PHONY: cover_open
cover_open: ## open coverage report in browser
	@go tool cover -html=coverage.out

.PHONY: count
count: ## count lines of code
	@cloc --exclude-dir=x,.git,.github,examples --read-lang-def=editor/keai.cloc .

.PHONY: test
test: ## lint and test
	$(MAKE) setup
	go mod verify
	@go fmt ./...
	@go vet ./...
	@staticcheck ./...
	@go test ./...

.PHONY: tags
tags: ## generate ctags
	@ctags --exclude=x --exclude=examples --exclude=editor -R .

.PHONY: version
version: ## bump semver version, see git-release.sh for details
	./git-release.sh patch
