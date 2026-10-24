#!/usr/bin/env python3
"""Issue or check OnionMail voucher codes through the local control port.

Usage:
  om-voucher [days]            create a code (days defaults to 0)
  om-voucher check <code>      check a code
"""

import os
import socket
import sys

NICK = os.environ.get("ONIONMAIL_NICK", "onionmail")
SERVER_DIR = f"/etc/onionmail/servers.d/1-{NICK}.conf"
CONTROL_CONF = "/etc/onionmail/control.conf"
CTRL_IP = "127.0.0.1"


def read_conf():
    service_pass = None
    with open(SERVER_DIR, "r", encoding="utf-8", errors="replace") as fh:
        for line in fh:
            parts = line.split()
            if len(parts) >= 2 and parts[0].lower() == "passwd":
                service_pass = parts[1]
                break
    ctrl_port = 9010
    with open(CONTROL_CONF, "r", encoding="utf-8", errors="replace") as fh:
        for line in fh:
            parts = line.split()
            if len(parts) >= 2 and parts[0].lower() == "controlport":
                ctrl_port = int(parts[1])
                break
    if not service_pass:
        raise RuntimeError("cannot read the service password from the server config")
    return service_pass, ctrl_port


def control(command):
    service_pass, ctrl_port = read_conf()
    with socket.create_connection((CTRL_IP, ctrl_port), timeout=15) as sock:
        sock.settimeout(15)
        fh = sock.makefile("rwb", buffering=0)

        def readline():
            line = fh.readline()
            if not line:
                raise RuntimeError("control port closed the connection")
            return line.decode("utf-8", "replace").rstrip("\r\n")

        readline()
        fh.write(f"server {NICK} {service_pass}\n".encode())
        auth = readline()
        if not auth.startswith("+OK"):
            raise RuntimeError(f"control authentication failed: {auth}")
        fh.write((command + "\n").encode())
        reply = readline()
        fh.write(b"quit\n")
        return reply


def main(argv):
    if argv and argv[0] in ("-h", "--help"):
        sys.stdout.write(__doc__)
        return 0
    if argv and argv[0] == "check":
        if len(argv) < 2:
            sys.stderr.write("usage: om-voucher check <code>\n")
            return 2
        reply = control(f"voucherchk {argv[1]}")
        sys.stdout.write(reply + "\n")
        return 0 if reply.startswith("+OK") else 1
    days = "0"
    if argv:
        days = argv[0]
    if not days.lstrip("-").isdigit():
        sys.stderr.write("usage: om-voucher [days]\n")
        return 2
    reply = control(f"vouchermk {int(days)}")
    if not reply.startswith("+OK"):
        sys.stderr.write(reply + "\n")
        return 1
    sys.stdout.write(reply[3:].strip() + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
