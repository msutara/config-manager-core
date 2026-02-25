#!/bin/sh
set -e

# Only stop and disable on actual removal, not during upgrades.
case "$1" in
    remove|purge)
        if [ -d /run/systemd/system ]; then
            systemctl stop cm || true
            systemctl disable cm || true
        fi
        ;;
esac
