#!/usr/bin/env bash
#
# Generate two CRIU checkpoints under $1 to test added, removed,
# and unchanged state across checkpoints.

set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "error: must be run as root (piggie needs CLONE_NEWPID, criu needs CAP_SYS_ADMIN)" >&2
    exit 1
fi

CRIU="${CRIU:-criu}"
OUT="$1"

# Count server-side ESTABLISHED connections inside piggie's netns.
count_conns() {
    nsenter -t "$SK_PID" -n ss -tnH state established 'sport = :5000' | wc -l
}

# Poll `count_conns` up to ~5s until it equals $1; fail loudly otherwise.
wait_for_conns() {
    local expected=$1 n
    for _ in $(seq 1 100); do
        n=$(count_conns)
        [ "$n" = "$expected" ] && return
        sleep 0.05
    done
    echo "timeout: expected $expected connections, have $n" >&2
    exit 1
}

# Run CRIU dump; on failure surface its log and abort.
dump() {
    local dir=$1
    shift
    "$CRIU" dump --tcp-established "$@" -v4 -o dump.log -D "$dir" -t "$SK_PID" \
        || { cat "$dir/dump.log"; exit 1; }
}

rm -rf "$OUT"
mkdir -p "$OUT/a" "$OUT/b"

# Paired command / ack FIFOs. Keep both open on fds 3 and 4 so piggie
# never sees EOF between commands.
TMPDIR_FIFO=$(mktemp -d -t piggie.XXXXXX)
CMD_FIFO="$TMPDIR_FIFO/cmd"
ACK_FIFO="$TMPDIR_FIFO/ack"
mkfifo "$CMD_FIFO" "$ACK_FIFO"
trap 'rm -rf "$TMPDIR_FIFO"; [ -n "${SK_PID:-}" ] && kill $SK_PID 2>/dev/null || true' EXIT

# Start piggie pointing at the FIFOs; capture SK_PID from its stdout.
SK_PID=$(PIGGIE_CMD_FIFO="$CMD_FIFO" PIGGIE_ACK_FIFO="$ACK_FIFO" \
	piggie/piggie --tcp-socket)
if ! [ "${SK_PID:-0}" -gt 0 ] 2>/dev/null; then
    echo "error: piggie did not return a valid pid (got '$SK_PID')" >&2
    exit 1
fi
exec 3>"$CMD_FIFO"
exec 4<"$ACK_FIFO"

# Send one command to piggie and read its reply. Returns non-zero on fail.
piggie_cmd() {
    local resp
    printf '%s\n' "$1" >&3
    IFS= read -r -u 4 -t 5 resp || { echo "piggie_cmd '$1': timed out" >&2; return 1; }
    case "$resp" in
        ok) return 0 ;;
        *)  echo "piggie_cmd '$1': $resp" >&2; return 1 ;;
    esac
}

# Wait for the initial tcp-client (started by --tcp-socket, not by a
# command) to establish its connection.
wait_for_conns 1

# Two extras that will only appear in A.
piggie_cmd spawn-tcp-client
piggie_cmd spawn-tcp-client

# Checkpoint A: original client + two extras.
dump "$OUT/a" --leave-running

# Tear down both extras.
piggie_cmd kill-tcp-client
piggie_cmd kill-tcp-client

# A fresh extra that will only appear in B.
piggie_cmd spawn-tcp-client

# Checkpoint B: original client + the new extra.
dump "$OUT/b"
