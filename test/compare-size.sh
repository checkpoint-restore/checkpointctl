#!/bin/bash

set -euo pipefail

# The limit applies to the final merged PR, rather than individual commits.
MAX_DIFF=$((512 * 1024))
result=error
temp_dir=
source_dir=
repo_dir=
go_version=
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# Record a result for the GitHub Actions workflow, if there is one.
output() {
	if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
		printf '%s=%s\n' "$1" "$2" >> "$GITHUB_OUTPUT"
	fi
}

cleanup() {
	if [[ -n "$source_dir" ]]; then
		git -C "$repo_dir" worktree remove --force "$source_dir" >/dev/null 2>&1 || true
	fi
	if [[ -n "$temp_dir" ]]; then
		rm -rf -- "$temp_dir"
	fi
	output result "$result"
	output max_diff "$MAX_DIFF"
	output go_version "$go_version"
}
trap cleanup EXIT

fail() {
	echo "ERROR: $*" >&2
	exit 1
}

if [[ $# -ne 3 ]]; then
	fail "Usage: $0 binary|library MERGE_SHA HEAD_SHA"
fi
mode=$1
# Files that affect the measured size, or the size check itself.
build_inputs=('*.go' go.mod go.sum vendor test/compare-size.sh test/size-comment.sh)
case "$mode" in
	binary)
		binary_path=checkpointctl
		build_inputs+=('Makefile*' .github/workflows/check-size.yml)
		;;
	library)
		binary_path=test/lib-size/lib-size-test
		build_inputs+=(test/lib-size .github/workflows/check-lib-size.yml)
		;;
	*) fail "Unknown size check: $mode" ;;
esac

repo_dir=$(git rev-parse --show-toplevel)
merge_sha=$(git rev-parse --verify --end-of-options "$2^{commit}") || fail "Invalid merge commit: $2"
head_sha=$(git rev-parse --verify --end-of-options "$3^{commit}") || fail "Invalid PR head commit: $3"

# Derive the baseline from the pinned merge; the event's base SHA can be stale.
read -r base_sha head_parent extra_parents <<< "$(git show -s --format=%P "$merge_sha")"
if [[ "$head_parent" != "$head_sha" || -n "$extra_parents" ]]; then
	fail "Merge commit $merge_sha must have exactly two parents, with PR head $head_sha second"
fi

# A git error counts as a change, so that the sizes are still compared.
if git diff --quiet "$base_sha" "$merge_sha" -- "${build_inputs[@]}"; then
	result=skipped
	echo "SKIPPED: the PR does not change the build inputs"
	if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
		echo "::notice::No build inputs changed; skipping the $mode size check"
	fi
	exit 0
fi

# Prevent go.mod or toolchain directives from selecting a different compiler.
export GOTOOLCHAIN=local
go_version=$(go version)
echo "$go_version"
echo "BASE COMMIT: $base_sha"
echo "MERGED PR COMMIT: $merge_sha"

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/checkpointctl-size.XXXXXX")
source_dir=$temp_dir/source

measure() {
	local sha=$1
	local label=$2
	git -C "$repo_dir" worktree add --detach --quiet "$source_dir" "$sha"
	# Use the same path and a fresh source tree for each build.
	if [[ "$mode" == binary ]]; then
		make -C "$source_dir" || fail "Build failed for $label commit $sha"
	else
		# Keep the consumer identical, including when the base predates it.
		rm -rf -- "$source_dir/test/lib-size"
		mkdir -p "$source_dir/test/lib-size"
		cp "$script_dir/lib-size/main.go" "$source_dir/test/lib-size/main.go"
		(cd "$source_dir" && go build -o "$binary_path" ./test/lib-size) || fail "Build failed for $label commit $sha"
	fi
	measured_size=$(stat -c%s "$source_dir/$binary_path")
	echo "$label SIZE ($sha): $measured_size"
	git -C "$repo_dir" worktree remove --force "$source_dir"
}

measure "$base_sha" BASE
base_size=$measured_size
measure "$merge_sha" MERGED_PR
candidate_size=$measured_size
difference=$((candidate_size - base_size))
output base_sha "$base_sha"
output base_size "$base_size"
output candidate_size "$candidate_size"

if (( difference > MAX_DIFF )); then
	result=over_limit
	echo "FAIL: size difference of $difference B exceeds limit $MAX_DIFF B"
	exit 1
fi
result=pass
echo "PASS: size difference of $difference B within limit $MAX_DIFF B"
