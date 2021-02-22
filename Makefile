SHELL = /bin/bash
GO ?= go
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(shell $(GO) env GOBIN)
GO_SRC = $(shell find . -name \*.go)
GO_BUILD = $(GO) build
NAME = checkpointctl

all: $(NAME)

$(NAME): $(GO_SRC)
	$(GO_BUILD) -buildmode=pie -o $@ -ldflags "-X main.name=$(NAME)"

clean:
	rm -f $(NAME)

golang-lint:
	golangci-lint run

shellcheck:
	shellcheck test/*bats

lint: golang-lint shellcheck

test: $(NAME)
	bats test/*bats

vendor:
	GO111MODULE=on go mod tidy
	GO111MODULE=on go mod vendor
	GO111MODULE=on go mod verify

help:
	@echo "Usage: make <target>"
	@echo " * clean - remove artifacts"
	@echo " * lint - verify the source code (shellcheck/golangci-lint)"
	@echo " * golang-lint - run golang-lint"
	@echo " * shellcheck - run shellecheck"
	@echo " * vendor - update go.mod, go.sum and vendor directory"
	@echo " * test - run tests"
	@echo " * help - show help"

.PHONY: clean lint golang-lint shellcheck vendor test help
