#!/bin/bash

# Compare the default checkpointctl binary at the base and merged PR commits.
exec bash "$(dirname -- "$0")/compare-size.sh" binary "$@"
