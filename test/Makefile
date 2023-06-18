CRIU ?= criu
CC ?= gcc
PODMAN ?= podman
PODMAN_IMAGE ?= looper

all: piggie-test podman-test clean

piggie-test: piggie-test-imgs
	@echo "Running BATS tests with piggie..."
	bats piggie.bats

podman-test: podman-test-imgs
	@echo "Running BATS tests with Podman checkpoint..."
	bats podman.bats

podman-test-imgs:
	$(PODMAN) run -d --name $(PODMAN_IMAGE) busybox /bin/sh -c 'i=0; while true; do echo $i; i=$(expr $i + 1); sleep 1; done'
	podman container checkpoint -l --export=podman-chkpt.tar.gz
	mkdir -p $@
	tar -xvf podman-chkpt.tar.gz -C $@

test-junit: piggie-test-imgs podman-test-imgs
	@echo "Running BATS tests with JUnit results..."
	bats -F junit *.bats > junit.xml

piggie-test-imgs: piggie/piggie
	$(eval PID := $(shell piggie/piggie))
	mkdir -p $@
	$(CRIU) dump -v4 -o dump.log -D $@ -t $(PID)

piggie/piggie: piggie/piggie.c
	$(CC) $^ -o $@

clean:
	@echo "Cleaning up piggie test files..."
	@rm -rf piggie-test-imgs piggie/piggie
	@echo "cleaning up podman image..."
	@podman container rm $(PODMAN_IMAGE) || true
	@echo "Cleaning up podman test files..."
	@rm -rf podman-chkpt.tar.gz podman-test-imgs

.PHONY: all piggie-test podman-test test-junit clean
