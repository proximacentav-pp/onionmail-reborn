#!/usr/bin/env bash
# Publish the OnionMail hidden service from the HOST tor, so the container
# does not need to run its own tor.
#
# Run once on the host:  sudo ./host_tor_setup.sh
# Re-running is safe.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

NICK="${ONIONMAIL_NICK:-onionmail}"
CONT="${ONIONMAIL_CONTAINER:-onionmail}"
HS_DIR=/var/tor/onionmail
TORRC=/etc/tor/torrc
MARK="### OnionMail hidden service (managed)"
HS_PORTS=("25 127.0.0.1:20025" "110 127.0.0.1:20110" "80 127.0.0.1:18000")

if [ ! -s "$HS_DIR/hs_ed25519_secret_key" ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  if docker inspect "$CONT" >/dev/null 2>&1; then
    docker cp "$CONT:/var/lib/onionmail/HiddenServiceDir-1-${NICK}/." "$tmp/" 2>/dev/null || true
  fi
  if [ ! -s "$tmp/hs_ed25519_secret_key" ]; then
    vol="$(docker volume inspect "$(docker inspect "$CONT" --format '{{range .Mounts}}{{if eq .Destination "/var/lib/onionmail"}}{{.Name}}{{end}}{{end}}' 2>/dev/null)" -f '{{.Mountpoint}}' 2>/dev/null || true)"
    if [ -n "$vol" ] && [ -d "$vol/HiddenServiceDir-1-${NICK}" ]; then
      cp -a "$vol/HiddenServiceDir-1-${NICK}/." "$tmp/"
    fi
  fi
  if [ -s "$tmp/hs_ed25519_secret_key" ]; then
    mkdir -p "$HS_DIR"
    cp -a "$tmp/." "$HS_DIR/"
  else
    echo "no onion identity found; generating a new one"
    gen="$(mktemp -d)"
    cat > "$gen/torrc" <<GEN
SocksPort 0
DataDirectory $gen/data
HiddenServiceDir $gen/hs
HiddenServiceVersion 3
HiddenServicePort 80 127.0.0.1:1
GEN
    timeout 10 tor -f "$gen/torrc" >/dev/null 2>&1 || true
    if [ -s "$gen/hs/hs_ed25519_secret_key" ]; then
      mkdir -p "$HS_DIR"
      cp -a "$gen/hs/." "$HS_DIR/"
    else
      echo "ERROR: could not generate an onion identity." >&2
      exit 1
    fi
    rm -rf "$gen"
  fi
fi

detect_tor_user() {
  if [ -n "${ONIONMAIL_TOR_USER:-}" ]; then
    printf '%s\n' "$ONIONMAIL_TOR_USER"
    return
  fi
  local u
  u="$(systemctl show tor -p User --value 2>/dev/null || true)"
  if [ -n "$u" ] && id "$u" >/dev/null 2>&1; then
    printf '%s\n' "$u"
    return
  fi
  u="$(ps -o user= -C tor 2>/dev/null | head -n1 | tr -d ' ' || true)"
  if [ -n "$u" ] && id "$u" >/dev/null 2>&1; then
    printf '%s\n' "$u"
    return
  fi
  for u in debian-tor _tor tor; do
    if id "$u" >/dev/null 2>&1; then
      printf '%s\n' "$u"
      return
    fi
  done
  printf 'root\n'
}

TOR_USER="$(detect_tor_user)"
chown -R "$TOR_USER:$TOR_USER" "$HS_DIR"
chmod 700 "$HS_DIR"
chmod 600 "$HS_DIR/hs_ed25519_secret_key" "$HS_DIR/hs_ed25519_public_key"
[ -f "$HS_DIR/hostname" ] && chmod 600 "$HS_DIR/hostname"

if ! grep -qF "$MARK" "$TORRC"; then
  cp -a "$TORRC" "${TORRC}.bak.onionmail.$(date +%s)"
  {
    echo
    echo "$MARK"
    echo "HiddenServiceDir ${HS_DIR}"
    echo "HiddenServiceVersion 3"
    for p in "${HS_PORTS[@]}"; do echo "HiddenServicePort ${p}"; done
  } >> "$TORRC"
  echo "added hidden service to ${TORRC}"
else
  echo "hidden service already present in ${TORRC}"
fi

systemctl reload tor 2>/dev/null || systemctl restart tor

for _ in $(seq 1 60); do
  [ -s "$HS_DIR/hostname" ] && break
  sleep 1
done

if [ ! -s "$HS_DIR/hostname" ]; then
  echo "ERROR: tor did not publish the hidden service. Recent log:" >&2
  journalctl -u tor -n 40 --no-pager >&2 || true
  exit 1
fi

echo "onion address: $(cat "$HS_DIR/hostname")"
echo "put this in .env before the first container start:"
echo "  ONION_ADDRESS=$(cat "$HS_DIR/hostname")"
