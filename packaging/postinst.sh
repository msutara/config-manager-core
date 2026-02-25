#!/bin/sh
set -e

# Create cm system user and group if they don't exist.
if ! getent group cm >/dev/null 2>&1; then
    groupadd --system cm
fi
if ! getent passwd cm >/dev/null 2>&1; then
    useradd --system --gid cm --home-dir /var/lib/cm --shell /usr/sbin/nologin cm
fi

# Create required directories.
mkdir -p /var/log/cm /var/lib/cm /etc/cm
chown cm:cm /var/log/cm /var/lib/cm
chmod 750 /var/log/cm /var/lib/cm

case "$1" in
    configure)
        # Only run systemctl when systemd is the init system.
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload
            if [ -z "$2" ]; then
                # Fresh install: enable and start.
                systemctl enable cm
                systemctl start cm
                echo "Config Manager installed and started."
            else
                # Upgrade: restart to load new binary.
                systemctl try-restart cm
            fi
        fi
        ;;
esac
