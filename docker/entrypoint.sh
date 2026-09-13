#!/bin/sh
# unRAID hands containers a PUID/PGID to own their appdata. When we start as
# root we take ownership of the data directory and drop to that user; when we
# are already unprivileged (plain `docker run --user`), we just exec.
set -e

DB_PATH="${LINGLE_DB:-/data/lingle.db}"
DB_DIR=$(dirname "$DB_PATH")
mkdir -p "$DB_DIR"

if [ "$(id -u)" = "0" ]; then
    PUID="${PUID:-99}"
    PGID="${PGID:-100}"

    if ! getent group lingle >/dev/null 2>&1; then
        addgroup -g "$PGID" lingle 2>/dev/null || true
    fi
    if ! getent passwd lingle >/dev/null 2>&1; then
        adduser -D -H -u "$PUID" -G lingle lingle 2>/dev/null || true
    fi

    chown -R "$PUID:$PGID" "$DB_DIR" 2>/dev/null || true
    echo "lingle: dropping to uid=$PUID gid=$PGID, data in $DB_DIR"
    exec su-exec "$PUID:$PGID" /usr/local/bin/lingle "$@"
fi

exec /usr/local/bin/lingle "$@"
