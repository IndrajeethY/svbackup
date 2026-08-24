#!/usr/bin/env bash
# Build svbackup and install the systemd user timer.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

echo "==> Building binary"
( cd "$REPO" && go build -o svbackup . )

echo "==> Installing systemd user units to $UNIT_DIR"
mkdir -p "$UNIT_DIR"
cp "$REPO/systemd/svbackup.service" "$UNIT_DIR/"
cp "$REPO/systemd/svbackup.timer" "$UNIT_DIR/"

echo "==> Enabling timer"
systemctl --user daemon-reload
systemctl --user enable --now svbackup.timer

echo
echo "Timer installed. Next runs:"
systemctl --user list-timers svbackup.timer --no-pager || true

echo
echo "IMPORTANT: so the timer fires even when you're not logged in, run once:"
echo "    loginctl enable-linger $USER"
echo
echo "Pre-flight your config with:  $REPO/svbackup --check"
echo "Confirm targets with:         $REPO/svbackup --probe"
echo "Manual run:                   $REPO/svbackup --once"
