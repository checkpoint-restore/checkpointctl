SHELL = /bin/bash
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(shell $(GO) env GOBIN)
GO_SRC = $(shell find . -name \*.go)
GO_BUILD = $(GO) build
NAME = checkpointctl

include Makefile.versions

COVERAGE_PATH ?= $(shell pwd)/.coverage

GO_MAJOR_VER = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f1)
GO_MINOR_VER = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
MIN_GO_MAJOR_VER = 1
MIN_GO_MINOR_VER = 20
GO_VALIDATION_ERR = Go version is not supported. Please update to at least $(MIN_GO_MAJOR_VER).$(MIN_GO_MINOR_VER)

all: $(NAME)

check-go-version:
	@if [ $(GO_MAJOR_VER) -gt $(MIN_GO_MAJOR_VER) ]; then \
		exit 0 ;\
	elif [ $(GO_MAJOR_VER) -lt $(MIN_GO_MAJOR_VER) ]; then \
		echo '$(GO_VALIDATION_ERR)';\
		exit 1; \
	elif [ $(GO_MINOR_VER) -lt $(MIN_GO_MINOR_VER) ] ; then \
		echo '$(GO_VALIDATION_ERR)';\
		exit 1; \
	fi


$(NAME): $(GO_SRC)
	$(GO_BUILD) -o $@ -ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"

$(NAME).coverage: check-go-version $(GO_SRC)
	$(GO) build \
		-cover \
		-o $@ \
		-ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"

release:
	CGO_ENABLED=0 $(GO_BUILD) -o $(NAME) -ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"

install: $(NAME)
	@echo "  INSTALL " $<
	@mkdir -p $(DESTDIR)$(BINDIR)
	@install -m0755 $< $(DESTDIR)$(BINDIR)
	@make -C docs install

uninstall:
	@make -C docs uninstall
	@echo " UNINSTALL" $(NAME)
	@$(RM) $(addprefix $(DESTDIR)$(BINDIR)/,$(NAME))

clean:
	rm -f $(NAME) junit.xml $(NAME).coverage $(COVERAGE_PATH)/*
	if [ -d $(COVERAGE_PATH) ]; then rmdir $(COVERAGE_PATH); fi
	@make -C docs clean

golang-lint:
	golangci-lint run

shellcheck:
	shellcheck test/*bats

lint: golang-lint shellcheck

test: $(NAME)
	$(GO) test -v ./...
	make -C test

test-junit: $(NAME)
	make -C test test-junit clean

coverage: check-go-version $(NAME).coverage
	mkdir -p $(COVERAGE_PATH)
	COVERAGE_PATH=$(COVERAGE_PATH) COVERAGE=1 make -C test
	# Print coverage from this run
	$(GO) tool covdata percent -i=${COVERAGE_PATH}
	$(GO) tool covdata textfmt -i=${COVERAGE_PATH} -o ${COVERAGE_PATH}/coverage.out

codecov:
	curl -Os https://uploader.codecov.io/latest/linux/codecov
	chmod +x codecov
	./codecov -f "$(COVERAGE_PATH)"/coverage.out

vendor:
	go mod tidy
	go mod vendor
	go mod verify

docs:
	@make -C docs

help:
	@echo "Usage: make <target>"
	@echo " * clean - remove artifacts"
	@echo " * docs - build man pages"
	@echo " * lint - verify the source code (shellcheck/golangci-lint)"
	@echo " * golang-lint - run golangci-lint"
	@echo " * shellcheck - run shellcheck"
	@echo " * vendor - update go.mod, go.sum, and vendor directory"
	@echo " * test - run tests"
	@echo " * test-junit - run tests and create junit output"
	@echo " * coverage - generate test coverage report"
	@echo " * codecov - upload coverage report to codecov.io"
	@echo " * install - install the binary to $(BINDIR)"
	@echo " * uninstall - remove the installed binary from $(BINDIR)"
	@echo " * release - build a static binary"
	@echo " * help - show help"

.PHONY: clean docs install uninstall release lint golang-lint shellcheck vendor test help check-go-version test-junit
