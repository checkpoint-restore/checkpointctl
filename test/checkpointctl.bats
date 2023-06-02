if [ -n "$COVERAGE" ]; then
	export GOCOVERDIR="${COVERAGE_PATH}"
	CHECKPOINTCTL="./checkpointctl.coverage"
else
	CHECKPOINTCTL="./checkpointctl"
fi
TEST_TMP_DIR1=""
TEST_TMP_DIR2=""

function checkpointctl() {
	# shellcheck disable=SC2086
	run $CHECKPOINTCTL "$@"
	echo "$output"
}

function setup() {
	TEST_TMP_DIR1=$(mktemp -d)
	TEST_TMP_DIR2=$(mktemp -d)
}

function teardown() {
	[ "$TEST_TMP_DIR1" != "" ] && rm -rf "$TEST_TMP_DIR1"
	[ "$TEST_TMP_DIR2" != "" ] && rm -rf "$TEST_TMP_DIR2"
}

@test "Run checkpointctl" {
	checkpointctl
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl with wrong parameter" {
	checkpointctl --wrong-parameter
	[ "$status" -eq 1 ]
	[ "$output" = "Error: unknown flag: --wrong-parameter" ]
}

@test "Run checkpointctl show with non existing directory" {
	checkpointctl show /does-not-exist
	[ "$status" -eq 1 ]
	[[ ${lines[0]} = "Error: stat /does-not-exist: no such file or directory" ]]
}

@test "Run checkpointctl show with empty tar file" {
	touch "$TEST_TMP_DIR1"/empty.tar
	checkpointctl show "$TEST_TMP_DIR1"/empty.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl show with tar file with empty config.dump" {
	touch "$TEST_TMP_DIR1"/config.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"config.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and no spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: no such file or directory" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and empty spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	touch "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and no checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"Podman"* ]]
}

@test "Run checkpointctl show with tar file from containerd with valid config.dump and valid spec.dump and checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo "{}" > "$TEST_TMP_DIR1"/status
	echo "{}" >  "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"containerd"* ]]
}

@test "Run checkpointctl show with tar file and --stats and missing stats-dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 1 ]
	[[ ${lines[6]} == *"unable to display checkpointing statistics"* ]]
}

@test "Run checkpointctl show with tar file and --stats and invalid stats-dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"/stats-dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 1 ]
	[[ ${lines[6]} == *"Unknown magic"* ]]
}

@test "Run checkpointctl show with tar file and --stats and valid stats-dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	cp test/stats-dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"CRIU dump statistics"* ]]
	[[ ${lines[8]} == *"MEMWRITE TIME"* ]]
	[[ ${lines[10]} == *"446571 us"* ]]
}

@test "Run checkpointctl show with tar file and --mounts and valid spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --mounts
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Overview of Mounts"* ]]
	[[ ${lines[8]} == *"DESTINATION"* ]]
	[[ ${lines[10]} == *"/proc"* ]]
}

@test "Run checkpointctl show with tar file and --mounts and --full-paths and valid spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --mounts --full-paths
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Overview of Mounts"* ]]
	[[ ${lines[8]} == *"DESTINATION"* ]]
	[[ ${lines[10]} == *"/proc"* ]]
}

@test "Run checkpointctl show with tar file and --all and valid spec.dump and valid stats-dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	cp test/stats-dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --all
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Overview of Mounts"* ]]
	[[ ${lines[8]} == *"DESTINATION"* ]]
	[[ ${lines[10]} == *"/proc"* ]]
	[[ ${lines[11]} == *"/etc/hostname"* ]]
	[[ ${lines[13]} == *"CRIU dump statistics"* ]]
	[[ ${lines[15]} == *"MEMWRITE TIME"* ]]
	[[ ${lines[17]} == *"446571 us"* ]]
}

@test "Run checkpointctl show with tar file and missing --mounts/--all and --full-paths" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar --full-paths
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: Cannot use --full-paths without --mounts/--all option"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump (CRI-O) and no checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump (CRI-O) and checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"CRI-O"* ]]
}

@test "Run checkpointctl show with tar file compressed" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test.tar.gz . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar.gz
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"Podman"* ]]
}

@test "Run checkpointctl show with tar file corrupted" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	dd if=/dev/urandom of="$TEST_TMP_DIR2"/test.tar bs=1 count=10 seek=2 conv=notrunc
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: archive/tar: invalid tar header"* ]]
}

@test "Run checkpointctl show with tar file compressed and corrupted" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test.tar.gz . )
	dd if=/dev/urandom of="$TEST_TMP_DIR2"/test.tar.gz bs=1 count=10 seek=2 conv=notrunc
	checkpointctl show "$TEST_TMP_DIR2"/test.tar.gz
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: unexpected EOF"* ]]
}

@test "Run checkpointctl show with tar file and rootfs-diff tar file" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo 1 > "$TEST_TMP_DIR1"/test.pid
	tar -cf "$TEST_TMP_DIR1"/rootfs-diff.tar -C "$TEST_TMP_DIR1" test.pid
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[2]} == *"ROOT FS DIFF SIZE"* ]]
}

