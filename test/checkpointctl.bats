if [ -n "$COVERAGE" ]; then
	export GOCOVERDIR="${COVERAGE_PATH}"
	CHECKPOINTCTL="../checkpointctl.coverage"
else
	CHECKPOINTCTL="../checkpointctl"
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
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: no such file or directory" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and empty spec.dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	touch "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and no checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"Podman"* ]]
}

@test "Run checkpointctl show with tar file from containerd with valid config.dump and valid spec.dump and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo "{}" >  "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"containerd"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump (CRI-O) and no checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump (CRI-O) and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"CRI-O"* ]]
}

@test "Run checkpointctl show with tar file compressed" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test.tar.gz . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar.gz
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"Podman"* ]]
}

@test "Run checkpointctl show with tar file corrupted" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	dd if=/dev/urandom of="$TEST_TMP_DIR2"/test.tar bs=1 count=10 seek=2 conv=notrunc
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: archive/tar: invalid tar header"* ]]
}

@test "Run checkpointctl show with tar file compressed and corrupted" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test.tar.gz . )
	dd if=/dev/urandom of="$TEST_TMP_DIR2"/test.tar.gz bs=1 count=10 seek=2 conv=notrunc
	checkpointctl show "$TEST_TMP_DIR2"/test.tar.gz
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: unexpected EOF"* ]]
}

@test "Run checkpointctl show with tar file and rootfs-diff tar file" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo 1 > "$TEST_TMP_DIR1"/test.pid
	tar -cf "$TEST_TMP_DIR1"/rootfs-diff.tar -C "$TEST_TMP_DIR1" test.pid
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[2]} == *"ROOT FS DIFF SIZE"* ]]
}

@test "Run checkpointctl show with multiple tar files" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar .  && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/*.tar
	[ "$status" -eq 0 ]
	[[ ${lines[3]} == *"Podman"* ]]
	[[ ${lines[5]} == *"Podman"* ]]
}

@test "Run checkpointctl inspect with invalid format" {
	touch "$TEST_TMP_DIR1"/config.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --format=invalid
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"invalid output format"* ]]
}

@test "Run checkpointctl inspect with tar file with empty config.dump" {
	touch "$TEST_TMP_DIR1"/config.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"config.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl inspect with tar file with valid config.dump and no spec.dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: no such file or directory" ]]
}

@test "Run checkpointctl inspect with tar file with valid config.dump and empty spec.dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	touch "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"spec.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl inspect with tar file and --stats and missing stats-dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to get dump statistics"* ]]
}

@test "Run checkpointctl inspect with tar file and --stats and invalid stats-dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"/stats-dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"unknown magic"* ]]
}

@test "Run checkpointctl inspect with tar file and --stats and valid stats-dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	cp test-imgs/stats-dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --stats
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"CRIU dump statistics"* ]]
	[[ ${lines[12]} == *"Memwrite time"* ]]
	[[ ${lines[13]} =~ [1-9]+" us" ]]
}

@test "Run checkpointctl inspect with tar file and --mounts and valid spec.dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --mounts
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"Overview of mounts"* ]]
	[[ ${lines[9]} == *"Destination"* ]]
	[[ ${lines[10]} == *"proc"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"Process tree"* ]]
	[[ ${lines[9]} == *"piggie"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree and missing pstree.img" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to get process tree"* ]]
}

@test "Run checkpointctl inspect with tar file and --files" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/files.img \
		test-imgs/fs-*.img \
		test-imgs/ids-*.img \
		test-imgs/fdinfo-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --files
	[ "$status" -eq 0 ]
	[[ ${lines[10]} == *"[REG 0]"* ]]
	[[ ${lines[11]} == *"[cwd]"* ]]
	[[ ${lines[12]} == *"[root]"* ]]
}

@test "Run checkpointctl inspect with tar file and --files and missing files.img" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --files
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to get file descriptors"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree and valid PID" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree --pid 1
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"Process tree"* ]]
	[[ ${lines[9]} == *"piggie"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree and invalid PID" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree --pid 99999
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"no process with PID 99999"* ]]
}

@test "Run checkpointctl inspect with tar file and --all and valid spec.dump and valid stats-dump" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	cp test-imgs/stats-dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/files.img \
		test-imgs/fs-*.img \
		test-imgs/ids-*.img \
		test-imgs/fdinfo-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --all
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"CRIU dump statistics"* ]]
	[[ ${lines[12]} == *"Memwrite time"* ]]
	[[ ${lines[13]} =~ [1-9]+" us" ]]
	[[ ${lines[15]} == *"Process tree"* ]]
	[[ ${lines[16]} == *"piggie"* ]]
	[[ ${lines[17]} == *"[REG 0]"* ]]
	[[ ${lines[18]} == *"[cwd]"* ]]
	[[ ${lines[19]} == *"[root]"* ]]
	[[ ${lines[20]} == *"Overview of mounts"* ]]
	[[ ${lines[21]} == *"Destination"* ]]
	[[ ${lines[22]} == *"proc"* ]]
	[[ ${lines[24]} == *"/etc/hostname"* ]]

}

@test "Run checkpointctl inspect with tar file with valid config.dump and valid spec.dump and no checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: checkpoint directory is missing in the archive file"* ]]
}

@test "Run checkpointctl inspect with tar file with valid config.dump and valid spec.dump and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Podman"* ]]
}

@test "Run checkpointctl inspect with tar file from containerd with valid config.dump and valid spec.dump and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo "{}" > "$TEST_TMP_DIR1"/status
	echo "{}" >  "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"containerd"* ]]
}

@test "Run checkpointctl inspect with tar file with valid config.dump and valid spec.dump (CRI-O) and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"CRI-O"* ]]
}

@test "Run checkpointctl inspect with tar file and rootfs-diff tar file" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo 1 > "$TEST_TMP_DIR1"/test.pid
	tar -cf "$TEST_TMP_DIR1"/rootfs-diff.tar -C "$TEST_TMP_DIR1" test.pid
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[8]} == *"Root FS diff size"* ]]
}

@test "Run checkpointctl inspect with multiple tar files" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar .  && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/*.tar
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Podman"* ]]
	[[ ${lines[14]} == *"Podman"* ]]
}
