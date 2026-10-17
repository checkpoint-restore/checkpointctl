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

@test "Size check comment shows the change in bytes and percent" {
	local sizes base candidate expected
	for sizes in '10748232 10800648 +52416 B (+0.49%)' '200 100 -100 B (-50.00%)' '100 100 +0 B (0.00%)'; do
		read -r base candidate expected <<< "$sizes"
		run env SIZE_RESULT=pass BASE_SIZE="$base" CANDIDATE_SIZE="$candidate" MAX_DIFF=524288 \
			BASE_SHA=base MERGE_SHA=merge bash "$BATS_TEST_DIRNAME/size-comment.sh" binary
		echo "$output"
		[ "$status" -eq 0 ]
		[[ "$output" == *"| Change | **$expected** |"* ]]
	done
}
