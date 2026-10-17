#!/usr/bin/env bats
# Each test builds synthetic Git snapshots without a compiler or network access.

setup() {
	export GITHUB_OUTPUT="$BATS_TEST_TMPDIR/output" TMPDIR="$BATS_TEST_TMPDIR"
	export GITHUB_REPOSITORY=owner/repo GITHUB_RUN_ID=1
	export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
	mkdir "$BATS_TEST_TMPDIR/bin" "$BATS_TEST_TMPDIR/repo"
	cat > "$BATS_TEST_TMPDIR/bin/go" <<'EOF'
#!/bin/bash
set -eu
case "$1" in
version) echo 'go version go1.26.0 linux/amd64' ;;
build) truncate -s "$(cat lib/size.go)" "$3" ;;
*) exit 1 ;;
esac
EOF
	chmod +x "$BATS_TEST_TMPDIR/bin/go"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	cd "$BATS_TEST_TMPDIR/repo" || return
	git init -q -b main
	git config user.name 'Size check test'
	git config user.email 'size-check@example.invalid'
	# shellcheck disable=SC2016
	printf 'all:\n\ttruncate -s $$(cat lib/size.go) checkpointctl\n' > Makefile
	# The fake build reads the size from a .go file, so it is a build input.
	mkdir lib
	printf '100000\n' > lib/size.go
	commit 'Initial size'
}

commit() {
	git add .
	git commit -qm "$1"
}

merge_pr() {
	HEAD_SHA=$(git rev-parse feature)
	git switch -q main
	BASE_PARENT_SHA=$(git rev-parse HEAD)
	git merge -q --no-ff feature -m 'Merge PR'
	MERGE_SHA=$(git rev-parse HEAD)
}

output_value() {
	sed -n "s/^$1=//p" "$GITHUB_OUTPUT"
}

# Run both size checks on the merged PR. Sizes are empty when not measured.
check_sizes() {
	local expected_result=$1 expected_status=$2 expected_base=${3:-} expected_candidate=${4:-}
	local mode
	for mode in binary library; do
		: > "$GITHUB_OUTPUT"
		run "$BATS_TEST_DIRNAME/compare-size.sh" "$mode" "$MERGE_SHA" "$HEAD_SHA"
		echo "$output"
		[ "$status" -eq "$expected_status" ]
		[ "$(output_value result)" = "$expected_result" ]
		[ "$(output_value base_size)" = "$expected_base" ]
		[ "$(output_value candidate_size)" = "$expected_candidate" ]
		if [ -n "$expected_base" ]; then
			[ "$(output_value base_sha)" = "$BASE_PARENT_SHA" ]
		fi
	done
}

@test "Size checks compare the merge with its first parent" {
	git switch -qc feature
	printf '800000\n' > lib/size.go
	commit 'Grow binary'
	git switch -q main
	touch README.md
	commit 'Advance base branch'
	merge_pr
	check_sizes over_limit 1 100000 800000
}

@test "Size checks preserve resolved merges in the base branch" {
	git switch -qc topic
	printf '110000\n' > lib/size.go
	commit 'Change topic size'
	git switch -q main
	printf '160000\n' > lib/size.go
	commit 'Change base size'
	run git merge --no-ff topic -m 'Merge topic'
	[ "$status" -eq 1 ]
	printf '165000\n' > lib/size.go
	commit 'Resolve size conflict'
	git switch -qc feature
	touch lib/helper.go
	commit 'Add an empty Go file'
	merge_pr
	check_sizes pass 0 165000 165000
}

@test "Size checks skip PRs that do not change the build inputs" {
	git switch -qc feature
	touch README.md
	commit 'Docs only'
	merge_pr
	check_sizes skipped 0
}

@test "Skipped size check comment is marked for update only" {
	run env SIZE_RESULT=skipped bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
	echo "$output"
	[ "$status" -eq 0 ]
	[ "${lines[0]}" = '<!-- checkpointctl-size:binary -->' ]
	[ "${lines[1]}" = '<!-- checkpointctl-size-result:skipped -->' ]
	[[ "$output" == *'**Skipped**'* ]]
}

@test "Library size check skips CLI-only changes" {
	git switch -qc feature
	mkdir cmd
	touch cmd/inspect.go
	commit 'Change CLI code'
	merge_pr
	run "$BATS_TEST_DIRNAME/compare-size.sh" library "$MERGE_SHA" "$HEAD_SHA"
	[ "$status" -eq 0 ]
	[ "$(output_value result)" = skipped ]
	: > "$GITHUB_OUTPUT"
	run "$BATS_TEST_DIRNAME/compare-size.sh" binary "$MERGE_SHA" "$HEAD_SHA"
	[ "$status" -eq 0 ]
	[ "$(output_value result)" = pass ]
}

@test "Size checks skip changes confined to Go tests" {
	git switch -qc feature
	touch lib/metadata_test.go
	commit 'Change a Go test'
	merge_pr
	check_sizes skipped 0
}

@test "Library size check still measures dependency changes" {
	git switch -qc feature
	touch go.mod
	commit 'Change dependencies'
	merge_pr
	check_sizes pass 0 100000 100000
}

@test "Size check comment does not assume a size limit" {
	run env SIZE_RESULT=pass BASE_SIZE=100 CANDIDATE_SIZE=200 BASE_SHA=base MERGE_SHA=merge \
		bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *'Unable to complete the comparison'* ]]
}

@test "Size check comment uses readable units and suppresses tiny percentages" {
	local sizes base candidate expected
	for sizes in '10485760 10616832 +128 KiB (+1.25%)' '10616832 10485760 -128 KiB (-1.23%)' '100 100 No change' '10801280 10800952 Less than 1 KiB decrease' '10800952 10801280 Less than 1 KiB increase' '0 1048576 +1.00 MiB' '52428800 52429824 +1 KiB' '52428800 52427776 -1 KiB' '52428800 52431872 +3 KiB (+0.01%)'; do
		read -r base candidate expected <<< "$sizes"
		run env SIZE_RESULT=pass BASE_SIZE="$base" CANDIDATE_SIZE="$candidate" MAX_DIFF=524288 \
			BASE_SHA=base MERGE_SHA=merge bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
		echo "$output"
		[ "$status" -eq 0 ]
		[[ "$output" == *"| Change | **$expected** |"* ]]
	done
}

@test "Size check report formats totals and the limit in binary units" {
	run env SIZE_RESULT=over_limit BASE_SIZE=10485760 CANDIDATE_SIZE=11534336 MAX_DIFF=524288 \
		BASE_SHA=base MERGE_SHA=merge bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
	[ "$status" -eq 0 ]
	[[ "$output" == *'| 10.00 MiB |'* ]]
	[[ "$output" == *'| 11.00 MiB |'* ]]
	[[ "$output" == *'| Change | **+1.00 MiB (+10.00%)** |'* ]]
	[[ "$output" == *'| Allowed increase | 512 KiB |'* ]]
}

@test "Size check report promotes KiB rounded to the next MiB" {
	run env SIZE_RESULT=pass BASE_SIZE=1048570 CANDIDATE_SIZE=1048575 MAX_DIFF=1048575 \
		BASE_SHA=base MERGE_SHA=merge bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
	[ "$status" -eq 0 ]
	[[ "$output" == *'| 1023.99 KiB |'* ]]
	[[ "$output" == *'| 1.00 MiB |'* ]]
	[[ "$output" == *'| Allowed increase | 1.00 MiB |'* ]]
	[[ "$output" != *'1024.00 KiB'* ]]
}
