#!/bin/sh
set -e

# Only stop and disable on actual removal, not during upgrades.
case "$1" in
    remove|purge)
        if [ -d /run/systemd/system ]; then
            if systemctl is-active --quiet cm 2>/dev/null; then
                systemctl stop cm
            fi
            if systemctl is-enabled --quiet cm 2>/dev/null; then
                systemctl disable cm
            fi
        fi
        ;;
esac
