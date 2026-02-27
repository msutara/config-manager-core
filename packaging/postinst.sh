#!/bin/sh
set -e

# Create required directories.
mkdir -p /var/log/cm /var/lib/cm /etc/cm
chmod 750 /var/log/cm /var/lib/cm

case "$1" in
    configure)
        # Only run systemctl when systemd is the init system.
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload || true
            if [ -z "$2" ]; then
                # Fresh install: enable and start.
                systemctl enable cm || true
                if systemctl start cm; then
                    echo "Config Manager installed and started."
                else
                    echo "Config Manager installed but failed to start. Check: journalctl -u cm"
                fi
            else
                # Upgrade: restart to load new binary.
                systemctl try-restart cm || true
            fi
        fi
        ;;
esac
