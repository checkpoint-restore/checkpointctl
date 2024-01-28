SHELL = /bin/bash
PREFIX ?= $(DESTDIR)/usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(shell $(GO) env GOBIN)
GO_SRC = $(shell find . -name \*.go)
GO_BUILD = $(GO) build
NAME = checkpointctl

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
ZSHINSTALLDIR=${PREFIX}/share/zsh/site-functions
FISHINSTALLDIR=${PREFIX}/share/fish/vendor_completions.d

include Makefile.versions

COVERAGE_PATH ?= $(shell pwd)/.coverage

GO_MAJOR_VER = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f1)
GO_MINOR_VER = $(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
MIN_GO_MAJOR_VER = 1
MIN_GO_MINOR_VER = 20
GO_VALIDATION_ERR = Go version is not supported. Please update to at least $(MIN_GO_MAJOR_VER).$(MIN_GO_MINOR_VER)

.PHONY: all
all: $(NAME)

.PHONY: check-go-version
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

.PHONY: release
release:
	CGO_ENABLED=0 $(GO_BUILD) -o $(NAME) -ldflags "-X main.name=$(NAME) -X main.version=${VERSION}"

.PHONY: install
install: $(NAME) install.completions
	@echo "  INSTALL " $<
	@mkdir -p $(DESTDIR)$(BINDIR)
	@install -m0755 $< $(DESTDIR)$(BINDIR)
	@make -C docs install

.PHONY: uninstall
uninstall: uninstall.completions
	@make -C docs uninstall
	@echo " UNINSTALL" $(NAME)
	@$(RM) $(addprefix $(DESTDIR)$(BINDIR)/,$(NAME))

.PHONY: clean
clean:
	rm -f $(NAME) junit.xml $(NAME).coverage $(COVERAGE_PATH)/*
	if [ -d $(COVERAGE_PATH) ]; then rmdir $(COVERAGE_PATH); fi
	@make -C docs clean

.PHONY: golang-lint
golang-lint:
	golangci-lint run

.PHONY: shellcheck
shellcheck:
	shellcheck test/*bats

.PHONY: lint
lint: golang-lint shellcheck

.PHONY: test
test: $(NAME)
	$(GO) test -v ./...
	make -C test

.PHONY: test-junit
test-junit: $(NAME)
	make -C test test-junit clean

.PHONY: coverage
coverage: check-go-version $(NAME).coverage
	mkdir -p $(COVERAGE_PATH)
	COVERAGE_PATH=$(COVERAGE_PATH) COVERAGE=1 make -C test
	# Print coverage from this run
	$(GO) tool covdata percent -i=${COVERAGE_PATH}
	$(GO) tool covdata textfmt -i=${COVERAGE_PATH} -o ${COVERAGE_PATH}/coverage.out

.PHONY: codecov
codecov:
	curl -Os https://uploader.codecov.io/latest/linux/codecov
	chmod +x codecov
	./codecov -f "$(COVERAGE_PATH)"/coverage.out

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: docs
docs:
	@make -C docs

.PHONY: completions
completions: $(NAME)
	declare -A outfiles=([bash]=%s [zsh]=_%s [fish]=%s.fish);\
	for shell in $${!outfiles[*]}; do \
		outfile=$$(printf "completions/$$shell/$${outfiles[$$shell]}" $(NAME)); \
		./$(NAME) completion $$shell >| $$outfile; \
	done

.PHONY: validate.completions
validate.completions: SHELL:=/usr/bin/env bash # Set shell to bash for this target
validate.completions:
	# Check if the files can be loaded by the shell
	. completions/bash/$(NAME)
	if [ -x /bin/zsh ]; then /bin/zsh completions/zsh/_$(NAME); fi
	if [ -x /bin/fish ]; then /bin/fish completions/fish/$(NAME).fish; fi

.PHONY: install.completions
install.completions:
	@install -d -m 755 ${DESTDIR}${BASHINSTALLDIR}
	@install -m 644 completions/bash/$(NAME) ${DESTDIR}${BASHINSTALLDIR}
	@install -d -m 755 ${DESTDIR}${ZSHINSTALLDIR}
	@install -m 644 completions/zsh/_$(NAME) ${DESTDIR}${ZSHINSTALLDIR}
	@install -d -m 755 ${DESTDIR}${FISHINSTALLDIR}
	@install -m 644 completions/fish/$(NAME).fish ${DESTDIR}${FISHINSTALLDIR}

.PHONY: uninstall.completions
uninstall.completions:
	@$(RM) $(addprefix ${DESTDIR}${BASHINSTALLDIR}/,$(NAME))
	@$(RM) $(addprefix ${DESTDIR}${ZSHINSTALLDIR}/,_$(NAME))
	@$(RM) $(addprefix ${DESTDIR}${FISHINSTALLDIR}/,$(NAME).fish)

.PHONY: help
help:
	@echo "Usage: make <target>"
	@echo " * completions - generate auto-completion files"
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
