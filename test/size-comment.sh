#!/bin/bash
# shellcheck disable=SC2016
# Markdown backticks in printf formats are literal.

set -euo pipefail

case "${1:-}" in
	binary)
		title='Binary size check'
		description='Compares the size of the `checkpointctl` binary, built with `make`, before and after this PR.'
		label=bloat-ok
		;;
	library)
		title='Library size check'
		description='Compares the executable size of a small program that reads container configuration and OCI specs through `lib`.'
		label=lib-bloat-ok
		;;
	*) echo "Usage: $0 binary|library" >&2; exit 1 ;;
esac

result=${SIZE_RESULT:-error}
base=${BASE_SIZE:-}
candidate=${CANDIDATE_SIZE:-}
# compare-size.sh reports the limit, so that it is defined in one place.
limit=${MAX_DIFF:-}
if [[ "$result" == pass || "$result" == over_limit ]] &&
	[[ ! "$base" =~ ^[0-9]+$ || ! "$candidate" =~ ^[0-9]+$ || ! "$limit" =~ ^[0-9]+$ ]]; then
	result=error
fi
format_size() {
	awk -v bytes="$1" 'BEGIN {
		if (bytes >= 1048576 || sprintf("%.2f", bytes / 1024) + 0 >= 1024) {
			printf "%.2f MiB", bytes / 1048576
		} else if (bytes >= 1024) {
			if (bytes % 1024 == 0) printf "%d KiB", bytes / 1024
			else printf "%.2f KiB", bytes / 1024
		} else {
			printf "%d B", bytes
		}
	}'
}

repo_url=${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:?}
run_url=$repo_url/actions/runs/${GITHUB_RUN_ID:?}/attempts/${GITHUB_RUN_ATTEMPT:-1}
printf '<!-- checkpointctl-size:%s -->\n' "$1"
# The comment workflow only uses a skipped result to update an earlier comment.
if [[ "$result" == skipped ]]; then
	echo '<!-- checkpointctl-size-result:skipped -->'
fi
printf '## %s\n\n%s\n\n' "$title" "$description"

case "$result" in
	pass) echo '**Passed** — the size increase is within the limit.' ;;
	over_limit) echo '**Failed** — the size increase exceeds the limit.' ;;
	bypassed) printf '**Bypassed** — the `%s` label disables this size check.\n' "$label" ;;
	skipped) echo '**Skipped** — this PR does not change the build inputs.' ;;
	*)
		echo '**Unable to complete the comparison.** See the CI logs for the build or revision error.'
		result=error
		;;
esac

if [[ "$result" == pass || "$result" == over_limit ]]; then
	difference=$((candidate - base))
	if (( difference == 0 )); then
		change='No change'
	elif (( difference > -1024 && difference < 1024 )); then
		if (( difference > 0 )); then
			change='Less than 1 KiB increase'
		else
			change='Less than 1 KiB decrease'
		fi
	elif (( difference > 0 )); then
		change="+$(format_size "$difference")"
	else
		change="-$(format_size "$((-difference))")"
	fi
	if (( base > 0 && (difference <= -1024 || difference >= 1024) )); then
		change+=$(awk -v diff="$difference" -v base="$base" 'BEGIN {
			percent = diff * 100 / base
			rounded = sprintf("%.2f", percent)
			if (rounded + 0 != 0) printf " (%s%s%%)", (percent > 0 ? "+" : ""), rounded
		}')
	fi
	printf '\n| | Size |\n|---|---:|\n'
	printf '| Base ([`%.12s`](%s/commit/%s)) | %s |\n' "${BASE_SHA:?}" "$repo_url" "$BASE_SHA" "$(format_size "$base")"
	printf '| Merged PR ([`%.12s`](%s/commit/%s)) | %s |\n' "${MERGE_SHA:?}" "$repo_url" "$MERGE_SHA" "$(format_size "$candidate")"
	printf '| Change | **%s** |\n| Allowed increase | %s |\n' "$change" "$(format_size "$limit")"
fi

if [[ -n "${GO_VERSION:-}" ]]; then
	printf '\nCompiler: `%s`.\n' "$GO_VERSION"
fi
printf '\n[View CI logs](%s).\n' "$run_url"

if [[ "$result" == over_limit ]]; then
	printf '\nIf this increase is expected, a maintainer can add `%s` to bypass the check.\n' "$label"
fi
