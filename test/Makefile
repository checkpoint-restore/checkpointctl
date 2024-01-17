CRIU ?= criu
CC ?= gcc

all: test clean

test: test-imgs
	@echo "Running BATS tests..."
	bats checkpointctl.bats

test-junit: test-imgs
	@echo "Running BATS tests with JUnit results..."
	bats -F junit checkpointctl.bats > junit.xml

test-imgs: piggie/piggie
	$(eval PID := $(shell export TEST_ENV=BAR TEST_ENV_EMPTY=; piggie/piggie --tcp-socket))
	mkdir -p $@
	$(CRIU) dump --tcp-established -v4 -o dump.log -D $@ -t $(PID) || cat $@/dump.log

piggie/piggie: piggie/piggie.c
	$(CC) $^ -o $@

clean:
	@echo "Cleaning up test files..."
	@rm -rf test-imgs piggie/piggie

.PHONY: all test test-junit clean
