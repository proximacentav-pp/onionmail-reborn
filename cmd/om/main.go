package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"onionmail/internal/olang"
	"onionmail/internal/omcore"
	"onionmail/internal/omtui"
)

const omVersion = "beta 0.6"

func main() {
	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "-v", "--version", "version":
		versionCmd()
		return
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "om: root privileges required (uid 0)")
		os.Exit(1)
	}
	switch cmd {
	case "-h", "--help", "help":
		fmt.Println("Usage: om [setup|status|version|check|ap|skip-tor]")
		fmt.Println("  setup    interactive purple TUI")
		fmt.Println("  status   show current state")
		fmt.Println("  version  show om and OnionMail versions")
		fmt.Println("  -v, --version  same as version")
		fmt.Println("  check    validate configuration")
		fmt.Println("  ap       admin panel:")
		fmt.Println("           om ap disable|enable|hide|show|passwd|data")
		fmt.Println("  controlport  local control port:")
		fmt.Println("           om controlport rootpass --set PASS")
		fmt.Println("           om controlport port --set PORT | --show")
		fmt.Println("           om controlport enable|disable|status")
		fmt.Println("           om controlport ssh enable|disable")
		fmt.Println("           om controlport ssh-port --set PORT | --show")
		fmt.Println("           om controlport ssh-user --set USER | --show")
		fmt.Println("           om controlport ssh-mode sshtunnel|unactive")
		fmt.Println("           om controlport connect [--encrypted]")
		fmt.Println("  skip-tor  disable onion and startup waits (localhost only):")
		fmt.Println("           om skip-tor [on|off|status]")
		fmt.Println("  voucher  manage vouchers:")
		fmt.Println("           om voucher --gen --days N")
		fmt.Println("           om voucher --rm CODE")
		fmt.Println("           om voucher --ls [--limit N]")
	case "__boot-env":
		omcore.BootEnv()
	case "__apply":
		if err := omcore.ApplySafe(omcore.LoadState()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "__torrc":
		omcore.WriteTorrc(omcore.LoadState())
	case "status":
		statusCmd()
	case "check":
		st := omcore.LoadState()
		errs := append(omcore.Validate(st), omcore.TorConfigErrors(st)...)
		if len(errs) > 0 {
			fmt.Println(strings.Join(errs, "\n"))
			os.Exit(2)
		}
		fmt.Println("OK")
	case "ap":
		os.Exit(apCmd(os.Args[2:]))
	case "controlport":
		os.Exit(controlportCmd(os.Args[2:]))
	case "skip-tor":
		os.Exit(skipTorCmd(os.Args[2:]))
	case "voucher":
		os.Exit(voucherCmd(os.Args[2:]))
	case "setup", "":
		st := omcore.LoadState()
		b, err := olang.Load("")
		if err != nil {
			fmt.Fprintln(os.Stderr, "om setup:", err)
			os.Exit(1)
		}
		os.Exit(omtui.Run(st, b))
	default:
		fmt.Fprintf(os.Stderr, "om: unknown command %s\n", cmd)
		os.Exit(1)
	}
}

func versionCmd() {
	fmt.Println("om version: " + omVersion)
	fmt.Println("onionmail version " + onionmailVersion())
}

func onionmailVersion() string {
	out, _ := exec.Command("/bin/onionmail", "-version").CombinedOutput()
	re := regexp.MustCompile(`(?m)^\s*OnionMail Ver\.\s*([0-9]+(?:\.[0-9]+)+)`)
	if m := re.FindStringSubmatch(string(out)); len(m) == 2 {
		return m[1]
	}
	return "1.12"
}

func statusCmd() {
	st := omcore.LoadState()
	live := omcore.DeriveState()
	conf := omcore.FindServerConf(omcore.Str(live, "nick"))
	if conf == "" {
		conf = omcore.FindServerConf("")
	}
	lim := omcore.Obj(live, "limits")
	reg := omcore.Obj(st, "registration")
	if conf == "" {
		conf = "-"
	}
	fmt.Println("config:  " + conf)
	fmt.Println("nick:    " + omcore.Str(live, "nick"))
	fmt.Println("server:  " + omcore.Str(live, "server_name"))
	fmt.Println("onion:   " + omcore.Str(live, "onion"))
	lang := omcore.Str(st, "locale")
	if lang == "" {
		lang = omcore.StrDef(live, "locale", "")
	}
	theme := omcore.StrDef(omcore.Obj(st, "design"), "theme", "light")
	fmt.Println("lang:    " + lang)
	fmt.Println("theme:   " + theme)
	fmt.Printf("limits:  mail=%s msg=%sMB box=%sMB ml=%s rcpt=%s spam=%s\n",
		fmt.Sprint(lim["max_messages"]), fmt.Sprint(lim["max_message_mb"]), fmt.Sprint(lim["box_mb"]),
		fmt.Sprint(lim["max_ml"]), fmt.Sprint(lim["max_rcpt"]), fmt.Sprint(lim["max_spam"]))
	fmt.Printf("flags:   debug=%s autodelete=%s exit=%s logvoucher=%s\n",
		onoff(omcore.Bool(lim, "debug", false)), onoff(omcore.Bool(lim, "autodelete", false)),
		onoff(omcore.Bool(lim, "exit", false)), onoff(omcore.Bool(lim, "logvoucher", false)))
	fmt.Printf("friends: %d\n", len(omcore.FriendsList(live)))
	admin := "enabled"
	if !omcore.Bool(st, "admin_enabled", true) {
		admin = "disabled"
	}
	fmt.Println("admin:   " + admin + " (front state)")
	fmt.Printf("reg:     captcha=%s vouchers=%s exit=%s\n",
		onoff(omcore.Bool(reg, "captcha", true)),
		map[bool]string{true: "required", false: "optional"}[omcore.Bool(reg, "voucher_required", false)],
		onoff(omcore.Bool(reg, "exit_enabled", true)))
	vs := omcore.Arr(st, "vouchers")
	fmt.Printf("vouchers: %d\n", len(vs))
	tor := omcore.Obj(live, "tor")
	mode := omcore.StrDef(omcore.Obj(st, "tor"), "mode", omcore.Str(tor, "mode"))
	ip := omcore.ParseKV(omcore.Read(omcore.CoreConf), "TorIP")
	if ip == "" {
		ip = omcore.StrDef(tor, "ip", "127.0.0.1")
	}
	port := omcore.ParseKV(omcore.Read(omcore.CoreConf), "TorPort")
	if port == "" {
		port = fmt.Sprint(omcore.Int(tor, "port", 9050))
	}
	state := "disabled"
	if mode == "internal" {
		state = omcore.TorProgress()
	} else if mode == "external" {
		state = "unreachable"
		var p int
		fmt.Sscan(port, &p)
		if omcore.TorSocketOK(ip, p) {
			state = "reachable"
		}
	}
	fmt.Println("tor:     " + mode + " " + ip + ":" + port + " " + state)
	var drift []string
	checks := [][3]string{
		{"nick", omcore.Str(st, "nick"), omcore.Str(live, "nick")},
		{"server", omcore.Str(st, "server_name"), omcore.Str(live, "server_name")},
		{"onion", omcore.Str(st, "onion"), omcore.Str(live, "onion")},
		{"relay", fmt.Sprint(omcore.Bool(st, "relay", false)), fmt.Sprint(omcore.Bool(live, "relay", false))},
	}
	for _, k := range []string{"max_messages", "max_message_mb", "box_mb", "max_ml", "max_rcpt", "max_spam"} {
		checks = append(checks, [3]string{k, fmt.Sprint(omcore.Obj(st, "limits")[k]), fmt.Sprint(lim[k])})
	}
	for _, c := range checks {
		if c[1] != c[2] {
			drift = append(drift, c[0])
		}
	}
	if len(drift) == 0 {
		fmt.Println("sync:    om.json matches current config")
	} else {
		fmt.Println("sync:    differs: " + strings.Join(drift, ", "))
	}
	fmt.Println("state:   " + omcore.StatePath)
}

func onoff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

type voucherRow struct {
	code    string
	created string
	expires string
	days    string
}

func voucherRows(st omcore.State) []voucherRow {
	out := make([]voucherRow, 0)
	for _, v := range omcore.Arr(st, "vouchers") {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		row := voucherRow{
			code:    omcore.Str(m, "code"),
			created: omcore.Str(m, "created"),
			expires: omcore.Str(m, "expires"),
		}
		if row.days = omcore.Str(m, "days"); row.days == "" {
			if n := omcore.Int(m, "days", 0); n > 0 {
				row.days = strconv.Itoa(n)
			}
		}
		if row.expires == "" {
			if n := omcore.Int(m, "days", 0); n > 0 {
				if t, err := time.ParseInLocation("2006-01-02 15:04", row.created, time.Local); err == nil {
					row.expires = t.AddDate(0, 0, n).Format("2006-01-02")
				}
			}
		}
		if row.days == "" {
			row.days = "-"
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].expires == out[j].expires {
			return out[i].created > out[j].created
		}
		if out[i].expires == "" {
			return false
		}
		if out[j].expires == "" {
			return true
		}
		return out[i].expires > out[j].expires
	})
	return out
}

func voucherCmd(args []string) int {
	mode := ""
	days := 30
	limit := 0
	code := ""
	usage := func() {
		fmt.Fprintln(os.Stderr, "Usage: om voucher --gen --days N | --rm CODE | --ls [--limit N]")
	}
	next := func(i *int) (string, bool) {
		if *i+1 >= len(args) {
			return "", false
		}
		*i++
		return args[*i], true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--gen":
			if mode != "" && mode != "gen" {
				usage()
				return 1
			}
			mode = "gen"
		case a == "--rm":
			if mode != "" && mode != "rm" {
				usage()
				return 1
			}
			mode = "rm"
			v, ok := next(&i)
			if !ok {
				usage()
				return 1
			}
			code = v
		case a == "--ls":
			if mode != "" && mode != "ls" {
				usage()
				return 1
			}
			mode = "ls"
		case a == "--days" || strings.HasPrefix(a, "--days="):
			v := strings.TrimPrefix(a, "--days=")
			if a == "--days" {
				var ok bool
				v, ok = next(&i)
				if !ok {
					usage()
					return 1
				}
			}
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 1 || n > 36500 {
				fmt.Fprintln(os.Stderr, "om voucher: days must be 1..36500")
				return 1
			}
			days = n
		case a == "--limit" || strings.HasPrefix(a, "--limit="):
			v := strings.TrimPrefix(a, "--limit=")
			if a == "--limit" {
				var ok bool
				v, ok = next(&i)
				if !ok {
					usage()
					return 1
				}
			}
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n < 0 || n > 8192 {
				fmt.Fprintln(os.Stderr, "om voucher: limit must be 0..8192")
				return 1
			}
			limit = n
		case a == "-h" || a == "--help":
			usage()
			return 0
		default:
			fmt.Fprintf(os.Stderr, "om voucher: unknown argument %s\n", a)
			usage()
			return 1
		}
	}
	if mode == "" {
		usage()
		return 1
	}
	st := omcore.LoadState()
	switch mode {
	case "gen":
		line, err := omcore.ControlCommand(fmt.Sprintf("vouchermk %d", days*24*60), 20*time.Second)
		if err != nil {
			fmt.Fprintln(os.Stderr, "om voucher: "+err.Error())
			return 1
		}
		if !strings.HasPrefix(line, "+OK") {
			fmt.Fprintln(os.Stderr, "om voucher: "+line)
			return 1
		}
		code := strings.TrimSpace(strings.TrimPrefix(line, "+OK"))
		if code == "" {
			fmt.Fprintln(os.Stderr, "om voucher: empty code from server")
			return 1
		}
		now := time.Now()
		vs := omcore.Arr(st, "vouchers")
		vs = append(vs, map[string]any{
			"code": code, "created": now.Format("2006-01-02 15:04"),
			"days": days, "expires": now.AddDate(0, 0, days).Format("2006-01-02"),
		})
		st["vouchers"] = vs
		if err := omcore.StateSave(st); err != nil {
			fmt.Fprintln(os.Stderr, "om voucher: warning: state not saved: "+err.Error())
		}
		fmt.Println(code)
		return 0
	case "rm":
		if strings.TrimSpace(code) == "" {
			usage()
			return 1
		}
		line, err := omcore.ControlCommand("voucher "+code, 20*time.Second)
		if err != nil {
			fmt.Fprintln(os.Stderr, "om voucher: "+err.Error())
			return 1
		}
		if !strings.HasPrefix(line, "+OK") {
			fmt.Fprintln(os.Stderr, "om voucher: "+line)
			return 1
		}
		vs := omcore.Arr(st, "vouchers")
		keep := make([]any, 0, len(vs))
		for _, v := range vs {
			m, _ := v.(map[string]any)
			if omcore.Str(m, "code") != code {
				keep = append(keep, v)
			}
		}
		st["vouchers"] = keep
		if err := omcore.StateSave(st); err != nil {
			fmt.Fprintln(os.Stderr, "om voucher: warning: state not saved: "+err.Error())
		}
		fmt.Println("Voucher deleted: " + code)
		return 0
	case "ls":
		rows := voucherRows(st)
		if limit > 0 && len(rows) > limit {
			rows = rows[:limit]
		}
		fmt.Printf("%-40s  %-16s  %-10s  %4s  %s\n", "CODE", "CREATED", "EXPIRES", "DAYS", "STATUS")
		today := time.Now().Format("2006-01-02")
		for _, r := range rows {
			status := "active"
			if r.expires != "" && r.expires < today {
				status = "expired"
			}
			fmt.Printf("%-40s  %-16s  %-10s  %4s  %s\n", r.code, r.created, r.expires, r.days, status)
		}
		return 0
	}
	return 1
}
