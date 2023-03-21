SHELL = /bin/bash
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(shell $(GO) env GOBIN)
GO_SRC = $(shell find . -name \*.go)
GO_BUILD = $(GO) build
NAME = checkpointctl
COVERAGE_PATH ?= $(shell pwd)/.coverage

all: $(NAME)

$(NAME): $(GO_SRC)
	$(GO_BUILD) -buildmode=pie -o $@ -ldflags "-X main.name=$(NAME)"

$(NAME).coverage: $(GO_SRC)
	$(GO) test \
		-covermode=count \
		-coverpkg=./... \
		-mod=vendor \
		-tags coverage \
		-buildmode=pie -c -o $@ \
		-ldflags "-X main.name=$(NAME)"


install: $(NAME)
	@echo "  INSTALL " $<
	@mkdir -p $(DESTDIR)$(BINDIR)
	@install -m0755 $< $(DESTDIR)$(BINDIR)

uninstall:
	@echo " UNINSTALL" $(NAME)
	@$(RM) $(addprefix $(DESTDIR)$(BINDIR)/,$(NAME))

clean:
	rm -f $(NAME) $(NAME).coverage $(COVERAGE_PATH)/*
	if [ -d $(COVERAGE_PATH) ]; then rmdir $(COVERAGE_PATH); fi

golang-lint:
	golangci-lint run

shellcheck:
	shellcheck test/*bats

lint: golang-lint shellcheck

test: $(NAME)
	bats test/*bats

coverage: $(NAME).coverage
	mkdir -p $(COVERAGE_PATH)
	COVERAGE_PATH=$(COVERAGE_PATH) COVERAGE=1 bats test/*bats

codecov:
	curl -Os https://uploader.codecov.io/latest/linux/codecov
	chmod +x codecov
	./codecov -f '.coverage/*'

vendor:
	go mod tidy
	go mod vendor
	go mod verify

help:
	@echo "Usage: make <target>"
	@echo " * clean - remove artifacts"
	@echo " * lint - verify the source code (shellcheck/golangci-lint)"
	@echo " * golang-lint - run golang-lint"
	@echo " * shellcheck - run shellecheck"
	@echo " * vendor - update go.mod, go.sum and vendor directory"
	@echo " * test - run tests"
	@echo " * help - show help"

.PHONY: clean install uninstall lint golang-lint shellcheck vendor test help
