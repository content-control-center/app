#!/bin/sh
set -e

# Railway (and Docker) mount the persistent KEK volume owned by root, which
# shadows the build-time chown — so the unprivileged appuser can't create
# kek.v1 ("permission denied"). We start as root, fix ownership of the KEK
# directory at runtime, then drop to appuser to run the server.
KEK_DIR="${OGEN_KEK_PATH:-/var/lib/ogen/keys}"
mkdir -p "$KEK_DIR"
chown -R appuser:appgroup "$KEK_DIR"
chmod 700 "$KEK_DIR"

exec su-exec appuser:appgroup /server "$@"
