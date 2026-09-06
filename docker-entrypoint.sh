#!/bin/sh
set -eu
umask 077

PUID="${PUID:-99}"
PGID="${PGID:-100}"

for ID_VALUE in "$PUID" "$PGID"; do
  case "$ID_VALUE" in
    ''|*[!0-9]*)
      echo "PUID and PGID must be numeric" >&2
      exit 1
      ;;
    *[1-9]*) ;;
    *)
      echo "PUID and PGID must be greater than zero" >&2
      exit 1
      ;;
  esac
done

if [ "$(id -u)" = "0" ]; then
  CURRENT_GID="$(getent group augur | cut -d: -f3)"
  if [ "$CURRENT_GID" != "$PGID" ]; then
    groupmod -o -g "$PGID" augur
  fi

  CURRENT_UID="$(id -u augur)"
  if [ "$CURRENT_UID" != "$PUID" ]; then
    usermod -o -u "$PUID" augur
  fi

  CONFIG_PATH="${AUGUR_CONFIG:-/data/config.json}"
  STORAGE_PATH="${AUGUR_STORAGE_PATH:-/data/augur-state.db}"
  CONFIG_DIR="$(dirname "$CONFIG_PATH")"
  STORAGE_DIR="$(dirname "$STORAGE_PATH")"

  mkdir -p "$CONFIG_DIR" "$STORAGE_DIR"
  if [ ! -e "$CONFIG_PATH" ]; then
    cp /app/config.example.json "$CONFIG_PATH"
  fi
  chown augur:augur "$CONFIG_DIR" "$STORAGE_DIR" "$CONFIG_PATH"
  chmod 0600 "$CONFIG_PATH"
  if [ -e "$STORAGE_PATH" ]; then
    chown augur:augur "$STORAGE_PATH"
    chmod 0600 "$STORAGE_PATH"
  fi
  if [ -e "$STORAGE_PATH-wal" ]; then
    chown augur:augur "$STORAGE_PATH-wal"
    chmod 0600 "$STORAGE_PATH-wal"
  fi
  if [ -e "$STORAGE_PATH-shm" ]; then
    chown augur:augur "$STORAGE_PATH-shm"
    chmod 0600 "$STORAGE_PATH-shm"
  fi

  if [ "${1:-}" != "" ] && [ "${1#-}" != "$1" ]; then
    set -- augur "$@"
  fi

  exec su-exec augur "$@"
fi

if [ "${1:-}" != "" ] && [ "${1#-}" != "$1" ]; then
  set -- augur "$@"
fi

exec "$@"
