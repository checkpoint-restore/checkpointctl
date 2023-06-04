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
	$(eval PID := $(shell piggie/piggie))
	mkdir -p $@
	$(CRIU) dump -v4 -o dump.log -D $@ -t $(PID)

piggie/piggie: piggie/piggie.c
	$(CC) $^ -o $@

clean:
	@echo "Cleaning up test files..."
	@rm -rf test-imgs piggie/piggie

.PHONY: all test test-junit clean
