#!/bin/sh
set -e

# Ensure storage directory exists and has proper non-root permissions
if [ -d "/app/data" ] || [ "$(id -u)" = '0' ]; then
    mkdir -p /app/data
    chown -R appuser:appgroup /app/data 2>/dev/null || true
    chmod 775 /app/data 2>/dev/null || true
fi

# If running as root, drop privileges and execute command as appuser
if [ "$(id -u)" = '0' ]; then
    exec su-exec appuser:appgroup "$@"
fi

exec "$@"
