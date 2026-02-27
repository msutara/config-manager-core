#!/bin/sh
set -e

# Create required directories.
mkdir -p /var/log/cm /var/lib/cm /etc/cm
chmod 750 /var/log/cm /var/lib/cm

case "$1" in
    configure)
        # Generate auth token on fresh install (never overwrite existing).
        if [ ! -f /etc/cm/auth.token ]; then
            if command -v openssl >/dev/null 2>&1; then
                (umask 077 && openssl rand -hex 32 > /etc/cm/auth.token)
            else
                (umask 077 && head -c 32 /dev/urandom | od -A n -t x1 | tr -d ' \n' > /etc/cm/auth.token)
            fi
            echo "Auth token generated: /etc/cm/auth.token"
        fi

        # Only run systemctl when systemd is the init system.
        if [ -d /run/systemd/system ]; then
            systemctl daemon-reload || true
            if [ -z "$2" ]; then
                # Fresh install: enable and start.
                systemctl enable cm || true
                if systemctl start cm; then
                    echo "Config Manager installed and started."
                    echo "Web UI: set listen_host to 0.0.0.0 in /etc/cm/config.yaml for remote access"
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
