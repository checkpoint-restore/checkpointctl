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
		description='Compares the size of a small program that imports the `lib` package, to show how this PR affects projects that use checkpointctl as a library.'
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
	base=$((10#$base))
	candidate=$((10#$candidate))
	limit=$((10#$limit))
	difference=$((candidate - base))
	printf -v change '%+d B' "$difference"
	if (( base > 0 )); then
		absolute=${difference#-}
		# Round the percentage to two decimal places using integer arithmetic.
		percent_hundredths=$(((absolute * 10000 + base / 2) / base))
		sign=
		if (( difference > 0 )); then sign=+; fi
		if (( difference < 0 )); then sign=-; fi
		printf -v percentage '%s%d.%02d%%' \
			"$sign" "$((percent_hundredths / 100))" "$((percent_hundredths % 100))"
		change+=" ($percentage)"
	fi
	printf '\n| | Size |\n|---|---:|\n'
	printf '| Base ([`%.12s`](%s/commit/%s)) | %s B |\n' "${BASE_SHA:?}" "$repo_url" "$BASE_SHA" "$base"
	printf '| Merged PR ([`%.12s`](%s/commit/%s)) | %s B |\n' "${MERGE_SHA:?}" "$repo_url" "$MERGE_SHA" "$candidate"
	printf '| Change | **%s** |\n| Allowed increase | %s B |\n' "$change" "$limit"
fi

if [[ -n "${GO_VERSION:-}" ]]; then
	printf '\nCompiler: `%s`.\n' "$GO_VERSION"
fi
printf '\n[View CI logs](%s).\n' "$run_url"

if [[ "$result" == over_limit ]]; then
	printf '\nIf this increase is expected, a maintainer can add `%s` to bypass the check.\n' "$label"
fi
