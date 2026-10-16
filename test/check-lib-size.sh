#!/bin/bash

# Compare the same library consumer at the base and merged PR commits.
exec bash "$(dirname -- "$0")/compare-size.sh" library "$@"
