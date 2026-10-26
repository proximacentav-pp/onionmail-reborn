package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"onionmail/internal/omcore"
)

var apInput = bufio.NewReader(os.Stdin)

func apCmd(args []string) int {
	if len(args) == 0 {
		apUsage()
		return 2
	}
	switch args[0] {
	case "disable":
		return apSetState("disable", args[1:])
	case "enable":
		return apSetState("enable", args[1:])
	case "hide":
		return apSetState("hide", args[1:])
	case "show":
		return apSetState("show", args[1:])
	case "passwd":
		return apPasswd(args[1:])
	case "data":
		return apData(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "om ap: unknown command %s\n", args[0])
		apUsage()
		return 1
	}
}

func apUsage() {
	fmt.Fprintln(os.Stderr, "Usage: om ap <disable|enable|hide|show|passwd|data>")
	fmt.Fprintln(os.Stderr, "  disable         disable admin-panel login")
	fmt.Fprintln(os.Stderr, "  enable          enable admin-panel login")
	fmt.Fprintln(os.Stderr, "  hide            disable the panel and hide its menu entry")
	fmt.Fprintln(os.Stderr, "  show [--enable] show the menu entry (and enable the panel)")
	fmt.Fprintln(os.Stderr, "  passwd [--gen [N]]  set a new admin-panel password")
	fmt.Fprintln(os.Stderr, "  data [stats|server|net] [--last] [--update]")
}

func apSetState(action string, args []string) int {
	enable := false
	if action == "show" {
		for _, a := range args {
			switch a {
			case "--enable":
				enable = true
			default:
				apUsage()
				return 2
			}
		}
	} else if len(args) > 0 {
		apUsage()
		return 2
	}
	st := omcore.LoadState()
	switch action {
	case "disable":
		st["admin_enabled"] = false
	case "enable":
		st["admin_enabled"] = true
	case "hide":
		st["admin_enabled"] = false
		st["admin_hidden"] = true
	case "show":
		st["admin_hidden"] = false
		if enable {
			st["admin_enabled"] = true
		}
	}
	if err := omcore.StateSave(st); err != nil {
		fmt.Fprintln(os.Stderr, "om ap:", err)
		return 1
	}
	switch action {
	case "disable":
		fmt.Println("Admin panel disabled.")
	case "enable":
		fmt.Println("Admin panel enabled.")
	case "hide":
		fmt.Println("Admin panel hidden and disabled.")
	case "show":
		if enable {
			fmt.Println("Admin panel menu shown and enabled.")
		} else {
			fmt.Println("Admin panel menu shown.")
		}
	}
	return 0
}

func apPasswd(args []string) int {
	generate := false
	length := 32
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--gen":
			generate = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				n, err := strconv.Atoi(args[i+1])
				if err != nil {
					fmt.Fprintln(os.Stderr, "om ap passwd: invalid length")
					return 2
				}
				length = n
				i++
			}
		case strings.HasPrefix(args[i], "--gen="):
			generate = true
			n, err := strconv.Atoi(strings.TrimPrefix(args[i], "--gen="))
			if err != nil {
				fmt.Fprintln(os.Stderr, "om ap passwd: invalid length")
				return 2
			}
			length = n
		default:
			fmt.Fprintln(os.Stderr, "Usage: om ap passwd [--gen [N]]")
			return 2
		}
	}
	var pass string
	if generate {
		if length < 0 || length > 512 {
			fmt.Fprintln(os.Stderr, "om ap passwd: length must be between 0 and 512")
			return 2
		}
		var err error
		pass, err = randomPassword(length)
		if err != nil {
			fmt.Fprintln(os.Stderr, "om ap passwd:", err)
			return 1
		}
	} else {
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
			line, err := apInput.ReadString('\n')
			if err != nil && line == "" {
				fmt.Fprintln(os.Stderr, "om ap passwd:", err)
				return 1
			}
			pass = strings.TrimRight(line, "\r\n")
		} else {
			fmt.Print("New admin panel password: ")
			line, err := readHiddenLine()
			if err != nil {
				fmt.Fprintln(os.Stderr, "\nom ap passwd:", err)
				return 1
			}
			pass = strings.TrimRight(line, "\r\n")
		}
		if pass == "" {
			fmt.Fprintln(os.Stderr, "om ap passwd: empty password")
			return 2
		}
	}
	st := omcore.LoadState()
	st["admin_password"] = pass
	if pass == "" {
		mode := os.FileMode(0o600)
		if err := omcore.WriteKeep(omcore.ServicePass, "", &mode); err != nil {
			fmt.Fprintln(os.Stderr, "om ap passwd:", err)
			return 1
		}
		if conf := omcore.FindServerConf(omcore.StrDef(st, "nick", "")); conf != "" {
			text := omcore.SetKeyServer(omcore.Read(conf), "PassWD", "")
			_ = omcore.WriteKeep(conf, text, nil)
		}
		if err := omcore.StateSave(st); err != nil {
			fmt.Fprintln(os.Stderr, "om ap passwd:", err)
			return 1
		}
	} else if err := omcore.ApplySafe(st); err != nil {
		fmt.Fprintln(os.Stderr, "om ap passwd:", err)
		return 1
	}
	nick := omcore.StrDef(st, "nick", "onionmail")
	if generate {
		fmt.Printf("Admin password for %s: %s\n", nick, pass)
	} else {
		fmt.Printf("Admin password for %s updated.\n", nick)
	}
	if len(omcore.OnionmailPIDs()) == 0 {
		fmt.Println("OnionMail is not running. Start it to apply the password.")
		return 0
	}
	if confirmRestart() {
		fmt.Println("Restarting OnionMail to apply the password...")
		omcore.Restart()
	} else {
		fmt.Println("Restart skipped. Changes will apply after the next server restart.")
	}
	return 0
}

func confirmRestart() bool {
	fmt.Print("restart server to apply changes: Y/N: ")
	line, err := apInput.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println("N")
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "д", "да":
		return true
	}
	return false
}

func readHiddenLine() (string, error) {
	stty := exec.Command("stty", "-echo")
	stty.Stdin = os.Stdin
	hasTTY := stty.Run() == nil
	if hasTTY {
		defer func() {
			restore := exec.Command("stty", "echo")
			restore.Stdin = os.Stdin
			_ = restore.Run()
		}()
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println()
	return line, err
}

func randomPassword(n int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	out := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
}

func apData(args []string) int {
	category := ""
	last := false
	update := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--last":
			last = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				category = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--last="):
			last = true
			category = strings.TrimPrefix(a, "--last=")
		case a == "--update":
			update = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(os.Stderr, "om ap data: unknown option", a)
			return 2
		default:
			if category != "" {
				fmt.Fprintln(os.Stderr, "Usage: om ap data [stats|server|net] [--last] [--update]")
				return 2
			}
			category = a
		}
	}
	if category == "" {
		if update {
			category = "server"
		} else {
			category = "stats"
		}
	}
	switch category {
	case "stats":
		return apDataStats(last)
	case "server":
		return apDataServer(last, update)
	case "net":
		return apDataNet(last)
	default:
		fmt.Fprintln(os.Stderr, "om ap data: category must be stats, server or net")
		return 2
	}
}

type apRegistry struct {
	Users map[string]map[string]any `json:"users"`
}

func apRegistryPath() string { return "/var/lib/onionmail/webmail/registry.json" }
func apHistoryPath() string  { return "/var/lib/onionmail/webmail/server-history.jsonl" }

func apDataStats(last bool) int {
	st := omcore.LoadState()
	nick := omcore.StrDef(st, "nick", "onionmail")
	mailDir := "/var/onionmail/" + nick
	users := 0
	known := 0
	pop3 := ""
	var reg apRegistry
	if b, err := os.ReadFile(apRegistryPath()); err == nil {
		_ = json.Unmarshal(b, &reg)
	}
	for _, u := range reg.Users {
		users++
		v := strings.TrimSpace(fmt.Sprint(u["pop3"]))
		if v != "" && v != "<nil>" {
			known++
		}
	}
	inboxSize := apDirSize(mailDir + "/inbox")
	sentSize := apDirSize("/var/lib/onionmail/webmail/sent")
	userSize := apDirSize(mailDir+"/usr") + apDirSize(mailDir+"/keys")
	if fi, err := os.Stat(apRegistryPath()); err == nil {
		userSize += fi.Size()
	}
	total := apDirSize(mailDir) + apDirSize("/var/lib/onionmail") + apDirSize("/etc/onionmail")
	service := total - inboxSize - sentSize - userSize
	if service < 0 {
		service = 0
	}
	msgCount := apCountFiles(mailDir+"/inbox", ".esf") + apCountFiles("/var/lib/onionmail/webmail/sent", ".json")
	diskTotal, diskFree := apDisk()
	exits, bad := apFeedLists()
	if last {
		fmt.Printf("%s users=%d inbox=%s sent=%s user-data=%s service=%s free=%s/%s\n",
			nick, users, apSize(inboxSize), apSize(sentSize), apSize(userSize), apSize(service),
			apSize(diskFree), apSize(diskTotal))
		return 0
	}
	fmt.Printf("Server: %s (%s)\n", nick, omcore.Str(st, "server_name"))
	fmt.Printf("Onion:  %s\n", omcore.Str(st, "onion"))
	fmt.Printf("Users:  %d (with POP3 password: %d)\n", users, known)
	fmt.Printf("Messages in mailboxes: %d\n", msgCount)
	fmt.Printf("Inbox:  %s\n", apSize(inboxSize))
	fmt.Printf("Sent:   %s\n", apSize(sentSize))
	fmt.Printf("User data:    %s\n", apSize(userSize))
	fmt.Printf("Service files: %s\n", apSize(service))
	fmt.Printf("Free disk: %s of %s\n", apSize(diskFree), apSize(diskTotal))
	fmt.Printf("Uptime: %s\n", apDuration(apUptime()))
	fmt.Printf("Exit / Friends: %d / %d\n", exits, len(omcore.FriendsList(st)))
	_ = pop3
	_ = bad
	return 0
}

type apSample struct {
	T       int64 `json:"t"`
	Recv    int64 `json:"r"`
	Sent    int64 `json:"s"`
	Service int64 `json:"sv"`
	Other   int64 `json:"o"`
	H2      int64 `json:"h2"`
	H403    int64 `json:"h403"`
	H4      int64 `json:"h4"`
	H5      int64 `json:"h5"`
}

func apReadHistory() []apSample {
	b, err := os.ReadFile(apHistoryPath())
	if err != nil {
		return nil
	}
	var out []apSample
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var sm apSample
		if json.Unmarshal([]byte(line), &sm) == nil && sm.T > 0 {
			out = append(out, sm)
		}
	}
	return out
}

func apDataServer(last, update bool) int {
	history := apReadHistory()
	var sm apSample
	if update {
		before := len(history)
		if apTriggerSample(before) {
			history = apReadHistory()
			if len(history) > 0 {
				sm = history[len(history)-1]
			}
		} else {
			sm = apWriteSample()
			history = append(history, sm)
		}
	} else if len(history) > 0 {
		sm = history[len(history)-1]
	} else {
		sm = apLiveSample()
	}
	st := omcore.LoadState()
	nick := omcore.StrDef(st, "nick", "onionmail")
	if last {
		fmt.Printf("%s %s received=%s sent=%s service=%s other=%s http=2xx:%d 403:%d 4xx:%d 5xx:%d\n",
			nick, time.Unix(sm.T, 0).Format("2006-01-02 15:04:05"),
			apSize(sm.Recv), apSize(sm.Sent), apSize(sm.Service), apSize(sm.Other), sm.H2, sm.H403, sm.H4, sm.H5)
		return 0
	}
	fmt.Printf("Server: %s (%s)\n", nick, omcore.Str(st, "server_name"))
	fmt.Printf("Samples: %d\n", len(history))
	fmt.Printf("Latest check: %s\n", time.Unix(sm.T, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf("Received mail:  %s\n", apSize(sm.Recv))
	fmt.Printf("Sent mail:      %s\n", apSize(sm.Sent))
	fmt.Printf("Service files:  %s\n", apSize(sm.Service))
	fmt.Printf("Other:          %s\n", apSize(sm.Other))
	fmt.Printf("HTTP: 2xx=%d 403=%d 4xx=%d 5xx=%d\n", sm.H2, sm.H403, sm.H4, sm.H5)
	return 0
}

func apDataNet(last bool) int {
	st := omcore.LoadState()
	friends := omcore.FriendsList(st)
	exits, bad := apFeedLists()
	if last {
		fmt.Printf("friends=%d exits=%d bad-exits=%d\n", len(friends), exits, bad)
		return 0
	}
	fmt.Printf("Friends: %d\n", len(friends))
	for _, f := range friends {
		fmt.Println("  " + f)
	}
	fmt.Printf("Exit routes: %d\n", exits)
	fmt.Printf("Bad exit routes: %d\n", bad)
	return 0
}

func apFeedLists() (int, int) {
	text := omcore.Read("/etc/onionmail/feed.conf")
	count := func(name string) int {
		re := regexp.MustCompile(`(?is)\b` + name + `\s*\{(.*?)\}`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			return 0
		}
		n := 0
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				n++
			}
		}
		return n
	}
	return count("ExitGood"), count("ExitBad")
}

func apSize(num int64) string {
	if num < 0 {
		num = 0
	}
	kb := float64(num) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	return fmt.Sprintf("%.1f GB", mb/1024)
}

func apDuration(sec int) string {
	if sec <= 0 {
		return "0m"
	}
	d := sec / 86400
	h := (sec % 86400) / 3600
	m := (sec % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func apUptime() int {
	st, err := os.Stat("/proc/1")
	if err != nil {
		return 0
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(time.Since(time.Unix(int64(sys.Ctim.Sec), int64(sys.Ctim.Nsec))).Seconds())
	}
	return int(time.Since(st.ModTime()).Seconds())
}

func apDisk() (int64, int64) {
	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) != nil {
		return 0, 0
	}
	return int64(fs.Blocks) * int64(fs.Bsize), int64(fs.Bavail) * int64(fs.Bsize)
}

func apDirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func apCountFiles(path, suffix string) int {
	n := 0
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && strings.HasSuffix(info.Name(), suffix) {
			n++
		}
		return nil
	})
	return n
}

func apLiveSample() apSample {
	recv, sent, service, other := apDiskCategories()
	return apSample{T: time.Now().Unix(), Recv: recv, Sent: sent, Service: service, Other: other}
}

func apTriggerSample(before int) bool {
	path := filepath.Join(filepath.Dir(apHistoryPath()), ".stats-update")
	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		return false
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(apReadHistory()) > before {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	_ = os.Remove(path)
	return false
}

func apWriteSample() apSample {
	sm := apLiveSample()
	path := apHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "om ap data:", err)
		return sm
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "om ap data:", err)
		return sm
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(sm)
	return sm
}

func apDiskCategories() (recv, sent, service, other int64) {
	st := omcore.LoadState()
	nick := omcore.StrDef(st, "nick", "onionmail")
	mailDir := "/var/onionmail/" + nick
	recv = apDirSize(mailDir + "/inbox")
	sent = apDirSize("/var/lib/onionmail/webmail/sent")
	rootfs := apDU("/")
	libAll := apDirSize("/var/lib/onionmail")
	var registry int64
	if fi, err := os.Stat(apRegistryPath()); err == nil {
		registry = fi.Size()
	}
	libService := libAll - sent - registry
	if libService < 0 {
		libService = 0
	}
	etcAll := apDirSize("/etc/onionmail")
	logAll := apDirSize("/var/log/onionmail")
	service = rootfs + libService + etcAll + logAll + apDirSize(mailDir+"/log")
	total := rootfs + apDirSize(mailDir) + libAll + etcAll + logAll + apDirSize("/var/lib/tor")
	other = total - recv - sent - service
	if other < 0 {
		other = 0
	}
	return
}

func apDU(path string) int64 {
	out, _ := exec.Command("du", "-s", "-x", "-B1", path).Output()
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(fields[0], 10, 64)
	return n
}
