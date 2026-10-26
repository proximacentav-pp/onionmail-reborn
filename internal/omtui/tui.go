package omtui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"onionmail/internal/olang"
	"onionmail/internal/omcore"
)

const (
	ansiFg      = "\x1b[38;5;183m"
	ansiAcc     = "\x1b[38;5;141m"
	ansiAccBold = "\x1b[38;5;141;1m"
	ansiSel     = "\x1b[38;5;135;1m"
	ansiOK      = "\x1b[38;5;141m"
	ansiErr     = "\x1b[38;5;203;1m"
	ansiMut     = "\x1b[38;5;97m"
)

type item struct{ label, kind, key string }

type UI struct {
	st     omcore.State
	lang   string
	b      *olang.Bundle
	cat    int
	sel    int
	scroll int
	msg    string
	msgOK  bool
	cats   []string
	rows   int
	cols   int
}

func Run(st omcore.State, b *olang.Bundle) int {
	lang := omcore.StrDef(st, "lang", "ru")
	if !b.HasLang(lang) {
		lang = "en"
	}
	u := &UI{st: st, lang: lang, b: b, msgOK: true,
		cats: []string{"1. Main", "2. Admin access", "3. Limits", "4. Server friends", "5. Registration", "6. Tor", "7. Design"}}
	if !u.setupTerm() {
		fmt.Fprintln(os.Stderr, "om setup: cannot configure the terminal")
		return 1
	}
	defer u.restoreTerm()
	u.languageScreen()
	for {
		u.termSize()
		u.draw()
		k := u.readKey()
		switch k {
		case "ctrl-c":
			r := u.choose(u.t("Exit with saving?"), []string{"Save and exit", "Exit without saving", "Cancel"}, 0)
			if r == 0 {
				ok, errs := omcore.Save(u.st, u.section())
				if ok {
					return 0
				}
				u.setMsg(u.t("Validation error")+": "+strings.Join(first(errs, 3), "; "), false)
			} else if r == 1 {
				return 0
			}
		case "ctrl-s":
			ok, errs := omcore.Save(u.st, u.section())
			if ok {
				u.setMsg(u.t("Saved."), true)
			} else {
				u.setMsg(u.t("Validation error")+": "+strings.Join(first(errs, 3), "; "), false)
			}
		case "left":
			u.cat = (u.cat - 1 + len(u.cats)) % len(u.cats)
			u.sel, u.scroll = 0, 0
		case "right":
			u.cat = (u.cat + 1) % len(u.cats)
			u.sel, u.scroll = 0, 0
		case "up":
			if u.sel > 0 {
				u.sel--
			}
		case "down":
			if n := len(u.items()); u.sel < n-1 {
				u.sel++
			}
		case "tab":
			if n := len(u.items()); n > 0 {
				u.sel = (u.sel + 1) % n
			}
		case "enter":
			items := u.items()
			if len(items) > 0 {
				if u.sel >= len(items) {
					u.sel = len(items) - 1
				}
				u.handle(items[u.sel])
			}
		}
	}
}

func first(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}

func (u *UI) section() string {
	if u.cat == 5 {
		return "tor"
	}
	return ""
}
func (u *UI) t(key string) string      { return u.b.OmText(u.lang, key) }
func (u *UI) setMsg(s string, ok bool) { u.msg, u.msgOK = s, ok }

func (u *UI) setupTerm() bool {
	c := exec.Command("stty", "raw", "-echo")
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return false
	}
	fmt.Print("\x1b[?25l")
	return true
}
func (u *UI) restoreTerm() {
	fmt.Print("\x1b[?25h\x1b[0m\n")
	c := exec.Command("stty", "sane")
	c.Stdin = os.Stdin
	_ = c.Run()
}
func (u *UI) termSize() {
	type winsize struct{ Row, Col, Xpixel, Ypixel uint16 }
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno == 0 && ws.Row > 0 {
		u.rows, u.cols = int(ws.Row), int(ws.Col)
	}
	if u.rows <= 0 || u.cols <= 0 {
		u.rows, u.cols = 24, 80
	}
}
func (u *UI) readByte() byte {
	var b [1]byte
	for {
		n, err := os.Stdin.Read(b[:])
		if err == nil && n == 1 {
			return b[0]
		}
		if err != nil {
			return 0
		}
	}
}
func (u *UI) readKey() string {
	b := u.readByte()
	switch b {
	case 3:
		return "ctrl-c"
	case 19:
		return "ctrl-s"
	case 9:
		return "tab"
	case 10, 13:
		return "enter"
	case 127, 8:
		return "backspace"
	case 27:
		b2 := u.readByte()
		if b2 != '[' && b2 != 'O' {
			return "esc"
		}
		b3 := u.readByte()
		switch b3 {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case 'C':
			return "right"
		case 'D':
			return "left"
		case 'H':
			return "home"
		case 'F':
			return "end"
		case '3':
			if u.readByte() == '~' {
				return "delete"
			}
		case '5':
			if u.readByte() == '~' {
				return "pgup"
			}
		case '6':
			if u.readByte() == '~' {
				return "pgdn"
			}
		}
		return "esc"
	}
	if b >= 32 {
		buf := []byte{b}
		if b >= 0xC0 {
			n := 1
			if b >= 0xF0 {
				n = 3
			} else if b >= 0xE0 {
				n = 2
			}
			for i := 0; i < n; i++ {
				buf = append(buf, u.readByte())
			}
		}
		return "text:" + string(buf)
	}
	return ""
}
func runeLen(s string) int { return len([]rune(s)) }
func trunc(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func truncANSI(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	count := 0
	cut := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < '@' || s[j] > '~') {
					j++
				}
				if j < len(s) {
					j++
				}
			} else if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		if count >= n {
			cut = true
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		count++
		i += size
	}
	if cut {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}
func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}
func (u *UI) paint(row, col int, text string) {
	if row < 1 || row > u.rows || col < 1 || col > u.cols {
		return
	}
	fmt.Printf("\x1b[%d;%dH%s%s", row, col, ansiFg, truncANSI(text, u.cols-col+1))
}
func (u *UI) clear() { fmt.Print("\x1b[2J") }

func (u *UI) draw() {
	u.termSize()
	u.clear()
	title := u.t("OnionMail Control")
	u.paint(1, 1, ansiAccBold+pad("  "+title, u.cols))
	nick := omcore.Str(u.st, "nick")
	full := nick + "@" + omcore.Str(u.st, "onion")
	info := full
	if runeLen(full)+runeLen(title)+8 > u.cols {
		info = nick
	}
	if info != "" {
		x := u.cols - runeLen(info) - 1
		if x >= runeLen(title)+4 {
			u.paint(1, x, ansiAcc+info)
		}
	}
	narrow := u.cols < 120
	if narrow {
		groups := [][]int{{0, 1, 2}, {3, 4, 5}, {6}}
		for row, group := range groups {
			x := 2
			for _, i := range group {
				label := u.t(u.cats[i])
				text := label
				if i == u.cat {
					text = ansiSel + "<" + label + ">" + "\x1b[0m"
				}
				u.paint(2+row, x, text)
				x += runeLen(text) + 2
			}
		}
		u.itemsArea(2+len(groups)+1, 2, u.rows-4, true)
	} else {
		left := u.cols / 3
		if left < 24 {
			left = 24
		}
		if left > 34 {
			left = 34
		}
		u.paint(2, 1, ansiAccBold+"MENU"+strings.Repeat(" ", max(0, left-6)))
		for i, name := range u.cats {
			label := u.t(name)
			text := " " + label
			if i == u.cat {
				text = ansiSel + "<" + label + ">" + "\x1b[0m"
			}
			u.paint(3+i, 1, pad(text, left-2))
		}
		u.itemsArea(3, left+1, u.rows-5, false)
	}
	fh := u.rows - 1
	u.paint(fh, 1, ansiAcc+pad("  ↑↓/Tab  Enter  Ctrl+S "+u.t("Save")+"  Ctrl+C "+u.t("Cancel"), u.cols))
	if u.msg != "" {
		if u.msgOK {
			u.paint(u.rows, 2, ansiOK+trunc(u.msg, u.cols-2))
		} else {
			u.paint(u.rows, 2, ansiErr+trunc(u.msg, u.cols-2))
		}
	}
	fmt.Print("\x1b[0m")
}

func (u *UI) itemsArea(y, x, height int, narrow bool) {
	items := u.items()
	vis := height
	if vis < 1 {
		vis = 1
	}
	if u.sel < u.scroll {
		u.scroll = u.sel
	}
	if u.sel >= u.scroll+vis {
		u.scroll = u.sel - vis + 1
	}
	for idx := u.scroll; idx < len(items) && idx < u.scroll+vis; idx++ {
		it := items[idx]
		y2 := y + idx - u.scroll
		selected := idx == u.sel
		var line string
		if it.kind == "action" {
			if selected {
				line = "<" + u.t(it.label) + ">"
			} else {
				line = "[ " + u.t(it.label) + " ]"
			}
		} else {
			label := u.t(it.label)
			if selected {
				label = "<" + label + ">"
			}
			val := u.valueFor(it)
			if narrow && it.kind == "friend" {
				val = ""
			}
			width := u.cols - x
			avail := width - runeLen(val) - 2
			if avail < runeLen(label) {
				avail = runeLen(label)
			}
			if avail > 34 {
				avail = 34
			}
			line = " " + pad(label, avail) + "  " + val
		}
		if selected {
			u.paint(y2, x, ansiSel+pad(line, u.cols-x+1)+"\x1b[0m")
		} else {
			u.paint(y2, x, line)
		}
	}
}

func (u *UI) items() []item {
	st := u.st
	switch u.cat {
	case 0:
		return []item{{"Server name", "text", "server_name"}, {"Onion address", "text", "onion"}, {"Get from daemon", "action", "get_onion"}, {"Hidden name (NICK)", "text", "nick"}, {"Encoding (LANG)", "text", "locale"}, {"Relay mode", "bool", "relay"}, {"Save", "action", "save"}}
	case 1:
		return []item{{"Admin login", "text", "admin_login"}, {"Admin password", "pass", "admin_password"}, {"Admin panel", "bool", "admin_enabled"}, {"Admin CAPTCHA", "bool", "admin_captcha"}, {"Save", "action", "save"}}
	case 2:
		return []item{{"Max messages in inbox", "int", "max_messages"}, {"Max message size (MB)", "int", "max_message_mb"}, {"Mailbox size (MB)", "int", "box_mb"}, {"Max mailing list size", "int", "max_ml"}, {"Max multi delivery addresses", "int", "max_rcpt"}, {"Max spam list size", "int", "max_spam"}, {"Debug mode", "bool", "debug"}, {"Delete read messages", "bool", "autodelete"}, {"EXIT/ENTER server", "bool", "exit"}, {"Log voucher codes", "bool", "logvoucher"}, {"Save", "action", "save"}}
	case 3:
		out := []item{{"Add friend", "action", "add_friend"}}
		for _, f := range omcore.FriendsList(st) {
			out = append(out, item{u.t("Friend") + " " + f, "friend", f})
		}
		return append(out, item{"Save", "action", "save"})
	case 4:
		return []item{{"Registration CAPTCHA", "bool", "reg_captcha"}, {"Require vouchers", "bool", "voucher_required"}, {"Voucher manager", "action", "vouchers"}, {"Exit name field", "bool", "exit_name_enabled"}, {"Exit name", "text", "exit_name"}, {"Exit connection", "bool", "exit_enabled"}, {"Save", "action", "save"}}
	case 6:
		return []item{
			{"Default theme", "choice", "default_theme"},
			{"Big logo", "logo", "big_logo_status"},
			{"Install big logo", "action", "add_big_logo"},
			{"Small logo", "logo", "small_logo_status"},
			{"Install small logo", "action", "add_small_logo"},
			{"Save", "action", "save"},
		}
	}
	tor := omcore.Obj(st, "tor")
	out := []item{{"Tor mode", "choice", "tor_mode"}}
	switch omcore.Str(tor, "mode") {
	case "external":
		out = append(out, item{"External address", "text", "tor_ip"}, item{"External address", "int", "tor_port"})
	case "internal":
		out = append(out, item{"Internal address", "text", "tor_internal_ip"}, item{"Internal address", "int", "tor_internal_port"}, item{"Use bridges", "bool", "tor_bridges"}, item{"Bridges", "text", "tor_bridge_lines"})
	}
	return append(out, item{"Tor status", "info", "tor_status"}, item{"Tor log", "action", "tor_log"}, item{"Restart Tor", "action", "tor_restart"}, item{"Save", "action", "save"})
}

func (u *UI) valueFor(it item) string {
	st := u.st
	switch it.kind {
	case "action":
		return ""
	case "logo":
		key := "logo"
		if it.key == "small_logo_status" {
			key = "small_logo"
		}
		logo := omcore.Str(omcore.Obj(st, "design"), key)
		if logo == "" || logo == omcore.DefaultLogo {
			return u.t("Default")
		}
		return u.t("Custom") + ": " + baseName(logo)
	case "info":
		tor := omcore.Obj(st, "tor")
		switch omcore.Str(tor, "mode") {
		case "internal":
			return fmt.Sprintf("%s  %s:%d  %s", u.t("Internal Tor"), omcore.Str(tor, "internal_ip"), omcore.Int(tor, "internal_port", 19050), omcore.TorProgress())
		case "external":
			ip, port := omcore.StrDef(tor, "ip", "127.0.0.1"), omcore.Int(tor, "port", 9050)
			ok := "unreachable"
			if omcore.TorSocketOK(ip, port) {
				ok = "OK"
			}
			return fmt.Sprintf("%s  %s:%d  %s", u.t("External Tor"), ip, port, ok)
		}
		return u.t("Disabled")
	}
	key := it.key
	switch key {
	case "admin_login":
		return omcore.Str(st, "nick")
	case "server_name", "onion", "nick", "locale":
		return omcore.Str(st, key)
	case "admin_password":
		return strings.Repeat("*", min(20, len(omcore.Str(st, key))))
	case "relay", "admin_enabled", "admin_captcha":
		return u.onOff(omcore.Bool(st, key, false))
	}
	if strings.HasPrefix(key, "reg_") {
		return u.onOff(omcore.Bool(omcore.Obj(st, "registration"), key[4:], false))
	}
	switch key {
	case "voucher_required", "exit_name_enabled", "exit_enabled":
		return u.onOff(omcore.Bool(omcore.Obj(st, "registration"), key, false))
	case "exit_name":
		v := omcore.Str(omcore.Obj(st, "registration"), "exit_name")
		if v == "" {
			return "-"
		}
		return v
	case "debug", "autodelete", "exit", "logvoucher":
		return u.onOff(omcore.Bool(omcore.Obj(st, "limits"), key, false))
	case "tor_ip", "tor_port", "tor_internal_ip", "tor_internal_port":
		tor := omcore.Obj(st, "tor")
		k := strings.TrimPrefix(key, "tor_")
		if v, ok := tor[k]; ok {
			return fmt.Sprint(v)
		}
		return ""
	case "tor_mode":
		switch omcore.Str(omcore.Obj(st, "tor"), "mode") {
		case "external":
			return u.t("External Tor")
		case "internal":
			return u.t("Internal Tor")
		case "off":
			return u.t("Disabled")
		}
		return "?"
	case "default_theme":
		if omcore.StrDef(omcore.Obj(st, "design"), "theme", "light") == "light" {
			return u.t("Light")
		}
		return u.t("Dark")
	case "tor_bridges":
		return u.onOff(omcore.Bool(omcore.Obj(st, "tor"), "bridges", false))
	case "tor_bridge_lines":
		v := strings.ReplaceAll(omcore.Str(omcore.Obj(st, "tor"), "bridge_lines"), "\n", " ; ")
		if v == "" {
			return "-"
		}
		return v
	}
	if lim := omcore.Obj(st, "limits"); lim != nil {
		if _, ok := lim[key]; ok {
			return fmt.Sprint(lim[key])
		}
	}
	if it.kind == "friend" {
		return "Enter: " + u.t("Delete")
	}
	return ""
}
func (u *UI) onOff(v bool) string {
	if v {
		return u.t("ON")
	}
	return u.t("OFF")
}
func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (u *UI) edit(label, current string, secret bool) *string {
	s := []rune(current)
	pos := len(s)
	for {
		u.termSize()
		shown := string(s)
		if secret {
			shown = strings.Repeat("*", len(s))
		}
		u.paint(u.rows-1, 1, ansiAcc+pad(" "+u.t(label)+": ", u.cols))
		maxw := u.cols - 4
		off := 0
		if runeLen(shown) > maxw {
			off = pos - maxw + 1
			if off < 0 {
				off = 0
			}
		}
		view := trunc(string([]rune(shown)[off:]), maxw)
		u.paint(u.rows, 1, "  "+view)
		fmt.Printf("\x1b[%d;%dH", u.rows, 2+pos-off)
		k := u.readKey()
		switch k {
		case "enter":
			out := string(s)
			return &out
		case "esc":
			return nil
		case "backspace":
			if pos > 0 {
				s = append(s[:pos-1], s[pos:]...)
				pos--
			}
		case "delete":
			if pos < len(s) {
				s = append(s[:pos], s[pos+1:]...)
			}
		case "left":
			if pos > 0 {
				pos--
			}
		case "right":
			if pos < len(s) {
				pos++
			}
		case "home":
			pos = 0
		case "end":
			pos = len(s)
		default:
			if strings.HasPrefix(k, "text:") {
				for _, r := range []rune(k[5:]) {
					s = append(s[:pos], append([]rune{r}, s[pos:]...)...)
					pos++
				}
			}
		}
	}
}

func (u *UI) confirm(text string) bool {
	u.termSize()
	u.paint(u.rows, 1, ansiAcc+pad(" "+text+"  [y/N]", u.cols))
	for {
		k := u.readKey()
		if strings.HasPrefix(k, "text:") {
			switch strings.ToLower(k[5:]) {
			case "y":
				return true
			case "n":
				return false
			}
		}
		if k == "enter" || k == "esc" {
			return false
		}
	}
}

func (u *UI) choose(title string, options []string, current int) int {
	if current < 0 || current >= len(options) {
		current = 0
	}
	for {
		u.termSize()
		u.clear()
		u.paint(3, 3, ansiAccBold+title)
		for i, o := range options {
			text := "  " + u.t(o)
			if i == current {
				text = ansiSel + "<" + u.t(o) + ">" + "\x1b[0m"
			}
			u.paint(5+i, 5, text)
		}
		u.paint(u.rows, 3, ansiAcc+"↑↓ Enter Esc")
		fmt.Print("\x1b[0m")
		k := u.readKey()
		switch k {
		case "up":
			current = (current - 1 + len(options)) % len(options)
		case "down":
			current = (current + 1) % len(options)
		case "enter":
			return current
		case "esc", "ctrl-c":
			return -1
		default:
			if strings.HasPrefix(k, "text:") && k[5] >= '1' && k[5] <= '9' {
				i := int(k[5] - '1')
				if i < len(options) {
					return i
				}
			}
		}
	}
}

func (u *UI) languageScreen() {
	opts := []string{"Русский", "Deutsch", "Français", "English", "Español"}
	codes := []string{"ru", "de", "fr", "en", "es"}
	cur := 0
	for i, c := range codes {
		if c == u.lang {
			cur = i
		}
	}
	r := u.choose("Choose language / Выберите язык", opts, cur)
	if r >= 0 && r < len(codes) {
		u.st["lang"] = codes[r]
		u.lang = codes[r]
		_ = omcore.StateSave(u.st)
	}
}

func (u *UI) vouchers() []omcore.State {
	raw, ok := u.st["vouchers"].([]any)
	if !ok {
		return nil
	}
	out := make([]omcore.State, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, omcore.State(m))
		}
	}
	return out
}

func expOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shortVoucherDate(s string, withTime bool) string {
	if s == "" {
		return "-"
	}
	if withTime {
		if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
			return t.Format("02.01.06 15:04")
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t.Format("02.01.06")
	}
	return s
}

func (u *UI) voucherScreen() {
	sel, off := 0, 0
	msg := ""
	for {
		u.termSize()
		u.clear()
		u.paint(3, 3, ansiAccBold+u.t("Voucher manager"))
		vs := u.vouchers()
		if sel >= len(vs) {
			sel = max(0, len(vs)-1)
		}
		vis := max(1, u.rows-8)
		if sel < off {
			off = sel
		}
		if sel >= off+vis {
			off = sel - vis + 1
		}
		if off > max(0, len(vs)-vis) {
			off = max(0, len(vs)-vis)
		}
		u.paint(4, 3, ansiAcc+fmt.Sprintf("%s: %d", u.t("Vouchers"), len(vs)))
		today := time.Now().Format("2006-01-02")
		for pos := off; pos < len(vs) && pos < off+vis; pos++ {
			v := vs[pos]
			created := omcore.Str(v, "created")
			days := omcore.Str(v, "days")
			if days == "" {
				if n := omcore.Int(v, "days", 0); n > 0 {
					days = strconv.Itoa(n)
				} else {
					days = "-"
				}
			}
			exp := omcore.Str(v, "expires")
			if exp == "" {
				if n := omcore.Int(v, "days", 0); n > 0 {
					if t, err := time.ParseInLocation("2006-01-02 15:04", created, time.Local); err == nil {
						exp = t.AddDate(0, 0, n).Format("2006-01-02")
					}
				}
			}
			status := u.t("Active")
			if exp != "" && exp < today {
				status = u.t("Expired")
			}
			code := omcore.Str(v, "code")
			avail := u.cols - 3
			line := code + "  " + expOrDash(created) + "  " + expOrDash(exp) + "  " + status
			if runeLen(line) > avail {
				line = code + "  " + shortVoucherDate(created, true) + " → " + shortVoucherDate(exp, false) + "  " + status
			}
			if runeLen(line) > avail {
				line = code + "  " + shortVoucherDate(exp, false) + "  " + status
			}
			if runeLen(line) > avail {
				line = code + "  " + status
			}
			y := 5 + (pos - off)
			if pos == sel {
				u.paint(y, 3, ansiSel+line+"\x1b[0m")
			} else {
				u.paint(y, 3, line)
			}
		}
		if len(vs) == 0 {
			u.paint(5, 3, u.t("No vouchers."))
		}
		u.paint(u.rows-2, 3, ansiAcc+"[G] "+u.t("Generate")+"    [D] "+u.t("Delete")+"    [T] "+u.t("Copy")+"    [S] "+u.t("Save")+"    [Q] "+u.t("Close"))
		u.paint(u.rows-1, 3, ansiMut+"↑↓/Tab "+u.t("Selected voucher"))
		if msg != "" {
			u.paint(u.rows, 3, trunc(msg, u.cols-3))
		}
		fmt.Print("\x1b[0m")
		k := u.readKey()
		if k == "esc" || k == "text:q" || k == "text:Q" {
			return
		}
		if k == "text:t" || k == "text:T" {
			if len(vs) == 0 || omcore.Str(vs[sel], "code") == "" {
				msg = u.t("No vouchers.")
			} else if clipboardCopy(omcore.Str(vs[sel], "code")) {
				msg = u.t("Voucher copied") + ": " + omcore.Str(vs[sel], "code")
			} else {
				msg = u.t("Clipboard unavailable")
			}
			continue
		}
		if k == "up" {
			sel = max(0, sel-1)
			msg = ""
			continue
		}
		if k == "down" || k == "tab" {
			sel = min(max(0, len(vs)-1), sel+1)
			msg = ""
			continue
		}
		if k == "text:g" || k == "text:G" {
			ans := u.edit("Validity (days)", "", false)
			if ans == nil {
				continue
			}
			days := 30
			if strings.TrimSpace(*ans) != "" {
				n, err := strconv.Atoi(strings.TrimSpace(*ans))
				if err != nil {
					msg = u.t("Days must be a number.")
					continue
				}
				days = n
			}
			if days < 1 || days > 36500 {
				msg = u.t("Days must be 1..36500.")
				continue
			}
			u.paint(u.rows, 3, u.t("Working..."))
			fmt.Print("\x1b[0m")
			line, err := omcore.ControlCommand(fmt.Sprintf("vouchermk %d", days*24*60), 15*time.Second)
			if err == nil && strings.HasPrefix(line, "+OK") {
				code := strings.TrimSpace(line[3:])
				now := time.Now()
				u.st["vouchers"] = append(u.vouchers(), omcore.State{
					"code": code, "created": now.Format("2006-01-02 15:04"),
					"days": days, "expires": now.AddDate(0, 0, days).Format("2006-01-02"),
				})
				_ = omcore.StateSave(u.st)
				sel = len(u.vouchers()) - 1
				msg = u.t("Voucher created") + ": " + code
			} else {
				msg = line
			}
		} else if (k == "text:d" || k == "text:D" || k == "delete") && len(vs) > 0 {
			code := omcore.Str(vs[sel], "code")
			if code == "" || !u.confirm(u.t("Delete this voucher?")) {
				continue
			}
			u.paint(u.rows, 3, u.t("Working..."))
			fmt.Print("\x1b[0m")
			line, err := omcore.ControlCommand("voucher "+code, 15*time.Second)
			if err == nil && strings.HasPrefix(line, "+OK") {
				arr := u.vouchers()
				arr = append(arr[:sel], arr[sel+1:]...)
				u.st["vouchers"] = arr
				_ = omcore.StateSave(u.st)
				msg = u.t("Voucher deleted") + ": " + code
			} else {
				msg = u.t("Delete failed") + ": " + line
			}
		} else if k == "text:s" || k == "text:S" {
			_ = omcore.StateSave(u.st)
			msg = u.t("Saved.")
		}
	}
}

func clipboardCopy(text string) bool {
	data := base64Encode(text)
	_, err := fmt.Printf("\x1b]52;c;%s\a", data)
	return err == nil
}
func base64Encode(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(s)
	var out strings.Builder
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rem := len(b) - i
		n = uint32(b[i]) << 16
		if rem > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(b[i+2])
		}
		out.WriteByte(alphabet[(n>>18)&63])
		out.WriteByte(alphabet[(n>>12)&63])
		if rem > 1 {
			out.WriteByte(alphabet[(n>>6)&63])
		} else {
			out.WriteByte('=')
		}
		if rem > 2 {
			out.WriteByte(alphabet[n&63])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}

func (u *UI) logScreen() {
	text := omcore.Read(omcore.TorLog)
	lines := strings.Split(text, "\n")
	if len(lines) > 300 {
		lines = lines[len(lines)-300:]
	}
	off := max(0, len(lines)-20)
	for {
		u.termSize()
		u.clear()
		u.paint(2, 3, ansiAccBold+u.t("Tor log"))
		for i := 0; i < max(1, u.rows-5) && off+i < len(lines); i++ {
			u.paint(4+i, 3, trunc(lines[off+i], u.cols-3))
		}
		u.paint(u.rows, 3, ansiAcc+"↑↓ PgUp/PgDn  Q")
		fmt.Print("\x1b[0m")
		k := u.readKey()
		switch k {
		case "esc", "text:q", "text:Q":
			return
		case "up":
			off = max(0, off-1)
		case "down":
			off = min(max(0, len(lines)-1), off+1)
		case "pgup":
			off = max(0, off-20)
		case "pgdn":
			off = min(max(0, len(lines)-1), off+20)
		}
	}
}

func (u *UI) setValue(it item, val string) {
	st := u.st
	key := it.key
	switch key {
	case "server_name", "onion", "nick", "locale", "admin_password":
		st[key] = val
		return
	case "relay", "admin_enabled", "admin_captcha":
		st[key] = val != ""
		return
	}
	if lim := omcore.Obj(st, "limits"); lim != nil {
		if _, ok := lim[key]; ok {
			if key == "debug" || key == "autodelete" || key == "exit" || key == "logvoucher" {
				lim[key] = val != ""
			} else if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				lim[key] = n
			}
			return
		}
	}
	if strings.HasPrefix(key, "reg_") {
		omcore.Obj(st, "registration")[key[4:]] = val != ""
		return
	}
	switch key {
	case "voucher_required", "exit_name_enabled", "exit_enabled":
		omcore.Obj(st, "registration")[key] = val != ""
	case "exit_name":
		omcore.Obj(st, "registration")[key] = val
	case "default_theme":
		omcore.Obj(st, "design")["theme"] = val
	}
	if strings.HasPrefix(key, "tor_") {
		k := key[4:]
		tor := omcore.Obj(st, "tor")
		switch k {
		case "mode", "bridge_lines", "ip", "internal_ip":
			tor[k] = val
		case "bridges":
			tor[k] = val != ""
		case "port", "internal_port":
			if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				tor[k] = n
			}
		}
	}
}

func (u *UI) toggleBool(it item) {
	st := u.st
	key := it.key
	if _, ok := st[key]; ok {
		st[key] = !omcore.Bool(st, key, false)
		return
	}
	if strings.HasPrefix(key, "reg_") {
		reg := omcore.Obj(st, "registration")
		reg[key[4:]] = !omcore.Bool(reg, key[4:], false)
		return
	}
	if reg := omcore.Obj(st, "registration"); reg != nil {
		if _, ok := reg[key]; ok {
			reg[key] = !omcore.Bool(reg, key, false)
			return
		}
	}
	if lim := omcore.Obj(st, "limits"); lim != nil {
		if _, ok := lim[key]; ok {
			lim[key] = !omcore.Bool(lim, key, false)
			return
		}
	}
	if strings.HasPrefix(key, "tor_") {
		tor := omcore.Obj(st, "tor")
		k := key[4:]
		if _, ok := tor[k]; ok {
			tor[k] = !omcore.Bool(tor, k, false)
		}
	}
}

func (u *UI) handle(it item) {
	switch it.kind {
	case "action":
		switch it.key {
		case "save":
			ok, errs := omcore.Save(u.st, u.section())
			if ok {
				u.setMsg(u.t("Saved.")+" "+u.t("Restart required for some settings."), true)
			} else {
				u.setMsg(u.t("Validation error")+": "+strings.Join(first(errs, 4), "; "), false)
			}
		case "get_onion":
			if v := omcore.GetOnion(u.st); v != "" {
				u.st["onion"] = v
				u.setMsg(u.t("Saved."), true)
			} else {
				u.setMsg("Onion address not found", false)
			}
		case "add_friend":
			v := u.edit("Friend", "", false)
			if v != nil {
				s := strings.TrimSpace(*v)
				if regexp.MustCompile(`^[a-z2-7]{16,56}\.onion$`).MatchString(s) {
					found := false
					for _, f := range omcore.FriendsList(u.st) {
						if f == s {
							found = true
						}
					}
					if !found {
						u.st["friends"] = append(omcore.Arr(u.st, "friends"), s)
					}
					u.setMsg(u.t("Saved."), true)
				} else if s != "" {
					u.setMsg("friend: invalid onion", false)
				}
			}
		case "add_logo":
			v := u.edit("Logo path", "", false)
			if v != nil && *v != "" {
				ok, msg := omcore.InstallLogo(u.st, *v, u.t)
				u.setMsg(msg, ok)
			}
		case "reset_logo":
			ok, msg := omcore.ResetLogo(u.st, u.t)
			u.setMsg(msg, ok)
		case "add_big_logo":
			v := u.edit(u.t("Big logo path"), "", false)
			if v != nil {
				if strings.TrimSpace(*v) == "" {
					ok, msg := omcore.ResetBigLogo(u.st, u.t)
					u.setMsg(msg, ok)
				} else {
					ok, msg := omcore.InstallBigLogo(u.st, *v, u.t)
					u.setMsg(msg, ok)
				}
			}
		case "add_small_logo":
			v := u.edit(u.t("Small logo path"), "", false)
			if v != nil {
				if strings.TrimSpace(*v) == "" {
					ok, msg := omcore.ResetSmallLogo(u.st, u.t)
					u.setMsg(msg, ok)
				} else {
					ok, msg := omcore.InstallSmallLogo(u.st, *v, u.t)
					u.setMsg(msg, ok)
				}
			}
		case "vouchers":
			u.voucherScreen()
		case "tor_log":
			u.logScreen()
		case "tor_restart":
			if omcore.Str(omcore.Obj(u.st, "tor"), "mode") == "internal" {
				ok := omcore.StartTor(u.st)
				if ok {
					u.setMsg("Tor restarted", true)
				} else {
					u.setMsg("Tor failed; see log", false)
				}
			} else {
				u.setMsg("Restart is available only for internal Tor", false)
			}
		}
	case "friend":
		if u.confirm(u.t("Delete this friend?")) {
			var out []any
			for _, f := range omcore.FriendsList(u.st) {
				if f != it.key {
					out = append(out, f)
				}
			}
			u.st["friends"] = out
			u.setMsg(u.t("Saved."), true)
		}
	case "bool":
		u.toggleBool(it)
	case "choice":
		switch it.key {
		case "tor_mode":
			modes := []string{"external", "internal", "off"}
			cur := 0
			for i, m := range modes {
				if m == omcore.Str(omcore.Obj(u.st, "tor"), "mode") {
					cur = i
				}
			}
			r := u.choose("Tor mode", []string{"External Tor", "Internal Tor", "Disabled"}, cur)
			if r >= 0 {
				omcore.Obj(u.st, "tor")["mode"] = modes[r]
				u.sel = 0
			}
		case "default_theme":
			modes := []string{"light", "dark"}
			cur := 0
			if omcore.StrDef(omcore.Obj(u.st, "design"), "theme", "light") == "dark" {
				cur = 1
			}
			r := u.choose("Default theme", []string{"Light", "Dark"}, cur)
			if r >= 0 {
				omcore.Obj(u.st, "design")["theme"] = modes[r]
			}
		}
	case "text", "pass", "int":
		cur := u.valueFor(it)
		if it.key == "admin_password" {
			cur = ""
		}
		v := u.edit(it.label, cur, it.kind == "pass")
		if v != nil {
			u.setValue(it, *v)
			u.msg = ""
		}
	}
}
