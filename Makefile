SHELL = /bin/bash
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(shell $(GO) env GOBIN)
GO_SRC = $(shell find . -name \*.go)
GO_BUILD = $(GO) build
NAME = checkpointctl

VERSION_MAJOR := 0
VERSION_MINOR := 1
VERSION_SUBLEVEL := 0
VERSION_EXTRA :=
VERSION := $(VERSION_MAJOR)$(if $(VERSION_MINOR),.$(VERSION_MINOR))$(if $(VERSION_SUBLEVEL),.$(VERSION_SUBLEVEL))$(if $(VERSION_EXTRA),.$(VERSION_EXTRA))

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
	$(GO_BUILD) -buildmode=pie -o $@ -ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"

$(NAME).coverage: check-go-version $(GO_SRC)
	$(GO) build \
		-cover \
		-buildmode=pie -o $@ \
		-ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"


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

coverage: check-go-version $(NAME).coverage
	mkdir -p $(COVERAGE_PATH)
	COVERAGE_PATH=$(COVERAGE_PATH) COVERAGE=1 bats test/*bats
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

help:
	@echo "Usage: make <target>"
	@echo " * clean - remove artifacts"
	@echo " * lint - verify the source code (shellcheck/golangci-lint)"
	@echo " * golang-lint - run golang-lint"
	@echo " * shellcheck - run shellecheck"
	@echo " * vendor - update go.mod, go.sum and vendor directory"
	@echo " * test - run tests"
	@echo " * help - show help"

.PHONY: clean install uninstall lint golang-lint shellcheck vendor test help check-go-version
