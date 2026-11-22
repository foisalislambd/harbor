#!/bin/sh
set -eu

data="${HARBOR_DATA_DIR:-/data}"
mkdir -p "$data"

if [ "$(id -u)" = "0" ]; then
  chown -R harbor:harbor "$data"
  exec su-exec harbor /usr/local/bin/harbor serve "$@"
fi

exec /usr/local/bin/harbor serve "$@"
