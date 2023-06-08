#!/bin/bash
#
# A shell script to check if the binary size
# has increased beyond a certain threshold
# across commits. Meant to be used in CI with
# git rebase <base branch>^ -x check-size.sh

BIN_NAME=checkpointctl
PREV_SIZE_FILE=prev_size
# Maximum allowable size difference, in bytes
MAX_DIFF=51200

# Build the checkpointctl binary. If the commit is not self-contained,
# the build will fail, in which case there is no point checking for a
# change in the size of the binary.
if ! make; then
	echo "ERROR: Compilation failed at $(git rev-parse --short HEAD)"
	echo "Make sure that the compilation is successful for each commit."
	exit 1
fi

# Fail fast on errors
set -e

# Store the binary size
BIN_SIZE=$(stat -c%s "$BIN_NAME")
# Print the size along with the commit hash
echo "BINARY SIZE ($(git rev-parse --short HEAD)): $BIN_SIZE"

if [[ -f "$PREV_SIZE_FILE" ]]; then
	# Read the previous size from the file
	PREV_SIZE=$(cat "$PREV_SIZE_FILE")
	# Calculate the difference between current and previous size
	DIFF=$((BIN_SIZE - PREV_SIZE))
	if [[ $DIFF -gt $MAX_DIFF ]]; then
		echo "FAIL: size difference of $DIFF B exceeds limit $MAX_DIFF B"
		exit 1
	else
		echo "PASS: size difference of $DIFF B within limit $MAX_DIFF B"
	fi
else
	# This means this is the first run of the script.
	# The original file size will be stored, and used
	# to compare against subsequent values ahead.
	echo "No previous size present, storing current size"
	echo "$BIN_SIZE" > "$PREV_SIZE_FILE"
fi

# Remove the binary to ensure it is
# built again with the next commit
rm -f $BIN_NAME
