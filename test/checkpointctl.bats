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


