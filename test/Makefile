CRIU ?= criu
CC ?= gcc

all: test clean

test: test-imgs test-imgs-diff
	@echo "Running BATS tests..."
	bats checkpointctl.bats check-size.bats

test-junit: test-imgs test-imgs-diff
	@echo "Running BATS tests with JUnit results..."
	bats -F junit checkpointctl.bats check-size.bats > junit.xml

test-imgs: piggie/piggie
	$(eval PID := $(shell export TEST_ENV=BAR TEST_ENV_EMPTY=; piggie/piggie --tcp-socket --zombie))
	mkdir -p $@
	$(CRIU) dump --tcp-established -v4 -o dump.log -D $@ -t $(PID) || cat $@/dump.log

test-imgs-diff: piggie/piggie
	CRIU=$(CRIU) ./test-imgs-diff.sh $@

piggie/piggie: piggie/piggie.c
	$(CC) $^ -o $@

test-compression:
	@test -n "$(CRIU_TEST_BINARY)" || { echo 'Set CRIU_TEST_BINARY to an LZ4-enabled CRIU executable'; exit 1; }
	cd .. && CRIU_TEST_BINARY="$(CRIU_TEST_BINARY)" go test ./cmd -run '^TestCRIUCompressed' -v -count=1 -timeout=5m

clean:
	@echo "Cleaning up test files..."
	@rm -rf test-imgs test-imgs-diff piggie/piggie

.PHONY: all test test-junit test-imgs-diff test-compression clean
