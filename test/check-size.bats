#!/usr/bin/env bats
# shellcheck disable=SC2030,SC2031
# Each test builds synthetic Git snapshots without a compiler or network access.

setup() {
	SCRIPTS_DIR=$(cd "$BATS_TEST_DIRNAME" && pwd)
	TEST_ROOT=$(mktemp -d)
	export TMPDIR="$TEST_ROOT/tmp" GITHUB_OUTPUT="$TEST_ROOT/output"
	export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
	mkdir -p "$TMPDIR" "$TEST_ROOT/bin" "$TEST_ROOT/repo"
	cat > "$TEST_ROOT/bin/go" <<'EOF'
#!/bin/bash
set -eu
case "$1" in
version) echo 'go version go1.26.0 linux/amd64' ;;
build) truncate -s "$(cat size.go)" "$3" ;;
*) exit 1 ;;
esac
EOF
	chmod +x "$TEST_ROOT/bin/go"
	export PATH="$TEST_ROOT/bin:$PATH"
	cd "$TEST_ROOT/repo" || return
	git init -q -b main
	git config user.name 'Size check test'
	git config user.email 'size-check@example.invalid'
	git config commit.gpgsign false
	# shellcheck disable=SC2016
	printf 'all:\n\ttruncate -s $$(cat size.go) checkpointctl\n' > Makefile
	# The fake build reads the size from a .go file, so it is a build input.
	printf '100000\n' > size.go
	commit 'Initial size'
}

teardown() {
	cd "$BATS_TEST_DIRNAME" || return
	rm -rf "$TEST_ROOT"
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

check_sizes() {
	local expected_result=$1 expected_base=$2 expected_candidate=$3 expected_status=$4
	local script
	for script in check-size.sh check-lib-size.sh; do
		: > "$GITHUB_OUTPUT"
		run "$SCRIPTS_DIR/$script" "$MERGE_SHA" "$HEAD_SHA"
		echo "$output"
		[ "$status" -eq "$expected_status" ]
		[ "$(sed -n 's/^result=//p' "$GITHUB_OUTPUT")" = "$expected_result" ]
		[ "$(sed -n 's/^base_sha=//p' "$GITHUB_OUTPUT")" = "$BASE_PARENT_SHA" ]
		[ "$(sed -n 's/^base_size=//p' "$GITHUB_OUTPUT")" -eq "$expected_base" ]
		[ "$(sed -n 's/^candidate_size=//p' "$GITHUB_OUTPUT")" -eq "$expected_candidate" ]
	done
}

@test "Size checks use the merge's first parent when the payload base is stale" {
	BASE_SHA=$(git rev-parse HEAD)
	export BASE_SHA
	git switch -qc feature
	printf '800000\n' > size.go
	commit 'Grow binary'
	git switch -q main
	touch README.md
	commit 'Advance base branch'
	merge_pr
	[ "$BASE_SHA" != "$BASE_PARENT_SHA" ]
	check_sizes over_limit 100000 800000 1
}

@test "Size checks preserve resolved merges in the base branch" {
	git switch -qc topic
	printf '110000\n' > size.go
	commit 'Change topic size'
	git switch -q main
	printf '160000\n' > size.go
	commit 'Change base size'
	run git merge --no-ff topic -m 'Merge topic'
	[ "$status" -eq 1 ]
	printf '165000\n' > size.go
	commit 'Resolve size conflict'
	git switch -qc feature
	touch helper.go
	commit 'Add an empty Go file'
	merge_pr
	check_sizes pass 165000 165000 0
}

@test "Size checks skip PRs that do not change the build inputs" {
	git switch -qc feature
	touch README.md
	commit 'Docs only'
	merge_pr
	local script
	for script in check-size.sh check-lib-size.sh; do
		: > "$GITHUB_OUTPUT"
		run "$SCRIPTS_DIR/$script" "$MERGE_SHA" "$HEAD_SHA"
		echo "$output"
		[ "$status" -eq 0 ]
		[ "$(sed -n 's/^result=//p' "$GITHUB_OUTPUT")" = skipped ]
		[ -z "$(sed -n 's/^base_size=//p' "$GITHUB_OUTPUT")" ]
	done
}

@test "Skipped size check comment is marked for update only" {
	run env GITHUB_REPOSITORY=owner/repo GITHUB_RUN_ID=1 SIZE_RESULT=skipped \
		bash "$SCRIPTS_DIR/size-comment.sh" binary
	echo "$output"
	[ "$status" -eq 0 ]
	[ "${lines[0]}" = '<!-- checkpointctl-size:binary -->' ]
	[ "${lines[1]}" = '<!-- checkpointctl-size-result:skipped -->' ]
	[[ "$output" == *'**Skipped**'* ]]
}

@test "Size check comment does not assume a size limit" {
	run env GITHUB_REPOSITORY=owner/repo GITHUB_RUN_ID=1 SIZE_RESULT=pass \
		BASE_SIZE=100 CANDIDATE_SIZE=200 BASE_SHA=base MERGE_SHA=merge \
		bash "$SCRIPTS_DIR/size-comment.sh" binary
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *'Unable to complete the comparison'* ]]
}

@test "Size check comment shows the change in bytes and percent" {
	local sizes base candidate expected
	for sizes in '10748232 10800648 +52416 B (+0.49%)' '200 100 -100 B (-50.00%)' '100 100 +0 B (0.00%)'; do
		read -r base candidate expected <<< "$sizes"
		run env GITHUB_REPOSITORY=owner/repo GITHUB_RUN_ID=1 SIZE_RESULT=pass \
			BASE_SIZE="$base" CANDIDATE_SIZE="$candidate" MAX_DIFF=524288 \
			BASE_SHA=base MERGE_SHA=merge bash "$SCRIPTS_DIR/size-comment.sh" binary
		echo "$output"
		[ "$status" -eq 0 ]
		[[ "$output" == *"| Change | **$expected** |"* ]]
	done
}
