#!/bin/bash
set -euo pipefail

NICK="${ONIONMAIL_NICK:-onionmail}"
DISPLAY_NAME="${ONIONMAIL_DISPLAY_NAME:-example}"
SMTP_PORT=20025
POP3_PORT=20110
WEB_PORT=18000
WEB_BIND="${WEB_BIND:-127.0.0.1}"
HTTP_BACKEND_PORT=18001
CTRL_PORT=9010
CTRL_IP=127.0.0.2
SSH_CTRL_ENABLED=0
SSH_CTRL_PORT=9011
SSH_CTRL_USER=om
USE_HOST_TOR="${USE_HOST_TOR:-0}"
TOR_IP="${TOR_IP:-}"
TOR_PORT="${TOR_PORT:-9050}"
ONION_ADDRESS="${ONION_ADDRESS:-}"
CAPTCHA_DISABLE="${CAPTCHA_DISABLE:-no}"
DEFAULT_THEME="${DEFAULT_THEME:-light}"
WWWROOT="/var/lib/onionmail/wwwroot"
TOR_LOG="/var/log/onionmail/tor.log"

log() { echo "[onionmail] $*"; }

tor_error() {
  printf 'STDERR: Tor connection error: %s\n' "$*" >&2
}

start_local_socks() {
  mkdir -p /var/lib/tor /run/tor
  chown -R debian-tor:debian-tor /var/lib/tor /run/tor 2>/dev/null || true
  chmod 700 /var/lib/tor
  cat > /etc/tor/torrc <<EOF
User debian-tor
SocksPort 127.0.0.1:${TOR_PORT}
DataDirectory /var/lib/tor
Log notice file ${TOR_LOG}
EOF
  chgrp debian-tor /var/log/onionmail 2>/dev/null || true
  chmod 750 /var/log/onionmail 2>/dev/null || true
  touch "$TOR_LOG"
  chown debian-tor:debian-tor "$TOR_LOG" 2>/dev/null || true
  chmod 640 "$TOR_LOG"
  service tor start >/dev/null 2>&1 || su -s /bin/sh debian-tor -c "tor -f /etc/tor/torrc" >/dev/null 2>&1 &
  local_tor_up=0
  for _ in $(seq 1 30); do
    if timeout 1 bash -c "exec 3<>/dev/tcp/${TOR_IP}/${TOR_PORT}" 2>/dev/null; then
      local_tor_up=1
      break
    fi
    sleep 1
  done
  [ "$local_tor_up" = "1" ]
}

escape_sed() {
  local s="$1"
  s=${s//\\/\\\\}
  s=${s//&/\\&}
  s=${s//|/\\|}
  s=${s//#/\\#}
  printf '%s' "$s"
}

restore_seed() {
  local src="$1" dst="$2"
  mkdir -p "$dst"
  if [ -z "$(ls -A "$dst" 2>/dev/null)" ]; then
    log "initialising $dst"
    cp -a "$src/." "$dst/"
  fi
}

restore_seed /opt/seed/etc-onionmail /etc/onionmail
restore_seed /opt/seed/var-lib-onionmail /var/lib/onionmail
restore_seed /opt/seed/var-onionmail /var/onionmail

mkdir -p /var/log/onionmail

if [ -f /etc/onionmail/om.json ] && [ -x /usr/local/bin/om ]; then
  eval "$(/usr/local/bin/om __boot-env 2>/dev/null || true)"
fi
if [ -z "${CONTROL_PORT:-}" ]; then
  CONTROL_PORT="$(awk 'tolower($1)=="controlport"{print $2; exit}' /etc/onionmail/control.conf 2>/dev/null || true)"
fi
CTRL_PORT="${CONTROL_PORT:-${CTRL_PORT}}"
CTRL_IP="${CONTROL_UPSTREAM_IP:-${CTRL_IP}}"
SSH_CTRL_ENABLED="${CONTROL_SSH_ENABLED:-${SSH_CTRL_ENABLED}}"
SSH_CTRL_PORT="${CONTROL_SSH_PORT:-${SSH_CTRL_PORT}}"
SSH_CTRL_USER="${CONTROL_SSH_USER:-${SSH_CTRL_USER}}"
SKIP_TOR="${SKIP_TOR:-0}"
if [ -z "${TOR_MODE:-}" ]; then
  if [ "$USE_HOST_TOR" = "1" ]; then TOR_MODE=external; else TOR_MODE=internal; fi
fi
export LANG="${LOCALE:-${LANG:-C.UTF-8}}"

HS_DIR="/var/lib/onionmail/HiddenServiceDir-1-${NICK}"
MAILDIR="/var/onionmail/${NICK}"
SERVERS_CONF="/etc/onionmail/servers.d/1-${NICK}.conf"

if [ "$TOR_MODE" = "external" ]; then
  log "external Tor mode: the container's tor is disabled"
  if [ -z "$TOR_IP" ]; then
    TOR_IP="$(ip route 2>/dev/null | awk '/^default/{print $3; exit}')"
  fi
  ONION="${ONION:-$ONION_ADDRESS}"
  if [ -z "$ONION" ] && [ -f "$SERVERS_CONF" ]; then
    ONION="$(awk '$1=="Onion"{print $2; exit}' "$SERVERS_CONF" || true)"
  fi
  if [ -z "$ONION" ] && [ -s "${HS_DIR}/hostname" ]; then
    ONION="$(cat "${HS_DIR}/hostname")"
  fi
  if [ -z "$ONION" ]; then
    log "WARNING: ONION_ADDRESS is empty; set it in .env to the .onion address published by the host tor"
  fi
  if ! timeout 3 bash -c "exec 3<>/dev/tcp/${TOR_IP}/${TOR_PORT}" 2>/dev/null; then
    tor_error "host tor SOCKS is not reachable on ${TOR_IP}:${TOR_PORT}"
    log "waiting for host tor SOCKS ${TOR_IP}:${TOR_PORT} (up to 90s)"
    for _ in $(seq 1 45); do
      timeout 3 bash -c "exec 3<>/dev/tcp/${TOR_IP}/${TOR_PORT}" 2>/dev/null && break
      sleep 2
    done
    if ! timeout 3 bash -c "exec 3<>/dev/tcp/${TOR_IP}/${TOR_PORT}" 2>/dev/null; then
      TOR_LOCAL_FALLBACK=1
    fi
  fi
  if [ "${TOR_LOCAL_FALLBACK:-0}" != "1" ]; then
    log "using host tor SOCKS at ${TOR_IP}:${TOR_PORT}"
  fi
elif [ "$TOR_MODE" = "internal" ]; then
  mkdir -p /var/lib/tor /run/tor
  chown -R debian-tor:debian-tor /var/lib/tor /run/tor 2>/dev/null || true
  chmod 700 /var/lib/tor

  log "configuring internal tor (hidden service)"
  if [ -f /etc/onionmail/om.json ] && [ -x /usr/local/bin/om ]; then
    /usr/local/bin/om __torrc
  else
    TOR_PORT=19050
    cat > /etc/tor/torrc <<EOF
User debian-tor
SocksPort 127.0.0.1:${TOR_PORT}
DataDirectory /var/lib/tor
Log notice file ${TOR_LOG}
HiddenServiceDir ${HS_DIR}
HiddenServiceVersion 3
HiddenServicePort 25 127.0.0.1:${SMTP_PORT}
HiddenServicePort 110 127.0.0.1:${POP3_PORT}
HiddenServicePort 80 127.0.0.1:${WEB_PORT}
EOF
  fi

  mkdir -p "$HS_DIR"
  chown -R debian-tor:debian-tor "$HS_DIR"
  chmod 700 "$HS_DIR"

  chgrp debian-tor /var/log/onionmail
  chmod 750 /var/log/onionmail
  touch "$TOR_LOG"
  chown debian-tor:debian-tor "$TOR_LOG"
  chmod 640 "$TOR_LOG"

  log "starting tor"
  service tor start >/dev/null 2>&1 || su -s /bin/sh debian-tor -c "tor -f /etc/tor/torrc" >/dev/null 2>&1 &

  log "waiting for the .onion address (first start can take a few minutes)"
  for _ in $(seq 1 180); do
    [ -s "${HS_DIR}/hostname" ] && break
    sleep 2
  done
  if [ ! -s "${HS_DIR}/hostname" ]; then
    tor_error "tor did not publish the hidden service in time"
    SKIP_TOR=1
    ONION="${ONION:-${ONION_ADDRESS:-}}"
  else
    ONION="$(cat "${HS_DIR}/hostname")"
  fi
  TOR_IP="127.0.0.1"

  log "waiting for tor SOCKS ${TOR_IP}:${TOR_PORT}"
  tor_up=0
  for _ in $(seq 1 90); do
    if timeout 2 bash -c "exec 3<>/dev/tcp/${TOR_IP}/${TOR_PORT}" 2>/dev/null; then
      tor_up=1
      break
    fi
    sleep 1
  done
  if [ "$tor_up" != "1" ]; then
    tor_error "tor SOCKS is not reachable on ${TOR_IP}:${TOR_PORT}"
    TOR_LOCAL_FALLBACK=1
  fi
else
  log "tor mode: disabled (skip-tor: localhost only)"
  SKIP_TOR=1
  ONION="${ONION:-${ONION_ADDRESS:-}}"
  TOR_MODE=local
  TOR_IP=127.0.0.1
  TOR_PORT=19050

  log "starting local tor SOCKS on ${TOR_IP}:${TOR_PORT} (no onion)"
  if ! start_local_socks; then
    tor_error "local tor SOCKS ${TOR_IP}:${TOR_PORT} is not reachable"
  fi
fi

if [ "${TOR_LOCAL_FALLBACK:-0}" = "1" ]; then
  TOR_MODE=local
  TOR_IP=127.0.0.1
  TOR_PORT=19050
  SKIP_TOR=1
  ONION="${ONION:-${ONION_ADDRESS:-}}"
  service tor stop >/dev/null 2>&1 || true
  log "falling back to local SOCKS-only Tor on ${TOR_IP}:${TOR_PORT} (no onion)"
  if ! start_local_socks; then
    tor_error "local tor SOCKS ${TOR_IP}:${TOR_PORT} is not reachable"
  fi
fi

sed -i "s/^TorIP.*/TorIP ${TOR_IP:-127.0.0.1}/; s/^TorPort.*/TorPort ${TOR_PORT:-1}/" /etc/onionmail/core.conf
# The upstream control service must listen on a private loopback address;
# the front owns 127.0.0.1:<port> and can be disabled there.
sed -i "s|^ControlIP.*|ControlIP\t127.0.0.2|" /etc/onionmail/control.conf
if [ "$SKIP_TOR" = "1" ]; then
  log "onion address: disabled (Tor skipped)"
else
  log "onion address: ${ONION}"
fi

if [ ! -f "${MAILDIR}/server.bin" ]; then
  log "first start: generating the OnionMail server (this can take a while)"

  rm -rf "$MAILDIR"
  mkdir -p "$MAILDIR"
  chown debian-onionmail:debian-onionmail "$MAILDIR"

  # Fresh master passphrase and root password for this deployment.
  head -c 200 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-84 \
    > /etc/onionmail/bootToken
  chown debian-onionmail:debian-onionmail /etc/onionmail/bootToken
  chmod 600 /etc/onionmail/bootToken
  BOOT="$(cat /etc/onionmail/bootToken)"
  ROOTPASS="$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-20)"
  sed -i "s|^RootPass.*|RootPass\t${ROOTPASS}|" /etc/onionmail/control.conf

  # Password used for the local control port / server administration.
  if [ -f /etc/onionmail/.service_pass ]; then
    SVCPASS="$(cat /etc/onionmail/.service_pass)"
  else
    SVCPASS="$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-24)"
    printf '%s' "$SVCPASS" > /etc/onionmail/.service_pass
    chmod 600 /etc/onionmail/.service_pass
  fi

  SYSOP_SMTP_PASS="${SYSOP_SMTP_PASSWORD:-$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-24)}"
  SYSOP_POP3_PASS="${SYSOP_POP3_PASSWORD:-$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-24)}"

  mkdir -p /etc/onionmail/servers.d
  sed -e "s#__NICK__#${NICK}#g" \
      -e "s#__ONION__#${ONION}#g" \
      -e "s#__PAR_PASSWD_SERV__#${SVCPASS}#g" \
      -e "s#__SMTP__#${SMTP_PORT}#g" \
      -e "s#__POP3__#${POP3_PORT}#g" \
      -e "s#__SSL_CC__#XX#g" \
      -e "s#__SSL_ORG__#$(escape_sed "$DISPLAY_NAME") OnionMail server#g" \
      -e "s#__SSL_UNI__#Mail#g" \
      -e "s#__SSL_STA__#OnionLand#g" \
      -e "s#__HTTP__#${HTTP_BACKEND_PORT}#g" \
      -e "s#__MSIZE__#256#g" \
      -e "s#__MNUMSG__#256#g" \
      -e "s#__POP3REG__#no#g" \
      -e "s#__NUSRD__#0#g" -e "s#__NUSRH__#0#g" \
      -e "s#__NLISTD__#0#g" -e "s#__NLISTH__#0#g" \
      -e "s#__FRIENDLY__#no#g" \
      -e "s#__ISHTTP__#yes#g" \
      -e "s#__HTTPMANF__#voucher#g" \
      -e "s#__EXITDOM__#${ONION}#g" \
      -e "s#__MAILDIR__#${MAILDIR}#g" \
      /etc/onionmail/setupkit/servers.conf > "$SERVERS_CONF"

  printf '%s\n%s\n' "$SYSOP_SMTP_PASS" "$SYSOP_POP3_PASS" | \
    su debian-onionmail -c "/bin/onionmail -f /etc/onionmail/config.conf -ndk -sp -p '${BOOT}' --gen-servers"

  if [ ! -f "${MAILDIR}/server.bin" ]; then
    log "FATAL: server generation failed; see /var/log/onionmail/server-gen.log"
    exit 1
  fi
  chmod 600 "${MAILDIR}/sysop.txt" "${MAILDIR}/keyblock.txt" 2>/dev/null || true
  log "server generated"
  log "sysop credentials: ${MAILDIR}/sysop.txt (keep them offline)"
  log "recovery keyblock:  ${MAILDIR}/keyblock.txt (keep it offline)"
fi

# ---- operator policy: built-in HTTP, voucher registration, limits ----------
if [ -f /etc/onionmail/om.json ] && [ -x /usr/local/bin/om ]; then
  /usr/local/bin/om __apply >/dev/null 2>&1 || true
elif [ -f "$SERVERS_CONF" ]; then
  sed -i \
    -e "s/^\([[:space:]]*MaxMsgSize\)[[:space:]].*/\1\t256M/" \
    -e "s/^\([[:space:]]*MaxMsgXuser\)[[:space:]].*/\1\t256/" \
    -e "s/^\([[:space:]]*MaxSpamEntryXUser\)[[:space:]].*/\1\t196/" \
    -e "s/^\([[:space:]]*MaxMailingListSize\)[[:space:]].*/\1\t4096/" \
    -e "s/^\([[:space:]]*MultiDeliverMaxRCPTTo\)[[:space:]].*/\1\t24/" \
    -e "s/^\([[:space:]]*HasHTTP\)[[:space:]].*/\1\t\tyes/" \
    -e "s/^\([[:space:]]*HTTPPort\)[[:space:]].*/\1\t${HTTP_BACKEND_PORT}/" \
    -e "s/^\([[:space:]]*ManifestWeb\)[[:space:]].*/\1\tvoucher/" \
    "$SERVERS_CONF"
fi

sed -i "s|^HTTPServerName.*|HTTPServerName $(escape_sed "$DISPLAY_NAME") onionmail server|" /etc/onionmail/http.conf
if [ ! -f /etc/onionmail/om.json ]; then
  sed -i "s/^\(AutoDeleteReaded\)[[:space:]].*/\1\tno/" /etc/onionmail/pop3.conf
fi

# Captcha is required for self-service registration.
if [ -f "${WWWROOT}/config.denied.conf" ]; then
  sed -i "s/^\(captcha-disable\)[[:space:]].*/\1\t${CAPTCHA_DISABLE}/" "${WWWROOT}/config.denied.conf"
fi

# Add the webmail entry to the site menu (idempotent).
if [ -f "${WWWROOT}/menu.denied.conf" ] && ! grep -q 'href="/web"' "${WWWROOT}/menu.denied.conf"; then
  sed -i '/href="\/what.html"/i <div><a href="/web">Web</a></div>' "${WWWROOT}/menu.denied.conf"
fi

# Brand the static pages: replace the nickname placeholder, but keep the
# /<!--#NICK#-->.pem and /<!--#NICK#-->.asc URLs so the server can resolve them.
export OM_DISPLAY_NAME="$DISPLAY_NAME"
find "$WWWROOT" -type f -name '*.html' -print0 | while IFS= read -r -d '' f; do
  perl -0pi -e 's{(?<!/)<!--#NICK#-->}{$ENV{OM_DISPLAY_NAME}}g' "$f"
done
unset OM_DISPLAY_NAME

# The upstream Logon page is the sysop/admin panel. It authenticates with the
# POP3 password, not the SMTP password; label it to avoid confusion.
if [ -f "${WWWROOT}/logon.html" ]; then
  sed -i 's|<tr><td>Password:</td>|<tr><td>Password (POP3, not SMTP):</td>|' "${WWWROOT}/logon.html"
  if ! grep -q 'Admin panel password is the POP3 password' "${WWWROOT}/logon.html"; then
    sed -i 's|<p>Logon to |<div class="minfo">Admin panel password is the POP3 password, not the SMTP password.</div>\n<p>Logon to |' "${WWWROOT}/logon.html"
  fi
fi

chown -R debian-onionmail:debian-onionmail "${WWWROOT}" 2>/dev/null || true

mkdir -p /var/lib/onionmail/webmail/sent
chown -R debian-onionmail:debian-onionmail /var/lib/onionmail/webmail
chmod 700 /var/lib/onionmail/webmail /var/lib/onionmail/webmail/sent

if [ "$SSH_CTRL_ENABLED" = "1" ]; then
  if [ -f /etc/onionmail/ssh/sshd.conf ] && [ -x /usr/sbin/sshd ]; then
    mkdir -p /run/sshd
    if /usr/sbin/sshd -t -f /etc/onionmail/ssh/sshd.conf 2>/tmp/sshd-control-check; then
      /usr/sbin/sshd -f /etc/onionmail/ssh/sshd.conf -E /var/log/onionmail/sshd-control.log
      log "encrypted control port on 127.0.0.1:${SSH_CTRL_PORT} (${SSH_CTRL_USER}@127.0.0.1)"
    else
      log "ERROR: encrypted control port config is invalid: $(cat /tmp/sshd-control-check 2>/dev/null)"
    fi
  else
    log "ERROR: encrypted control port is enabled but sshd/config is missing"
  fi
fi

log "starting OnionMail server"
su debian-onionmail -c "/bin/onionmail -M:server" &
OM_PID=$!

if [ "$SKIP_TOR" = "1" ]; then
  log "skip-tor: not waiting for the control port on ${CTRL_IP}:${CTRL_PORT}"
  if ! kill -0 "$OM_PID" 2>/dev/null; then
    log "WARNING: OnionMail exited during startup; front stays available on localhost"
  fi
else
  log "waiting for the control port on ${CTRL_IP}:${CTRL_PORT}"
  control_up=0
  for _ in $(seq 1 90); do
    if ! kill -0 "$OM_PID" 2>/dev/null; then
      tor_error "OnionMail exited during startup; see /var/log/onionmail/start.log"
      KEEP_ALIVE=1
      break
    fi
    if timeout 2 bash -c "exec 3<>/dev/tcp/${CTRL_IP}/${CTRL_PORT}" 2>/dev/null; then
      control_up=1
      break
    fi
    sleep 1
  done
  if [ "$control_up" != "1" ]; then
    tor_error "control port ${CTRL_IP}:${CTRL_PORT} did not open in time; OnionMail may still be booting"
    KEEP_ALIVE=1
  fi

  # TLS certificate SHA-1, shown to new users so they can pin it.
  FP=""
  for _ in $(seq 1 30); do
    FP="$(timeout 10 openssl s_client -starttls smtp -connect 127.0.0.1:${SMTP_PORT} </dev/null 2>/dev/null \
          | openssl x509 -noout -fingerprint -sha1 2>/dev/null | cut -d= -f2 || true)"
    [ -n "$FP" ] && break
    sleep 1
  done
  [ -n "$FP" ] && printf '%s\n' "$FP" > /var/lib/onionmail/cert.sha1
fi

if [ "$SKIP_TOR" = "1" ]; then
  log "skip-tor: not waiting for the built-in HTTP server on 127.0.0.1:${HTTP_BACKEND_PORT}"
else
  log "waiting for the built-in HTTP server on 127.0.0.1:${HTTP_BACKEND_PORT}"
  http_up=0
  for _ in $(seq 1 90); do
    if ! kill -0 "$OM_PID" 2>/dev/null; then
      tor_error "OnionMail exited during startup; see /var/log/onionmail/start.log"
      KEEP_ALIVE=1
      break
    fi
    if timeout 2 bash -c "exec 3<>/dev/tcp/127.0.0.1/${HTTP_BACKEND_PORT}" 2>/dev/null; then
      http_up=1
      break
    fi
    sleep 1
  done
  if [ "$http_up" != "1" ]; then
    tor_error "built-in HTTP server 127.0.0.1:${HTTP_BACKEND_PORT} did not open in time"
    KEEP_ALIVE=1
  fi
fi

log "starting the front on ${WEB_BIND}:${WEB_PORT} (onion port 80)"
NICK_SH=${NICK//\'/\'\\\'\'}
DISPLAY_SH=${DISPLAY_NAME//\'/\'\\\'\'}
su debian-onionmail -c "env ONIONMAIL_NICK='${NICK_SH}' ONIONMAIL_DISPLAY_NAME='${DISPLAY_SH}' ONIONMAIL_THEME='${DEFAULT_THEME}' WEB_BIND='${WEB_BIND}' WEB_PORT='${WEB_PORT}' /usr/local/bin/om-front" &
FRONT_PID=$!

cleanup() {
  log "shutting down"
  kill "${FRONT_PID:-}" "${OM_PID:-}" 2>/dev/null || true
  if [ "$USE_HOST_TOR" != "1" ]; then
    service tor stop >/dev/null 2>&1 || true
  fi
}
trap cleanup TERM INT

if [ "$SKIP_TOR" = "1" ]; then
  log "ready: localhost only (Tor skipped)"
  wait "$FRONT_PID"
elif [ "${KEEP_ALIVE:-0}" = "1" ]; then
  log "ready: front only (OnionMail control unavailable)"
  wait "$FRONT_PID"
else
  log "ready: ${ONION}"
  wait -n "$OM_PID" "$FRONT_PID"
fi
