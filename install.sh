#!/usr/bin/env sh
# Installs iseall as a boot-time systemd service.
#   sudo ./install.sh                 # picks the right binary from dist/ (or ./iseall)
#   sudo ./install.sh --kiosk         # also opens the dashboard full-screen on the Pi's own display
#   sudo ./install.sh --port 9000     # listen on another port
#   sudo ./install.sh --uninstall
set -eu

PORT=8765
KIOSK=0
UNINSTALL=0
while [ $# -gt 0 ]; do
  case "$1" in
    --kiosk) KIOSK=1 ;;
    --port) PORT="$2"; shift ;;
    --uninstall) UNINSTALL=1 ;;
    *) echo "unknown option: $1"; exit 1 ;;
  esac
  shift
done

[ "$(id -u)" -eq 0 ] || { echo "Run with sudo."; exit 1; }
cd "$(dirname "$0")"

if [ "$UNINSTALL" -eq 1 ]; then
  systemctl disable --now iseall.service 2>/dev/null || true
  rm -f /etc/systemd/system/iseall.service /usr/local/bin/iseall
  systemctl daemon-reload
  echo "iseall removed. Config kept in /etc/iseall; startup apps (iseall-*.service) were left untouched."
  exit 0
fi

case "$(uname -m)" in
  aarch64|arm64) ARCH=linux-arm64 ;;
  armv7l) ARCH=linux-armv7 ;;
  armv6l) ARCH=linux-armv6 ;;
  x86_64) ARCH=linux-amd64 ;;
  *) echo "unsupported architecture $(uname -m)"; exit 1 ;;
esac
BIN="dist/iseall-$ARCH"
[ -f "$BIN" ] || BIN=./iseall
[ -f "$BIN" ] || { echo "No binary found. Run ./build.sh first (or copy dist/iseall-$ARCH here)."; exit 1; }

# copy-then-rename so updating over a running binary never hits "text file busy"
install -m 0755 "$BIN" /usr/local/bin/iseall.new && mv -f /usr/local/bin/iseall.new /usr/local/bin/iseall
mkdir -p /etc/iseall && chmod 700 /etc/iseall

cat > /etc/systemd/system/iseall.service <<EOF
[Unit]
Description=iseall server dashboard
After=network-online.target docker.service
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/iseall -addr :$PORT -config /etc/iseall/config.json
Restart=always
RestartSec=3
# Root is needed to see every process/socket and to manage services & containers.
User=root
Nice=5
MemoryMax=96M
Environment=GOMEMLIMIT=40MiB

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable iseall.service
systemctl restart iseall.service   # restart (not just start) so a re-run picks up a new binary

if [ "$KIOSK" -eq 1 ]; then
  DESK_USER="${SUDO_USER:-$(getent passwd 1000 | cut -d: -f1)}"
  HOME_DIR="$(getent passwd "$DESK_USER" | cut -d: -f6)"
  BROWSER="$(command -v chromium-browser || command -v chromium || true)"
  if [ -z "$BROWSER" ]; then
    echo "Kiosk: no chromium found (sudo apt install chromium-browser), skipping."
  else
    CMD="$BROWSER --kiosk --noerrdialogs --disable-infobars --check-for-update-interval=31536000 --app=http://localhost:$PORT/"
    # XDG autostart is honoured by Raspberry Pi OS under labwc, wayfire and LXDE alike
    mkdir -p "$HOME_DIR/.config/autostart"
    cat > "$HOME_DIR/.config/autostart/iseall-kiosk.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=iseall kiosk
Exec=sh -c 'sleep 5; $CMD'
EOF
    chown -R "$DESK_USER": "$HOME_DIR/.config/autostart"
    echo "Kiosk: dashboard opens full-screen when $DESK_USER's desktop starts. (Tip: turn on Lite mode in Settings on the Pi's screen.)"
  fi
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
echo "  ✓ iseall is running and starts on every boot."
echo "  → open http://${IP:-<pi-ip>}:$PORT and create your password."
echo "  (reset it any time:  sudo iseall -passwd -config /etc/iseall/config.json)"
