package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"onionmail/internal/omcore"
)

func control(cmd string) (string, error) {
	nick := os.Getenv("ONIONMAIL_NICK")
	if nick == "" {
		nick = "onionmail"
	}
	conf := "/etc/onionmail/servers.d/1-" + nick + ".conf"
	pass := ""
	for _, line := range strings.Split(omcore.Read(conf), "\n") {
		p := strings.Fields(line)
		if len(p) >= 2 && strings.EqualFold(p[0], "passwd") {
			pass = p[1]
			break
		}
	}
	if pass == "" {
		return "", fmt.Errorf("cannot read the service password from the server config")
	}
	port := omcore.ControlPort()
	return omcore.ControlAs(nick, pass, port, cmd, 15_000_000_000)
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print("Issue or check OnionMail voucher codes through the local control port.\n\nUsage:\n  om-voucher [days]            create a code (days defaults to 0)\n  om-voucher check <code>      check a code\n")
		return
	}
	if len(args) > 0 && args[0] == "check" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: om-voucher check <code>")
			os.Exit(2)
		}
		reply, err := control("voucherchk " + args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(reply)
		if !strings.HasPrefix(reply, "+OK") {
			os.Exit(1)
		}
		return
	}
	days := "0"
	if len(args) > 0 {
		days = args[0]
	}
	if _, err := strconv.Atoi(strings.TrimPrefix(days, "-")); err != nil {
		fmt.Fprintln(os.Stderr, "usage: om-voucher [days]")
		os.Exit(2)
	}
	n, _ := strconv.Atoi(days)
	if n < 0 || n > 36500 {
		fmt.Fprintln(os.Stderr, "usage: om-voucher [days]")
		os.Exit(2)
	}
	reply, err := control(fmt.Sprintf("vouchermk %d", n*24*60))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !strings.HasPrefix(reply, "+OK") {
		fmt.Fprintln(os.Stderr, reply)
		os.Exit(1)
	}
	code := strings.TrimSpace(reply[3:])
	if code != "" {
		now := time.Now()
		if err := omcore.UpdateState(func(st omcore.State) error {
			vs := omcore.Arr(st, "vouchers")
			vs = append(vs, map[string]any{
				"code": code, "created": now.Format("2006-01-02 15:04"),
				"days": n, "expires": now.AddDate(0, 0, n).Format("2006-01-02"),
			})
			st["vouchers"] = vs
			return nil
		}); err != nil {
			fmt.Fprintln(os.Stderr, "om-voucher: warning: state not saved: "+err.Error())
		}
	}
	fmt.Println(code)
}
