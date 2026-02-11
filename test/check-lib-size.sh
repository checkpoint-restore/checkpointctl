#!/bin/bash
#
# A shell script to check if the library binary size
# has increased beyond a certain threshold across commits.
# Meant to be used in CI with:
# git rebase <base branch>^ -x test/check-lib-size.sh

BIN_NAME=lib-size-test
BIN_DIR=test/lib-size
PREV_SIZE_FILE=/tmp/prev_lib_size
# Maximum allowable size difference, in bytes
MAX_DIFF=51200

# Copy test files from /tmp if they exist there (CI mode).
# This handles rebasing across commits where the test files
# don't exist yet.
COPIED_FROM_TMP=false
if [[ -d /tmp/lib-size ]] && [[ ! -f "$BIN_DIR/main.go" ]]; then
	mkdir -p "$BIN_DIR"
	/usr/bin/cp -f /tmp/lib-size/main.go "$BIN_DIR/main.go"
	COPIED_FROM_TMP=true
fi

# Build the minimal test binary that imports the lib package.
# If the commit is not self-contained, the build will fail,
# in which case there is no point checking for a change in
# the size of the binary.
if ! go build -o "$BIN_DIR/$BIN_NAME" "./$BIN_DIR"; then
	echo "ERROR: Compilation failed at $(git rev-parse --short HEAD)"
	echo "Make sure that the compilation is successful for each commit."
	exit 1
fi

# Fail fast on errors
set -e

# Store the binary size
BIN_SIZE=$(stat -c%s "$BIN_DIR/$BIN_NAME")
# Print the size along with the commit hash
echo "LIBRARY SIZE ($(git rev-parse --short HEAD)): $BIN_SIZE"

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
rm -f "$BIN_DIR/$BIN_NAME"

# Clean up copied test files to avoid conflicts when
# git checks out the commit that actually adds them
if [[ "$COPIED_FROM_TMP" == "true" ]]; then
	rm -rf "$BIN_DIR"
fi
