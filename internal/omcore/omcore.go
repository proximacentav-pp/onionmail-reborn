package omcore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	StatePath        = "/etc/onionmail/om.json"
	ServersDir       = "/etc/onionmail/servers.d"
	CoreConf         = "/etc/onionmail/core.conf"
	HTTPConf         = "/etc/onionmail/http.conf"
	POP3Conf         = "/etc/onionmail/pop3.conf"
	ControlConf      = "/etc/onionmail/control.conf"
	Torrc            = "/etc/tor/torrc"
	TorLog           = "/var/log/onionmail/tor.log"
	ServicePass      = "/etc/onionmail/.service_pass"
	WWWRoot          = "/var/lib/onionmail/wwwroot"
	VoucherLog       = "/var/log/onionmail/vouchers-tui.log"
	DefaultLogo      = "/img/OM-BL.gif"
	DefaultSmallLogo = ""
	LogoMaxBytes     = 5 * 1024 * 1024
)

var DefaultLangs = []string{"ru", "de", "fr", "en", "es"}

type State map[string]any

func Read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func ReadDefault(path, def string) string {
	if b, err := os.ReadFile(path); err == nil {
		return string(b)
	}
	return def
}

func WriteKeep(path, text string, mode *os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var oldMode os.FileMode
	var oldUID, oldGID int
	haveOld := false
	if st, err := os.Stat(path); err == nil {
		oldMode = st.Mode().Perm()
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			oldUID, oldGID = int(sys.Uid), int(sys.Gid)
		}
		haveOld = true
	}
	tmp := path + ".omtmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	m := os.FileMode(0o644)
	if mode != nil {
		m = *mode
	} else if haveOld {
		m = oldMode
	}
	_ = os.Chmod(tmp, m)
	if haveOld {
		_ = os.Chown(tmp, oldUID, oldGID)
	}
	return os.Rename(tmp, path)
}

func StateLoad() State {
	b, err := os.ReadFile(StatePath)
	if err != nil {
		return nil
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil
	}
	return st
}

func StateSave(st State) error {
	st["updated"] = time.Now().Format("2006-01-02T15:04:05")
	if err := os.MkdirAll(filepath.Dir(StatePath), 0o755); err != nil {
		return err
	}
	tmp := StatePath + ".tmp"
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o640)
	if g, err := user.LookupGroup("debian-onionmail"); err == nil {
		gid, _ := strconv.Atoi(g.Gid)
		if err := os.Chown(tmp, 0, gid); err != nil {
			_ = os.Chmod(tmp, 0o644)
		}
	} else {
		_ = os.Chmod(tmp, 0o644)
	}
	return os.Rename(tmp, StatePath)
}

func FindServerConf(nick string) string {
	if nick != "" {
		p := filepath.Join(ServersDir, "1-"+nick+".conf")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if env := os.Getenv("ONIONMAIL_NICK"); env != "" {
		p := filepath.Join(ServersDir, "1-"+env+".conf")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	files, _ := filepath.Glob(filepath.Join(ServersDir, "1-*.conf"))
	sort.Strings(files)
	if len(files) > 0 {
		return files[0]
	}
	return ""
}

func ParseKV(text, key string) string {
	re := regexp.MustCompile(`(?im)^\s*` + regexp.QuoteMeta(key) + `\s+([^\s#]+)`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

type Server struct {
	Text  string
	Nick  string
	Onion string
}

func ParseServer(path string) Server {
	text := Read(path)
	s := Server{Text: text}
	re := regexp.MustCompile(`(?im)^\s*SMTPServer\s+(\S+)\s*\{`)
	if m := re.FindStringSubmatch(text); m != nil {
		s.Nick = m[1]
	}
	s.Onion = ParseKV(text, "Onion")
	return s
}

var onionRe = regexp.MustCompile(`[a-z2-7]{16,56}\.onion`)

func ParseFriends(text string) []string {
	re := regexp.MustCompile(`(?is)\bFriends\s*\{(.*?)\}`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return onionRe.FindAllString(m[1], -1)
}

func Obj(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func Arr(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func Str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func StrDef(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func IntVal(v any, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
	}
	return def
}

func Int(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		return IntVal(v, def)
	}
	return def
}

func BoolVal(v any, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "yes", "true", "y", "1":
			return true
		case "no", "false", "n", "0":
			return false
		}
	case float64:
		return x != 0
	case int:
		return x != 0
	}
	return def
}

func Bool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		return BoolVal(v, def)
	}
	return def
}

func BoolStr(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "y", "1":
		return true
	}
	return false
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func onOff(v bool, on, off string) string {
	if v {
		return on
	}
	return off
}

func parseNum(s string, def, mult int) int {
	m := regexp.MustCompile(`^([0-9]+)\s*([MGK]?)`).FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return def
	}
	n, _ := strconv.Atoi(m[1])
	switch strings.ToUpper(m[2]) {
	case "G":
		n *= 1024
	case "K":
		n = max(1, n/1024)
	}
	return n * mult
}

func envMap() map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile("/proc/1/environ")
	if err != nil {
		return out
	}
	for _, part := range strings.Split(string(b), "\x00") {
		if i := strings.IndexByte(part, '='); i >= 0 {
			out[part[:i]] = part[i+1:]
		}
	}
	return out
}

func DeriveState() State {
	conf := FindServerConf("")
	srv := Server{}
	if conf != "" {
		srv = ParseServer(conf)
	}
	nick := srv.Nick
	if nick == "" {
		nick = StrDef(map[string]any{"v": os.Getenv("ONIONMAIL_NICK")}, "v", "")
		if nick == "" {
			nick = "onionmail"
		}
	}
	http := Read(HTTPConf)
	display := ParseKV(http, "HTTPServerName")
	display = regexp.MustCompile(`(?i)\s+onionmail\s+server\s*$`).ReplaceAllString(display, "")
	if display == "" {
		display = StrDef(map[string]any{"v": os.Getenv("ONIONMAIL_DISPLAY_NAME")}, "v", "example")
	}
	text := srv.Text
	debug := BoolStr(ParseKV(Read(CoreConf), "Debug"))
	autodel := BoolStr(ParseKV(Read(POP3Conf), "AutoDeleteReaded"))
	srvtype := strings.ToLower(ParseKV(text, "ServerType"))
	relay := BoolStr(ParseKV(text, "CanRelay"))
	logv := ParseKV(text, "LogVoucherTo") != "" || (strings.Contains(text, "LogVoucherTo") && !regexp.MustCompile(`(?im)^\s*#\s*LogVoucherTo`).MatchString(text))
	cden := Read(filepath.Join(WWWRoot, "config.denied.conf"))
	captchaDisabled := strings.ToLower(ParseKV(cden, "captcha-disable")) == "yes"
	logo := ParseKV(cden, "mainlogo")
	if logo == "" {
		logo = DefaultLogo
	}
	em := envMap()
	hostTor := "1"
	if v, ok := em["USE_HOST_TOR"]; ok {
		hostTor = v
	} else if v := os.Getenv("USE_HOST_TOR"); v != "" {
		hostTor = v
	}
	core := Read(CoreConf)
	tip := ParseKV(core, "TorIP")
	if tip == "" {
		tip = "127.0.0.1"
	}
	tport := ParseKV(core, "TorPort")
	if tport == "" {
		tport = "9050"
	}
	tportN, err := strconv.Atoi(tport)
	if err != nil {
		tportN = 9050
	}
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = "C.UTF-8"
	}
	friends := ParseFriends(Read("/etc/onionmail/feed.conf"))
	onion := srv.Onion
	if onion == "" {
		onion = em["ONION_ADDRESS"]
	}
	return State{
		"lang":           "ru",
		"server_name":    display,
		"onion":          onion,
		"nick":           nick,
		"locale":         locale,
		"relay":          relay,
		"admin_password": strings.TrimSpace(Read(ServicePass)),
		"admin_enabled":  true,
		"admin_captcha":  !captchaDisabled,
		"limits": map[string]any{
			"max_messages":   parseNum(ParseKV(text, "MaxMsgXuser"), 256, 1),
			"max_message_mb": parseNum(ParseKV(text, "MaxMsgSize"), 256, 1),
			"box_mb":         4096,
			"max_ml":         parseNum(ParseKV(text, "MaxMailingListSize"), 4096, 1),
			"max_rcpt":       parseNum(ParseKV(text, "MultiDeliverMaxRCPTTo"), 24, 1),
			"max_spam":       parseNum(ParseKV(text, "MaxSpamEntryXUser"), 196, 1),
			"debug":          debug,
			"autodelete":     autodel,
			"exit":           srvtype == "exit",
			"logvoucher":     logv,
		},
		"friends": []any(stringsToAny(friends)),
		"registration": map[string]any{
			"captcha":           !captchaDisabled,
			"voucher_required":  false,
			"exit_name_enabled": true,
			"exit_enabled":      true,
			"exit_name":         "",
		},
		"design": map[string]any{"theme": "light", "logo": logo, "small_logo": DefaultSmallLogo},
		"tor": map[string]any{
			"mode":          map[bool]string{true: "external", false: "internal"}[hostTor == "1"],
			"ip":            tip,
			"port":          tportN,
			"internal_ip":   "127.0.0.1",
			"internal_port": 19050,
			"bridges":       false,
			"bridge_lines":  "",
		},
		"vouchers": []any{},
	}
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func merge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		bm, bok := v.(map[string]any)
		om, ook := out[k].(map[string]any)
		if bok && ook {
			out[k] = merge(om, bm)
		} else {
			out[k] = v
		}
	}
	return out
}

func NormalizeState(st, base State) State {
	result := merge(base, st)
	design, ok := result["design"].(map[string]any)
	if !ok {
		design = map[string]any{}
	}
	theme, _ := design["theme"].(string)
	if theme != "light" && theme != "dark" {
		design["theme"] = "light"
	}
	logo, _ := design["logo"].(string)
	if strings.TrimSpace(logo) == "" || regexp.MustCompile(`\s`).MatchString(logo) {
		design["logo"] = DefaultLogo
	}
	small, _ := design["small_logo"].(string)
	small = strings.TrimSpace(small)
	if small != "" && regexp.MustCompile(`\s`).MatchString(small) {
		small = DefaultSmallLogo
	}
	design["small_logo"] = small
	result["design"] = design
	return result
}

func LoadState() State {
	base := DeriveState()
	if st := StateLoad(); st != nil {
		base = NormalizeState(st, base)
	}
	return base
}

func SetKeyFlat(text, key, value string) string {
	pat := regexp.MustCompile(`(?im)^(\s*` + regexp.QuoteMeta(key) + `\s+).*$`)
	if pat.MatchString(text) {
		return pat.ReplaceAllString(text, "${1}"+value)
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + key + "\t" + value + "\n"
}

func SetKeyServer(text, key, value string) string {
	pat := regexp.MustCompile(`(?im)^(\s*` + regexp.QuoteMeta(key) + `\s+).*$`)
	if pat.MatchString(text) {
		return pat.ReplaceAllString(text, "${1}"+value)
	}
	lines := strings.Split(text, "\n")
	idx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "}") {
			idx = i
			break
		}
	}
	ins := "\t" + key + "\t" + value
	if idx < 0 {
		lines = append(lines, ins)
	} else {
		lines = append(lines[:idx], append([]string{ins}, lines[idx:]...)...)
	}
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(text, "\n") {
		out += "\n"
	}
	return out
}

func SetNickPaths(text, old, new string) string {
	text = strings.ReplaceAll(text, old, new)
	re := regexp.MustCompile(`(?m)^(\s*SMTPServer\s+)\S+`)
	return re.ReplaceAllString(text, "${1}"+new)
}

func MigrateNick(st State) {
	newNick := strings.TrimSpace(Str(st, "nick"))
	if newNick == "" {
		return
	}
	newconf := filepath.Join(ServersDir, "1-"+newNick+".conf")
	if _, err := os.Stat(newconf); err == nil {
		return
	}
	files, _ := filepath.Glob(filepath.Join(ServersDir, "1-*.conf"))
	var oldconf string
	for _, p := range files {
		if p != newconf {
			oldconf = p
			break
		}
	}
	if oldconf == "" {
		return
	}
	old := ParseServer(oldconf).Nick
	if old == "" || old == newNick {
		return
	}
	if err := os.Rename(oldconf, newconf); err != nil {
		return
	}
	text := Read(newconf)
	text = SetNickPaths(text, old, newNick)
	_ = WriteKeep(newconf, text, nil)
	oldmail := "/var/onionmail/" + old
	newmail := "/var/onionmail/" + newNick
	if isDir(oldmail) && !pathExists(newmail) {
		_ = os.Rename(oldmail, newmail)
	}
	oldhs := "/var/lib/onionmail/HiddenServiceDir-1-" + old
	newhs := "/var/lib/onionmail/HiddenServiceDir-1-" + newNick
	if isDir(oldhs) && !pathExists(newhs) {
		_ = os.Rename(oldhs, newhs)
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func CaptchaDisableValue(st State) string {
	reg := Bool(Obj(st, "registration"), "captcha", true)
	adm := Bool(st, "admin_captcha", true)
	if reg || adm {
		return "no"
	}
	return "yes"
}

func ApplyState(st State, doMigrate bool) string {
	if doMigrate {
		MigrateNick(st)
	}
	conf := FindServerConf(Str(st, "nick"))
	if conf == "" {
		conf = FindServerConf("")
	}
	lim := Obj(st, "limits")
	nick := StrDef(st, "nick", "onionmail")
	if conf != "" {
		text := Read(conf)
		text = SetKeyServer(text, "Onion", Str(st, "onion"))
		text = SetKeyServer(text, "MaxMsgXuser", strconv.Itoa(Int(lim, "max_messages", 256)))
		text = SetKeyServer(text, "MaxMsgSize", strconv.Itoa(Int(lim, "max_message_mb", 256))+"M")
		text = SetKeyServer(text, "MaxMailingListSize", strconv.Itoa(Int(lim, "max_ml", 4096)))
		text = SetKeyServer(text, "MultiDeliverMaxRCPTTo", strconv.Itoa(Int(lim, "max_rcpt", 24)))
		text = SetKeyServer(text, "MaxSpamEntryXUser", strconv.Itoa(Int(lim, "max_spam", 196)))
		text = SetKeyServer(text, "CanRelay", onOff(Bool(st, "relay", false), "yes", "no"))
		text = SetKeyServer(text, "ServerType", map[bool]string{true: "Exit", false: "normal"}[Bool(lim, "exit", false)])
		text = SetKeyServer(text, "CanPOP3Register", "no")
		vlog := "/var/log/onionmail/" + nick + "-vouchers.log"
		vlist := "/var/log/onionmail/" + nick + "-vouchers.lst"
		if Bool(lim, "logvoucher", false) {
			text = SetKeyServer(text, "LogVoucherTo", vlog)
		} else {
			text = regexp.MustCompile(`(?im)^(\s*)LogVoucherTo\s+.*$`).ReplaceAllString(text, "${1}#LogVoucherTo disabled")
		}
		text = SetKeyServer(text, "VoucherListTo", vlist)
		text = regexp.MustCompile(`(?m)^(\s*MailDir\s+).*$`).ReplaceAllString(text, "${1}/var/onionmail/"+nick)
		text = regexp.MustCompile(`(?m)^(\s*StatFile\s+).*$`).ReplaceAllString(text, "${1}/var/log/onionmail/"+nick+"-stats.csv")
		text = regexp.MustCompile(`(?m)^(\s*BinStatFile\s+).*$`).ReplaceAllString(text, "${1}/var/log/onionmail/"+nick+"-stats.dat")
		_ = WriteKeep(conf, text, nil)
	}
	core := Read(CoreConf)
	core = regexp.MustCompile(`(?im)^(\s*Debug\s+).*$`).ReplaceAllString(core, "${1}"+onOff(Bool(lim, "debug", false), "yes", "no"))
	_ = WriteKeep(CoreConf, core, nil)
	http := Read(HTTPConf)
	http = regexp.MustCompile(`(?im)^(\s*HTTPServerName\s+).*$`).ReplaceAllString(http, "${1}"+StrDef(st, "server_name", "example")+" onionmail server")
	_ = WriteKeep(HTTPConf, http, nil)
	pop := Read(POP3Conf)
	pop = regexp.MustCompile(`(?im)^(\s*AutoDeleteReaded\s+).*$`).ReplaceAllString(pop, "${1}"+onOff(Bool(lim, "autodelete", false), "yes", "no"))
	_ = WriteKeep(POP3Conf, pop, nil)
	cden := filepath.Join(WWWRoot, "config.denied.conf")
	if pathExists(cden) {
		c := Read(cden)
		c = regexp.MustCompile(`(?im)^(\s*captcha-disable\s+).*$`).ReplaceAllString(c, "${1}"+CaptchaDisableValue(st))
		logo := Str(Obj(st, "design"), "logo")
		if logo == "" {
			logo = DefaultLogo
		}
		if regexp.MustCompile(`\s`).MatchString(logo) {
			logo = DefaultLogo
		}
		c = SetKeyFlat(c, "mainlogo", logo)
		_ = WriteKeep(cden, c, nil)
	}
	if p := Str(st, "admin_password"); p != "" {
		m := os.FileMode(0o600)
		_ = WriteKeep(ServicePass, p, &m)
		text := ""
		if conf != "" {
			text = Read(conf)
		}
		if text != "" {
			text = SetKeyServer(text, "PassWD", p)
			_ = WriteKeep(conf, text, nil)
		}
	}
	if st["friends"] != nil {
		text := ""
		if conf != "" {
			text = Read(conf)
		}
		if text != "" {
			text = regexp.MustCompile(`(?is)\n\s*Friends\s*\{.*?\}\s*`).ReplaceAllString(text, "\n")
			_ = WriteKeep(conf, text, nil)
		}
		feedPath := "/etc/onionmail/feed.conf"
		feed := Read(feedPath)
		var b strings.Builder
		b.WriteString("Friends {\n")
		for _, f := range friendsList(st) {
			b.WriteString("\t" + f + "\n")
		}
		b.WriteString("\t}")
		block := b.String()
		if regexp.MustCompile(`(?is)\bFriends\s*\{.*?\}`).MatchString(feed) {
			feed = regexp.MustCompile(`(?is)\bFriends\s*\{.*?\}`).ReplaceAllString(feed, block)
		} else {
			feed = strings.TrimRight(feed, "\n") + "\n\n" + block + "\n"
		}
		_ = WriteKeep(feedPath, feed, nil)
	}
	envPath := "/etc/environment"
	envText := Read(envPath)
	envText = regexp.MustCompile(`(?m)^LANG=.*\n?`).ReplaceAllString(envText, "")
	envText = strings.TrimRight(envText, "\n") + "\nLANG=" + StrDef(st, "locale", "C.UTF-8") + "\n"
	_ = WriteKeep(envPath, envText, nil)
	oldDisplay := Str(st, "brand_name")
	newDisplay := Str(st, "server_name")
	if oldDisplay != "" && newDisplay != "" && oldDisplay != newDisplay {
		BrandPages(oldDisplay, newDisplay)
	}
	st["brand_name"] = newDisplay
	_ = StateSave(st)
	return conf
}

func friendsList(st State) []string {
	arr := Arr(st, "friends")
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func FriendsList(st State) []string { return friendsList(st) }

func BrandPages(old, newName string) {
	if old == "" || newName == "" || old == newName {
		return
	}
	_ = filepath.Walk(WWWRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		text := Read(path)
		if strings.Contains(text, old) {
			_ = WriteKeep(path, strings.ReplaceAll(text, old, newName), nil)
		}
		return nil
	})
}

func WriteTorrc(st State) string {
	tor := Obj(st, "tor")
	nick := StrDef(st, "nick", "onionmail")
	ip := StrDef(tor, "internal_ip", "127.0.0.1")
	port := Int(tor, "internal_port", 19050)
	hsdir := "/var/lib/onionmail/HiddenServiceDir-1-" + nick
	lines := []string{
		"User debian-tor",
		fmt.Sprintf("SocksPort %s:%d", ip, port),
		"DataDirectory /var/lib/tor",
		"Log notice file " + TorLog,
		"HiddenServiceDir " + hsdir,
		"HiddenServiceVersion 3",
		"HiddenServicePort 25 127.0.0.1:20025",
		"HiddenServicePort 110 127.0.0.1:20110",
		"HiddenServicePort 80 127.0.0.1:18000",
	}
	var bridges []string
	for _, x := range strings.Split(strings.ReplaceAll(Str(tor, "bridge_lines"), ",", "\n"), "\n") {
		if x = strings.TrimSpace(x); x != "" {
			bridges = append(bridges, x)
		}
	}
	if Bool(tor, "bridges", false) && len(bridges) > 0 {
		lines = append(lines, "UseBridges 1")
		hasObfs := false
		for _, b := range bridges {
			if strings.Contains(b, "obfs4") {
				hasObfs = true
			}
		}
		if hasObfs {
			lines = append(lines, "ClientTransportPlugin obfs4 exec /usr/bin/obfs4proxy")
		}
		for _, b := range bridges {
			if strings.HasPrefix(strings.ToLower(b), "bridge ") {
				lines = append(lines, b)
			} else {
				lines = append(lines, "Bridge "+b)
			}
		}
	}
	mode := os.FileMode(0o644)
	_ = WriteKeep(Torrc, strings.Join(lines, "\n")+"\n", &mode)
	_ = os.MkdirAll("/var/lib/tor", 0o755)
	_ = os.MkdirAll("/run/tor", 0o755)
	_ = os.MkdirAll(hsdir, 0o755)
	for _, d := range []string{"/var/lib/tor", "/run/tor", hsdir} {
		chownTor(d)
		_ = os.Chmod(d, 0o700)
	}
	logdir := filepath.Dir(TorLog)
	_ = os.MkdirAll(logdir, 0o755)
	chownTorGroup(logdir)
	_ = os.Chmod(logdir, 0o750)
	if f, err := os.OpenFile(TorLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640); err == nil {
		f.Close()
		chownTor(TorLog)
		_ = os.Chmod(TorLog, 0o640)
	}
	return hsdir
}

func chownTor(path string) {
	if u, err := user.Lookup("debian-tor"); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown(path, uid, gid)
	}
}

func chownTorGroup(path string) {
	if g, err := user.LookupGroup("debian-tor"); err == nil {
		gid, _ := strconv.Atoi(g.Gid)
		_ = os.Chown(path, -1, gid)
	}
}

func Run(cmd string, timeout time.Duration) (int, string) {
	return RunArgs([]string{"bash", "-c", cmd}, timeout)
}

func RunArgs(args []string, timeout time.Duration) (int, string) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := c.CombinedOutput()
	rc := 0
	if err != nil {
		rc = 1
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		}
	}
	return rc, string(out)
}

func TorPIDs() []int {
	rc, out := Run("pgrep -x tor", 20*time.Second)
	if rc != 0 {
		return nil
	}
	var pids []int
	for _, f := range strings.Fields(out) {
		if n, err := strconv.Atoi(f); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

func StopTor() {
	for _, pid := range TorPIDs() {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	time.Sleep(500 * time.Millisecond)
	for _, pid := range TorPIDs() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	_, _ = Run("service tor stop", 20*time.Second)
}

func StartTor(st State) bool {
	WriteTorrc(st)
	StopTor()
	_, _ = Run("service tor start", 20*time.Second)
	time.Sleep(time.Second)
	if len(TorPIDs()) == 0 {
		_ = os.MkdirAll(filepath.Dir(TorLog), 0o755)
		if logf, err := os.OpenFile(TorLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640); err == nil {
			cmd := exec.Command("tor", "-f", Torrc)
			cmd.Stdout = logf
			cmd.Stderr = logf
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			_ = cmd.Start()
			logf.Close()
			time.Sleep(time.Second)
		}
	}
	return len(TorPIDs()) > 0
}

func TorProgress() string {
	text := Read(TorLog)
	pcts := regexp.MustCompile(`Bootstrapped\s+(\d+)%`).FindAllStringSubmatch(text, -1)
	if len(pcts) > 0 {
		return pcts[len(pcts)-1][1] + "%"
	}
	if len(TorPIDs()) > 0 {
		return "starting"
	}
	if text != "" {
		lines := strings.Split(text, "\n")
		var nonempty []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				nonempty = append(nonempty, l)
			}
		}
		if len(nonempty) > 0 {
			last := nonempty[len(nonempty)-1]
			if len(last) > 80 {
				last = last[len(last)-80:]
			}
			return last
		}
	}
	return "inactive"
}

func TorSocketOK(ip string, port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func GetOnion(st State) string {
	paths := []string{
		"/var/lib/onionmail/HiddenServiceDir-1-" + StrDef(st, "nick", "onionmail") + "/hostname",
		"/var/tor/onionmail/hostname",
	}
	re := regexp.MustCompile(`^[a-z2-7]{16,56}\.onion$`)
	for _, p := range paths {
		v := strings.TrimSpace(Read(p))
		if re.MatchString(v) {
			return v
		}
	}
	if conf := FindServerConf(""); conf != "" {
		if v := ParseServer(conf).Onion; v != "" {
			return v
		}
	}
	return ""
}

func ControlCommand(cmd string, timeout time.Duration) (string, error) {
	st := LoadState()
	return ControlAs(StrDef(st, "nick", "onionmail"), Read(ServicePass), 9010, cmd, timeout)
}

func ControlAs(nick, pass string, port int, cmd string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if strings.ContainsAny(cmd, "\r\n\x00") {
		return "", fmt.Errorf("invalid control command")
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	r := bufio.NewReader(c)
	readline := func() (string, error) {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	if _, err := readline(); err != nil {
		return "", err
	}
	if _, err := io.WriteString(c, "server "+nick+" "+pass+"\n"); err != nil {
		return "", err
	}
	auth, err := readline()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(auth, "+OK") {
		return "", fmt.Errorf("control authentication failed: %s", auth)
	}
	if _, err := io.WriteString(c, cmd+"\n"); err != nil {
		return "", err
	}
	line, err := readline()
	if err != nil {
		return "", err
	}
	_, _ = io.WriteString(c, "quit\n")
	return line, nil
}

func Validate(st State) []string {
	var errs []string
	nick := Str(st, "nick")
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`).MatchString(nick) {
		errs = append(errs, "NICK: 1-31 chars, lowercase a-z, digits, _ or -")
	}
	name := strings.TrimSpace(Str(st, "server_name"))
	if name == "" || len(name) > 48 || strings.ContainsAny(name, "\r\n") {
		errs = append(errs, "server name: 1-48 chars, single line")
	}
	onion := strings.TrimSpace(Str(st, "onion"))
	if !regexp.MustCompile(`^(?:[a-z2-7]{16}|[a-z2-7]{56})\.onion$`).MatchString(onion) {
		errs = append(errs, "onion address: 16 or 56 base32 chars + .onion")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,40}$`).MatchString(Str(st, "locale")) {
		errs = append(errs, "LANG: invalid")
	}
	if len(Str(st, "admin_password")) < 8 {
		errs = append(errs, "admin password: at least 8 chars")
	}
	lim := Obj(st, "limits")
	ranges := [][3]any{
		{"max_messages", 1, 1000000},
		{"max_message_mb", 1, 2048},
		{"box_mb", 16, 1048576},
		{"max_ml", 1, 1000000},
		{"max_rcpt", 1, 10000},
		{"max_spam", 1, 1000000},
	}
	for _, r := range ranges {
		k := r[0].(string)
		lo, hi := r[1].(int), r[2].(int)
		v, ok := lim[k]
		if !ok {
			errs = append(errs, k+": number required")
			continue
		}
		n := IntVal(v, -1)
		if n < 0 {
			errs = append(errs, k+": number required")
		} else if n < lo || n > hi {
			errs = append(errs, fmt.Sprintf("%s: range %d..%d", k, lo, hi))
		}
	}
	for _, f := range FriendsList(st) {
		if !regexp.MustCompile(`^[a-z2-7]{16,56}\.onion$`).MatchString(f) {
			errs = append(errs, "friend: invalid onion "+f)
		}
	}
	reg := Obj(st, "registration")
	en := strings.TrimSpace(Str(reg, "exit_name"))
	if Bool(reg, "exit_name_enabled", true) && en != "" && !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,30}[a-z0-9]$`).MatchString(en) {
		errs = append(errs, "exit name: 4-32 chars latin/digits/._-")
	}
	tor := Obj(st, "tor")
	mode := Str(tor, "mode")
	if mode != "external" && mode != "internal" && mode != "off" {
		errs = append(errs, "tor mode: invalid")
	}
	for _, key := range []string{"ip", "internal_ip"} {
		v := Str(tor, key)
		if v != "" && !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(v) {
			errs = append(errs, "tor address: invalid")
		}
	}
	for _, key := range []string{"port", "internal_port"} {
		v, ok := tor[key]
		if !ok {
			errs = append(errs, "tor port: number required")
			continue
		}
		n := IntVal(v, -1)
		if n < 1 || n > 65535 {
			errs = append(errs, "tor port: 1..65535")
		}
	}
	logo := Str(Obj(st, "design"), "logo")
	if logo == "" || regexp.MustCompile(`\s`).MatchString(logo) {
		errs = append(errs, "logo: invalid path")
	}
	small := Str(Obj(st, "design"), "small_logo")
	if small != "" && regexp.MustCompile(`\s`).MatchString(small) {
		errs = append(errs, "small logo: invalid path")
	}
	return errs
}

func TorConfigErrors(st State) []string {
	tor := Obj(st, "tor")
	if Str(tor, "mode") != "internal" {
		return nil
	}
	var cfgErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				cfgErr = fmt.Errorf("%v", r)
			}
		}()
		WriteTorrc(st)
	}()
	if cfgErr != nil {
		return []string{"tor config: " + cfgErr.Error()}
	}
	if _, err := os.Stat("/usr/bin/tor"); err != nil {
		return nil
	}
	rc, out := RunArgs([]string{"tor", "--verify-config", "-f", Torrc}, 20*time.Second)
	if rc == 0 {
		return nil
	}
	lines := []string{}
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	var warns, errs []string
	for _, l := range lines {
		if strings.Contains(l, "[warn]") {
			warns = append(warns, l)
		}
		if strings.Contains(l, "[err]") {
			errs = append(errs, l)
		}
	}
	msg := "invalid"
	if len(warns) > 0 {
		msg = warns[len(warns)-1]
	} else if len(errs) > 0 {
		msg = errs[len(errs)-1]
	} else if len(lines) > 0 {
		msg = lines[len(lines)-1]
	}
	return []string{"tor config: " + msg}
}

func Save(st State, section string) (bool, []string) {
	if errs := Validate(st); len(errs) > 0 {
		return false, errs
	}
	ApplyState(st, false)
	if section == "tor" {
		if Str(Obj(st, "tor"), "mode") == "internal" {
			if !StartTor(st) {
				return false, []string{"Tor did not start; see Tor log"}
			}
		} else {
			StopTor()
		}
	}
	return true, nil
}

func BootEnv() {
	st := LoadState()
	MigrateNick(st)
	func() {
		defer func() { _ = recover() }()
		ApplyState(st, false)
	}()
	tor := Obj(st, "tor")
	mode := StrDef(tor, "mode", "external")
	var tip string
	var tp int
	switch mode {
	case "internal":
		WriteTorrc(st)
		tip = StrDef(tor, "internal_ip", "127.0.0.1")
		tp = Int(tor, "internal_port", 19050)
	case "external":
		tip = StrDef(tor, "ip", "127.0.0.1")
		tp = Int(tor, "port", 9050)
	default:
		tip, tp = "127.0.0.1", 1
	}
	useHost := "0"
	if mode == "external" {
		useHost = "1"
	}
	vals := [][2]string{
		{"NICK", StrDef(st, "nick", "onionmail")},
		{"DISPLAY_NAME", StrDef(st, "server_name", "example")},
		{"ONION", Str(st, "onion")},
		{"LOCALE", StrDef(st, "locale", "C.UTF-8")},
		{"TOR_MODE", mode},
		{"TOR_IP", tip},
		{"TOR_PORT", strconv.Itoa(tp)},
		{"USE_HOST_TOR", useHost},
		{"CAPTCHA_DISABLE", CaptchaDisableValue(st)},
		{"DEFAULT_THEME", StrDef(Obj(st, "design"), "theme", "light")},
	}
	for _, kv := range vals {
		fmt.Printf("%s=%s\n", kv[0], ShellQuote(kv[1]))
	}
}

func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '@' || r == '+' || r == '=' || r == ',' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func InstallLogo(st State, path string, tr func(string) string) (bool, string) {
	return InstallBigLogo(st, path, tr)
}

func InstallBigLogo(st State, path string, tr func(string) string) (bool, string) {
	return installLogo(st, path, "om-custom-logo", "logo", DefaultLogo, tr)
}

func InstallSmallLogo(st State, path string, tr func(string) string) (bool, string) {
	return installLogo(st, path, "om-custom-small-logo", "small_logo", DefaultSmallLogo, tr)
}

func installLogo(st State, path, prefix, key, def string, tr func(string) string) (bool, string) {
	src := strings.TrimSpace(path)
	src = strings.Trim(src, `"'`)
	if strings.HasPrefix(src, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			src = filepath.Join(home, src[1:])
		}
	}
	abs, _ := filepath.Abs(src)
	src = abs
	fi, err := os.Stat(src)
	if err != nil || fi.IsDir() {
		return false, tr("Image not found") + ": " + src
	}
	ext := strings.ToLower(filepath.Ext(src))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return false, tr("Unsupported image format (PNG/JPG only).")
	}
	if fi.Size() <= 0 {
		return false, tr("Image not found") + ": " + src
	}
	if fi.Size() > LogoMaxBytes {
		return false, tr("Image is too large (max 5 MB).")
	}
	f, err := os.Open(src)
	if err != nil {
		return false, tr("Image not found") + ": " + src
	}
	head := make([]byte, 8)
	_, _ = io.ReadFull(f, head)
	f.Close()
	if ext == ".png" && !strings.HasPrefix(string(head), "\x89PNG\r\n\x1a\n") {
		return false, tr("Unsupported image format (PNG/JPG only).")
	}
	if (ext == ".jpg" || ext == ".jpeg") && !(head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff) {
		return false, tr("Unsupported image format (PNG/JPG only).")
	}
	imgdir := filepath.Join(WWWRoot, "img")
	dst := filepath.Join(imgdir, prefix+ext)
	if err := os.MkdirAll(imgdir, 0o755); err != nil {
		return false, err.Error()
	}
	if src != dst {
		tmp := dst + ".omtmp"
		data, err := os.ReadFile(src)
		if err != nil {
			return false, err.Error()
		}
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return false, err.Error()
		}
		if u, err := user.Lookup("debian-onionmail"); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			_ = os.Chown(tmp, uid, gid)
		}
		if err := os.Rename(tmp, dst); err != nil {
			return false, err.Error()
		}
	}
	matches, _ := filepath.Glob(filepath.Join(imgdir, prefix+".*"))
	for _, old := range matches {
		if old != dst {
			_ = os.Remove(old)
		}
	}
	if _, ok := st["design"]; !ok || st["design"] == nil {
		st["design"] = map[string]any{}
	}
	Obj(st, "design")[key] = "/img/" + filepath.Base(dst)
	if err := ApplySafe(st); err != nil {
		return false, err.Error()
	}
	return true, tr("Logo updated.")
}

func ResetLogo(st State, tr func(string) string) (bool, string) {
	return ResetBigLogo(st, tr)
}

func ResetBigLogo(st State, tr func(string) string) (bool, string) {
	return resetLogo(st, "om-custom-logo", "logo", DefaultLogo, tr)
}

func ResetSmallLogo(st State, tr func(string) string) (bool, string) {
	return resetLogo(st, "om-custom-small-logo", "small_logo", DefaultSmallLogo, tr)
}

func resetLogo(st State, prefix, key, def string, tr func(string) string) (bool, string) {
	matches, _ := filepath.Glob(filepath.Join(WWWRoot, "img", prefix+".*"))
	for _, old := range matches {
		_ = os.Remove(old)
	}
	if def == DefaultLogo {
		defaultFile := filepath.Join(WWWRoot, "img", "OM-BL.gif")
		seedFile := "/opt/seed/var-lib-onionmail/wwwroot/img/OM-BL.gif"
		if !pathExists(defaultFile) && pathExists(seedFile) {
			if data, err := os.ReadFile(seedFile); err == nil {
				_ = os.WriteFile(defaultFile, data, 0o644)
			}
		}
	}
	if _, ok := st["design"]; !ok || st["design"] == nil {
		st["design"] = map[string]any{}
	}
	Obj(st, "design")[key] = def
	if err := ApplySafe(st); err != nil {
		return false, err.Error()
	}
	return true, tr("Logo reset.")
}

func OnionmailPIDs() []int {
	var pids []int
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(b), "\x00", " ")
		if strings.Contains(cmd, "java") && strings.Contains(cmd, "onionmail -M:server") {
			pids = append(pids, n)
		}
	}
	return pids
}

func Restart() int {
	pids := OnionmailPIDs()
	if len(pids) == 0 {
		fmt.Println("OnionMail server is not running; restarting container")
		if err := syscall.Kill(1, syscall.SIGTERM); err != nil {
			fmt.Fprintln(os.Stderr, "restart failed:", err)
			return 1
		}
		return 0
	}
	fmt.Println("OnionMail restart requested")
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	return 0
}

func ApplySafe(st State) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	ApplyState(st, false)
	return nil
}
