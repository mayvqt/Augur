#!/bin/sh
set -eu

PUID="${PUID:-99}"
PGID="${PGID:-100}"

for ID_VALUE in "$PUID" "$PGID"; do
  case "$ID_VALUE" in
    ''|*[!0-9]*)
      echo "PUID and PGID must be numeric" >&2
      exit 1
      ;;
  esac
done

if [ "$PUID" = "0" ] || [ "$PGID" = "0" ]; then
  echo "PUID and PGID must be greater than zero" >&2
  exit 1
fi

if [ "$(id -u)" = "0" ]; then
  CURRENT_GID="$(getent group augur | cut -d: -f3)"
  if [ "$CURRENT_GID" != "$PGID" ]; then
    groupmod -o -g "$PGID" augur
  fi

  CURRENT_UID="$(id -u augur)"
  if [ "$CURRENT_UID" != "$PUID" ]; then
    usermod -o -u "$PUID" augur
  fi

  mkdir -p /data
  if [ ! -e /data/config.json ]; then
    cp /app/config.docker.json /data/config.json
  fi
  chown -R augur:augur /data

  if [ "${1:-}" != "" ] && [ "${1#-}" != "$1" ]; then
    set -- augur "$@"
  fi

  exec su-exec augur "$@"
fi

if [ "${1:-}" != "" ] && [ "${1#-}" != "$1" ]; then
  set -- augur "$@"
fi

exec "$@"
