#!/usr/bin/env bash
#
# test-servers.sh — the FTP servers the test suite runs against.
#
#   ./scripts/test-servers.sh test    run the suite against them
#   ./scripts/test-servers.sh up      start them and leave them running
#   ./scripts/test-servers.sh down    stop them
#   ./scripts/test-servers.sh logs    show what they are saying
#
# `test` runs the suite in a container on the servers' own network, and
# that is not a convenience. FTP negotiates transfers by address: a
# passive transfer has the server tell the client where to connect, and
# an active one has the server connect back to the client. A server
# behind published ports names an address the host cannot route to, so
# passive transfers time out and active ones are refused. Sharing a
# network makes the address the server names the address the client can
# reach.
#
# `up` publishes the ports anyway, so `go test` from the host works for
# everything that is not a data transfer.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose="$repo/test-servers/compose.yaml"

command -v docker >/dev/null || {
    echo "docker is not installed. It is what these servers run in —" >&2
    echo "see https://docs.docker.com/get-docker/" >&2
    exit 1
}

# The Go the tests run under.
#
# Deliberately *not* taken from go.mod. That directive is this library's
# compatibility floor — the oldest toolchain it promises to build with —
# and testing only against the floor would never exercise the toolchain
# anyone actually uses. CI runs both; this is the one a developer gets by
# default.
#
# Pinned rather than "latest", so a Go release cannot change what this
# script does without somebody editing this line.
GO_DEFAULT=1.25

go_version() {
    echo "${GO_VERSION:-$GO_DEFAULT}"
}

case "${1:-test}" in
    up)
        GO_VERSION="$(go_version)" docker compose -f "$compose" up -d --build
        echo
        echo "Servers are up. From the host:"
        echo "  pure-ftpd            127.0.0.1:2121   (explicit TLS)"
        echo "  pure-ftpd-implicit   127.0.0.1:2122   (implicit TLS)"
        echo "  proftpd              127.0.0.1:2124"
        echo
        echo "Data transfers need the suite on the same network:"
        echo "  ./scripts/test-servers.sh test"
        ;;
    down)
        docker compose -f "$compose" --profile test down -v
        ;;
    logs)
        docker compose -f "$compose" logs "${@:2}"
        ;;
    test)
        GO_VERSION="$(go_version)" docker compose -f "$compose" up -d --build
        # `run` rather than `up`, so the suite's exit status is this
        # script's exit status.
        GO_VERSION="$(go_version)" docker compose -f "$compose" --profile test \
            run --rm tests go test "${@:2}" ./...
        ;;
    *)
        echo "usage: $0 [test|up|down|logs]" >&2
        exit 2
        ;;
esac
