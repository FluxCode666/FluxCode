#!/bin/sh
set -e

# Prepare writable mount roots when running as root. Generated media lives
# outside /app/data, so the existing application-data ownership repair does
# not recursively walk a potentially large artifact tree.
if [ "$(id -u)" = "0" ]; then
    ensure_writable_directory() {
        directory="$1"
        description="$2"

        mkdir -p "$directory"
        if ! chown sub2api:sub2api "$directory"; then
            echo "WARNING: could not change ownership of ${description} (${directory}); checking existing UID/GID 1000 permissions" >&2
        fi

        if ! su-exec sub2api sh -c '
            probe=$(mktemp "$1/.fluxcode-write-probe.XXXXXX") || exit 1
            rm -f "$probe"
        ' sh "$directory"; then
            echo "ERROR: ${description} (${directory}) must be writable by UID/GID 1000" >&2
            exit 1
        fi
    }

    mkdir -p /app/data
    # Preserve the existing image behavior for config, backup and certificate
    # subdirectories. Read-only bind mounts may reject chown and are tolerated.
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    ensure_writable_directory /app/data "application data directory"
    ensure_writable_directory /app/.fluxcode "FluxCode state directory"
    ensure_writable_directory /app/.fluxcode/generated "generated media directory"

    # A directly mounted, writable config file may be root-owned. Read-only
    # config mounts are still supported and retain their original ownership.
    if [ -e /app/data/config.yaml ] && [ -w /app/data/config.yaml ]; then
        if ! chown sub2api:sub2api /app/data/config.yaml; then
            echo "WARNING: writable /app/data/config.yaml could not be assigned to UID/GID 1000" >&2
        fi
    fi

    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

exec "$@"
