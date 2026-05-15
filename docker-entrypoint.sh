#!/bin/sh
set -e

# Ensure persistent data directory exists
mkdir -p /app/data

# Symlink SQLite database to persistent volume mount (/app/data/)
# The binary writes to ./lingma2api.db (CWD = /app), so we redirect it
# to the persistent location via a symlink.
if [ ! -L /app/lingma2api.db ]; then
  rm -f /app/lingma2api.db
  touch /app/data/lingma2api.db
  ln -s /app/data/lingma2api.db /app/lingma2api.db
fi

exec "$@"
