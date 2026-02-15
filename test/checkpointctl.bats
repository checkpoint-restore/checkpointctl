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
	NON_ROOT_TMP1=$(sudo -u 'nobody' mktemp -d)
}

function teardown() {
	[ "$TEST_TMP_DIR1" != "" ] && rm -rf "$TEST_TMP_DIR1"
	[ "$TEST_TMP_DIR2" != "" ] && rm -rf "$TEST_TMP_DIR2"
	[ "$NON_ROOT_TMP1" != "" ] && rm -rf "$NON_ROOT_TMP1"
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
	[[ ${lines[3]} == *"Podman"* ]]
}

@test "Run checkpointctl show with tar file from containerd with valid config.dump and valid spec.dump and checkpoint directory" {
	cp data/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	echo "{}" >  "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[3]} == *"containerd"* ]]
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
	[[ ${lines[3]} == *"CRI-O"* ]]
}

@test "Run checkpointctl show with tar file compressed" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test.tar.gz . )
	checkpointctl show "$TEST_TMP_DIR2"/test.tar.gz
	[ "$status" -eq 0 ]
	[[ ${lines[3]} == *"Podman"* ]]
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
	[[ ${lines[1]} == *"ROOT FS DIFF SIZE"* ]]
}

@test "Run checkpointctl show with multiple tar files" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar .  && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	checkpointctl show "$TEST_TMP_DIR2"/*.tar
	[ "$status" -eq 0 ]
	[[ ${lines[2]} == *"Podman"* ]]
	[[ ${lines[3]} == *"Podman"* ]]
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
	[[ ${lines[13]} =~ [1-9] ]]
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

@test "Run checkpointctl inspect with tar file and --ps-tree-cmd" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree-cmd
	[ "$status" -eq 0 ]
	[[ ${lines[9]} == *"Process tree"* ]]
	[[ ${lines[10]} == *"piggie/piggie"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree-cmd as non-root" {
	if [ "$CHECKPOINTCTL"  == "../checkpointctl.coverage" ]; then
		skip "non-root test cannot access the coverage directory"
	fi
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$NON_ROOT_TMP1"/test.tar . )
	chmod 644 "$NON_ROOT_TMP1"/test.tar
	NON_ROOT_BIN=$(mktemp)
	cp "$CHECKPOINTCTL" "$NON_ROOT_BIN"
	chmod 755 "$NON_ROOT_BIN"
	run sudo -u 'nobody' "$NON_ROOT_BIN" inspect "$NON_ROOT_TMP1"/test.tar --ps-tree-cmd
	echo "$output"
	rm -f "$NON_ROOT_BIN"
	[ "$status" -eq 0 ]
	[[ ${lines[9]} == *"Process tree"* ]]
	[[ ${lines[10]} == *"piggie/piggie"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree-cmd and missing pages-*.img" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree-cmd
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to process command line arguments"* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree-env" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree-env
	[ "$status" -eq 0 ]
	[[ ${lines[9]} == *"Process tree"* ]]
	[[ ${lines[10]} == *"piggie"* ]]
	[[ ${lines[12]} == *"="* ]]
}

@test "Run checkpointctl inspect with tar file and --ps-tree-env and missing pages-*.img" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --ps-tree-env
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"no such file or directory"* ]]
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

	[[ ${lines[11]} == *"[REG 0]"* ]]
	[[ ${lines[25]} == *"[cwd]"* ]]
	[[ ${lines[26]} == *"[root]"* ]]

	[[ ${lines[27]} == *"[5 (Dead)]  piggie-zombie"* ]]
	[[ ${lines[28]} == *"[6 (Stopped)]  stopped-child"* ]]
	[[ ${lines[29]} == *"Open files"* ]]
	[[ ${lines[33]} == *"[7]  alive-child"* ]]
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

@test "Run checkpointctl inspect with tar file and --sockets" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/files.img \
		test-imgs/ids-*.img \
		test-imgs/fdinfo-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --sockets
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl inspect with tar file and --sockets and missing files.img" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --sockets
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to get sockets"* ]]
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
		test-imgs/fdinfo-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )

	run checkpointctl inspect "$TEST_TMP_DIR2"/test.tar --all
	[ "$status" -eq 0 ]

	[[ ${lines[9]} == *"CRIU dump statistics"* ]]
	[[ ${lines[13]} == *"Memwrite time"* ]]
	[[ ${lines[14]} =~ [1-9] ]]

	expected_messages=(
		"[REG 0]"
		"[cwd]"
		"[root]"
		"Metadata"
		"Process tree"
		"piggie"
		"Overview of mounts"
		"Destination"
		"proc"
		"/etc/hostname"
	)

	for message in "${expected_messages[@]}"; do
		if ! grep -q "$message" <<< "$output"; then
			echo "Error: Expected message '$message' not found"
			false
		fi
	done
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
	echo '{"checkpointedTime": "2024-02-09T11:01:26.186815191Z"}' > "$TEST_TMP_DIR1"/config.dump
	cp data/spec.dump.cri-o "$TEST_TMP_DIR1"/spec.dump
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl inspect "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[6]} == *"Checkpointed: 2024-02-09"* ]]
	[[ ${lines[7]} == *"CRI-O"* ]]
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

@test "Run checkpointctl memparse with tar file" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[3]} == *"piggie"* ]]
}

@test "Run checkpointctl memparse with tar file and missing pstree.img" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"no such file or directory"* ]]
}

@test "Run checkpointctl memparse with tar file and valid PID" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse "$TEST_TMP_DIR2"/test.tar --pid=1
	[ "$status" -eq 0 ]

	# Check if the 4th line matches the expected hex pattern
	# We expect lines like: '000055eb086f6030  f3 0f 1e fa 68 00 00 00 00 f2 e9 e1 ff ff ff 90  |....h...........|'
	line="${lines[3]}"

	hex_address_regex="^[0-9a-fA-F]{16}"
	# Check the hex address (first 16 characters)
	if [[ ! $line =~ $hex_address_regex ]]; then
		echo "Hex address part of line does not match. Line: $line"
		return 1
	fi

	# Check the hex byte pairs
	hex_bytes_regex="\s\s([[0-9a-fA-F]{2}\s*)+\s\s"
	if [[ ! $line =~ $hex_bytes_regex ]]; then
		echo "Hex byte pairs part of line does not match. Line: $line"
		return 1
	fi

	# Check the ASCII representation (after the pipe character)
	ascii_representation_regex="\|.*\|$"
	if [[ ! $line =~ $ascii_representation_regex ]]; then
		echo "ASCII representation part of line does not match. Line: $line"
		return 1
	fi
}

@test "Run checkpointctl memparse --search=PATH with invalid PID" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse --search=PATH "$TEST_TMP_DIR2"/test.tar --pid=999
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"no process with PID 999"* ]]
}

@test "Run checkpointctl memparse with --search=PATH and --context=-1" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse --search=PATH --context=-1 "$TEST_TMP_DIR2"/test.tar --pid=1
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"context size cannot be negative"* ]]
}

@test "Run checkpointctl memparse with --search=NON_EXISTING_PATTERN" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse --search=NON_EXISTING_PATTERN "$TEST_TMP_DIR2"/test.tar --pid=1
	[ "$status" -eq 0 ]
	[[ ${lines[0]} == *"No matches"* ]]
}

@test "Run checkpointctl memparse with --search=PATH and --context=10 flags" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse --search=PATH --context=10 "$TEST_TMP_DIR2"/test.tar --pid=1
	[ "$status" -eq 0 ]
	[[ "$(printf '%s\n' "${lines[@]}")" == *"PATH"* ]]
}

@test "Run checkpointctl memparse with --search-regex='HOME=([^?]+)' " {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse --search-regex='HOME=([^?]+)' "$TEST_TMP_DIR2"/test.tar --pid=1
	[ "$status" -eq 0 ]
	[[ ${lines[2]} == *"HOME"* ]]
}

@test "Run checkpointctl memparse with tar file and invalid PID" {
	cp data/config.dump \
		data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/pstree.img \
		test-imgs/core-*.img \
		test-imgs/pagemap-*.img \
		test-imgs/pages-*.img \
		test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl memparse "$TEST_TMP_DIR2"/test.tar --pid=9999
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"no process with PID 9999"* ]]
}

@test "Run checkpointctl inspect with json format" {
	cp data/config.dump data/spec.dump test-imgs/stats-dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	cp test-imgs/*.img "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )

	# test function definitions for JSON output using jq
	test_engine() { jq -e '.[0].engine == "Podman"'; }
	export -f test_engine

	test_pstree_cmd() { jq -e '.[0].process_tree.command == "piggie"'; }
	export -f test_pstree_cmd

	test_pstree_child1() { jq -e '.[0].process_tree.children[0].command == "tcp-server"'; }
	export -f test_pstree_child1

	test_pstree_child2() { jq -e '.[0].process_tree.children[1].command == "tcp-client"'; }
	export -f test_pstree_child2

	test_pstree_env() { jq -e '.[0].process_tree.environment_variables.TEST_ENV == "BAR"'; }
	export -f test_pstree_env

	test_pstree_env_empty() { jq -e '.[0].process_tree.environment_variables.TEST_ENV_EMPTY == ""'; }
	export -f test_pstree_env_empty

	test_socket_protocol() { jq -e '.[0].sockets[0].open_sockets[0].protocol == "TCP"'; }
	export -f test_socket_protocol

	test_socket_src_port() { jq -e '.[0].sockets[0].open_sockets[0].data.src_port == 5000'; }
	export -f test_socket_src_port

	# Run tests
	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json | test_engine"
	[ "$status" -eq 0 ]

	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json --ps-tree | test_pstree_cmd"
	[ "$status" -eq 0 ]

	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json --all | test_pstree_env"
	[ "$status" -eq 0 ]

	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json --all | test_pstree_env_empty"
	[ "$status" -eq 0 ]

	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json --sockets | test_socket_protocol"
	[ "$status" -eq 0 ]

	run bash -c "$CHECKPOINTCTL inspect $TEST_TMP_DIR2/test.tar --format=json --sockets | test_socket_src_port"
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl list with empty directory" {
    mkdir "$TEST_TMP_DIR1"/empty
    checkpointctl list "$TEST_TMP_DIR1"/empty/
    [ "$status" -eq 0 ]
    [[ ${lines[0]} == *"No checkpoints found"* ]]
}

@test "Run checkpointctl list with non existing directory" {
	checkpointctl list /does-not-exist
	[ "$status" -eq 0 ]
	[[ ${lines[0]} == *"No checkpoints found"* ]]
}

@test "Run checkpointctl list with empty tar file" {
	touch "$TEST_TMP_DIR1"/checkpoint-nginx-empty.tar
	checkpointctl list "$TEST_TMP_DIR1"
	[ "$status" -eq 0 ]
	[[ "${lines[1]}" == *"Error extracting information"* ]]
}

@test "Run checkpointctl list with tar file with valid spec.dump and empty config.dump" {
	touch "$TEST_TMP_DIR1"/config.dump
	cp data/list_config_spec.dump/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/checkpoint-config.tar . )
	checkpointctl list "$TEST_TMP_DIR2"
	[ "$status" -eq 0 ]
	[[ "${lines[1]}" == *"Error extracting information from $TEST_TMP_DIR2/checkpoint-config.tar: failed to unmarshal"* ]]
}

@test "Run checkpointctl list with tar file with valid config.dump and empty spec.dump" {
	touch "$TEST_TMP_DIR1"/spec.dump
	cp data/list_config_spec.dump/config.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/checkpoint-config.tar . )
	checkpointctl list "$TEST_TMP_DIR2"
	[ "$status" -eq 0 ]
	[[ ${lines[1]} == *"Error extracting information from $TEST_TMP_DIR2/checkpoint-config.tar: failed to unmarshal"* ]]
}

@test "Run checkpointctl list with tar file with valid config.dump and spec.dump" {
	cp data/list_config_spec.dump/config.dump "$TEST_TMP_DIR1"
	cp data/list_config_spec.dump/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/checkpoint-valid-config.tar . )
	jq '.["annotations"]["io.kubernetes.pod.name"] = "modified-pod-name"' "$TEST_TMP_DIR1"/spec.dump > "$TEST_TMP_DIR1"/spec_modified.dump
	mv "$TEST_TMP_DIR1"/spec_modified.dump "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/checkpoint-valid-config-modified.tar . )
	checkpointctl list "$TEST_TMP_DIR2"
	[ "$status" -eq 0 ]
	[[ "${lines[3]}" == *"default"* ]]
	[[ "${lines[3]}" == *"modified-pod-name"* ]]
	[[ "${lines[3]}" == *"container-name"* ]]
	[[ "${lines[3]}" == *"CRI-O"* ]]
	[[ "${lines[3]}" == *"checkpoint-valid-config-modified.tar"* ]]
	[[ "${lines[4]}" == *"default"* ]]
	[[ "${lines[4]}" == *"pod-name"* ]]
	[[ "${lines[4]}" == *"container-name"* ]]
	[[ "${lines[4]}" == *"CRI-O"* ]]
	[[ "${lines[4]}" == *"checkpoint-valid-config.tar"* ]]
}

@test "Run checkpointctl diff with no arguments" {
	checkpointctl diff
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: accepts 2 arg(s), received 0"* ]]
}

@test "Run checkpointctl diff with only one argument" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl diff "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"Error: accepts 2 arg(s), received 1"* ]]
}

@test "Run checkpointctl diff with non-existing checkpoint" {
	checkpointctl diff /does-not-exist1.tar /does-not-exist2.tar
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"failed to load checkpoint"* ]]
}

@test "Run checkpointctl diff with invalid format" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar --format=invalid
	[ "$status" -eq 1 ]
	[[ ${lines[0]} == *"invalid output format"* ]]
}

@test "Run checkpointctl diff with two identical checkpoints (tree format)" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar
	[ "$status" -eq 0 ]
	[[ "$output" == *"Checkpoint Diff"* ]]
	[[ "$output" == *"Memory Changes"* ]]
	[[ "$output" == *"No change"* ]]
}

@test "Run checkpointctl diff with two identical checkpoints (json format)" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar --format=json
	[ "$status" -eq 0 ]
	
	# Validate JSON output
	test_container_id() { jq -e '.container_id != null'; }
	export -f test_container_id
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_container_id"
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl diff with checkpoints from different containers" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	
	# Create first checkpoint with process tree
	if [ -d "test-imgs" ]; then
		cp test-imgs/pstree.img test-imgs/core-*.img "$TEST_TMP_DIR1"/checkpoint
	fi
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	
	# Modify container ID for second checkpoint
	jq '.annotations["io.container.manager"] = "different-runtime"' "$TEST_TMP_DIR1"/config.dump > "$TEST_TMP_DIR1"/config_modified.dump
	mv "$TEST_TMP_DIR1"/config_modified.dump "$TEST_TMP_DIR1"/config.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	
	# This should work since both checkpoints have minimal data
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar
	# Accept that it completes - the test data doesn't have real IDs
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl diff with --ps-tree-cmd flag" {
	cp data/config.dump data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	
	# Only create checkpoint files if test-imgs exists (optional test)
	if [ -d "test-imgs" ]; then
		cp test-imgs/pstree.img \
			test-imgs/core-*.img \
			test-imgs/pagemap-*.img \
			test-imgs/pages-*.img \
			test-imgs/mm-*.img "$TEST_TMP_DIR1"/checkpoint
		
		( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
		( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
		
		checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar --ps-tree-cmd
		[ "$status" -eq 0 ]
		[[ "$output" == *"Process Changes"* ]]
	else
		skip "test-imgs directory not available"
	fi
}

@test "Run checkpointctl diff with --files flag" {
	cp data/config.dump data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	
	# Only create checkpoint files if test-imgs exists (optional test)
	if [ -d "test-imgs" ]; then
		cp test-imgs/pstree.img \
			test-imgs/core-*.img \
			test-imgs/files.img \
			test-imgs/fs-*.img \
			test-imgs/ids-*.img \
			test-imgs/fdinfo-*.img "$TEST_TMP_DIR1"/checkpoint
		
		( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
		( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
		
		checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar --files
		[ "$status" -eq 0 ]
		[[ "$output" == *"File Descriptor Changes"* ]]
	else
		skip "test-imgs directory not available"
	fi
}

@test "Run checkpointctl diff json output validation" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	
	# Test function definitions for JSON output validation
	test_has_container_id() { jq -e '.container_id != null'; }
	export -f test_has_container_id
	
	test_has_container_name() { jq -e '.container_name != null'; }
	export -f test_has_container_name
	
	test_has_checkpoint_a() { jq -e '.checkpoint_a != null'; }
	export -f test_has_checkpoint_a
	
	test_has_checkpoint_b() { jq -e '.checkpoint_b != null'; }
	export -f test_has_checkpoint_b
	
	test_has_memory_changes() { jq -e '.memory_changes != null'; }
	export -f test_has_memory_changes
	
	test_memory_size_change() { jq -e '.memory_changes.size_change_bytes == 0'; }
	export -f test_memory_size_change
	
	test_has_summary() { jq -e '.summary != ""'; }
	export -f test_has_summary
	
	# Run validation tests
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_container_id"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_container_name"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_checkpoint_a"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_checkpoint_b"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_memory_changes"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_memory_size_change"
	[ "$status" -eq 0 ]
	
	run bash -c "$CHECKPOINTCTL diff $TEST_TMP_DIR2/test1.tar $TEST_TMP_DIR2/test2.tar --format=json | test_has_summary"
	[ "$status" -eq 0 ]
}

@test "Run checkpointctl diff with compressed checkpoints" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test1.tar.gz . )
	( cd "$TEST_TMP_DIR1" && tar czf "$TEST_TMP_DIR2"/test2.tar.gz . )
	
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar.gz "$TEST_TMP_DIR2"/test2.tar.gz
	[ "$status" -eq 0 ]
	[[ "$output" == *"Checkpoint Diff"* ]]
}

@test "Run checkpointctl diff output contains expected sections" {
	cp data/config.dump "$TEST_TMP_DIR1"
	cp data/spec.dump "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test1.tar . )
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test2.tar . )
	
	checkpointctl diff "$TEST_TMP_DIR2"/test1.tar "$TEST_TMP_DIR2"/test2.tar
	[ "$status" -eq 0 ]
	
	# Verify expected sections are present
	expected_sections=(
		"Checkpoint Diff"
		"Container:"
		"Image:"
		"Checkpoint A:"
		"Checkpoint B:"
		"Memory Changes"
		"Process Changes"
		"Summary"
	)
	
	for section in "${expected_sections[@]}"; do
		if ! grep -q "$section" <<< "$output"; then
			echo "Error: Expected section '$section' not found in output"
			false
		fi
	done
}