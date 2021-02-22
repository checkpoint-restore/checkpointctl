CHECKPOINTCTL="./checkpointctl"
TEST_TMP_DIR1=""
TEST_TMP_DIR2=""

function checkpointctl() {
	run "$CHECKPOINTCTL" "$@"
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
	checkpointctl show -t /does-not-exist
	[ "$status" -eq 1 ]
	[[ ${lines[0]} = "Error: Target /does-not-exist access error" ]]
	[[ ${lines[1]} = ": stat /does-not-exist: no such file or directory" ]]
}

@test "Run checkpointctl show" {
	cp test/test.json "$TEST_TMP_DIR1"/checkpointed.pods
	checkpointctl show -t "$TEST_TMP_DIR1" --show-pod-uid
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ checkpointed.pods ]]
	[[ ${lines[4]} =~ "uid1" ]]
	[[ ${lines[8]} =~ "uid2" ]]
}

@test "Run checkpointctl extract without -t" {
	cp test/test.json "$TEST_TMP_DIR1"/checkpointed.pods
	checkpointctl extract -t "$TEST_TMP_DIR1"
	[ "$status" -eq 1 ]
	[[ ${lines[8]} = "Error: Specifying an output file (-o|--output) is required" ]]
}

@test "Run checkpointctl extract with missing tar archives" {
	cp test/test.json "$TEST_TMP_DIR1"/checkpointed.pods
	checkpointctl extract -t "$TEST_TMP_DIR1" -o "$TEST_TMP_DIR1"/output.tar.zstd
	[ "$status" -eq 1 ]
	[[ ${lines[1]} =~ "Cannot access" ]]
}

@test "Run checkpointctl extract and show" {
	cp test/test.json "$TEST_TMP_DIR1"/checkpointed.pods
	touch "$TEST_TMP_DIR1"/sandbox1.tar
	touch "$TEST_TMP_DIR1"/sandbox2.tar
	checkpointctl extract -t "$TEST_TMP_DIR1" -o "$TEST_TMP_DIR1"/output.tar.zstd
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ "Extracting checkpoint data" ]]
	checkpointctl show -t "$TEST_TMP_DIR1" --show-pod-uid
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ checkpointed.pods ]]
	[[ ${lines[4]} =~ "uid1" ]]
	[[ ${lines[8]} =~ "uid2" ]]
}

@test "Run checkpointctl extract, insert and show" {
	# First extract a checkpoint
	cp test/test.json "$TEST_TMP_DIR1"/checkpointed.pods
	touch "$TEST_TMP_DIR1"/sandbox1.tar
	touch "$TEST_TMP_DIR1"/sandbox2.tar
	checkpointctl extract -t "$TEST_TMP_DIR1" -o "$TEST_TMP_DIR1"/output.tar.zstd
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ "Extracting checkpoint data" ]]

	# Create a destination directory with different UIDs
	cp test/test.json "$TEST_TMP_DIR2"/checkpointed.pods
	sed -e "s,uid1,1uid,g;s,uid2,2uid,g;" -i "$TEST_TMP_DIR2"/checkpointed.pods
	checkpointctl insert -t "$TEST_TMP_DIR2" -i "$TEST_TMP_DIR1"/output.tar.zstd
	[ "$status" -eq 0 ]

	# Check if the checkpoint has been correctly inserted
	checkpointctl show -t "$TEST_TMP_DIR2" --show-pod-uid
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ checkpointed.pods ]]
	[[ ${lines[4]} =~ "1uid" ]]
	[[ ${lines[8]} =~ "2uid" ]]
	[[ ${lines[12]} =~ "uid1" ]]
	[[ ${lines[16]} =~ "uid2" ]]
}

@test "Run checkpointctl show with empty tar file" {
	touch "$TEST_TMP_DIR1"/empty.tar
	checkpointctl show -t "$TEST_TMP_DIR1"/empty.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} =~ config.dump ]]
}

@test "Run checkpointctl show with tar file with empty config.dump" {
	touch "$TEST_TMP_DIR1"/config.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"config.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and no spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"spec.dump: no such file or directory" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and empty spec.dump" {
	cp test/config.dump "$TEST_TMP_DIR1"
	touch "$TEST_TMP_DIR1"/spec.dump
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"spec.dump: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and no network.status" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"network.status: no such file or directory" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and empty network.status" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	touch "$TEST_TMP_DIR1"/network.status
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"network.status: unexpected end of JSON input" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and valid network.status and no checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	cp test/network.status "$TEST_TMP_DIR1"
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 1 ]
	[[ ${lines[1]} == *"checkpoint: no such file or directory" ]]
}

@test "Run checkpointctl show with tar file with valid config.dump and valid spec.dump and valid network.status and checkpoint directory" {
	cp test/config.dump "$TEST_TMP_DIR1"
	cp test/spec.dump "$TEST_TMP_DIR1"
	cp test/network.status "$TEST_TMP_DIR1"
	mkdir "$TEST_TMP_DIR1"/checkpoint
	( cd "$TEST_TMP_DIR1" && tar cf "$TEST_TMP_DIR2"/test.tar . )
	checkpointctl show -t "$TEST_TMP_DIR2"/test.tar
	[ "$status" -eq 0 ]
	[[ ${lines[4]} == *"Unknown"* ]]
}
