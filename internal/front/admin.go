package front

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type adminUser struct {
	Login     string
	Exit      string
	LastLogin int64
	RecvCount int
	RecvBytes int64
	SentCount int
	SentBytes int64
	Status    string
	HasPass   bool
}

type dashboard struct {
	Users     []adminUser
	UserTotal int
	UserKnown int
	Stat      map[string]any
	Uptime    int
	Threads   map[string]any
	Web       map[string]any
	Version   string
	Exits     int
	Friends   int
	DiskTotal int64
	DiskFree  int64
	Received  int64
	Sent      int64
	UserData  int64
	Service   int64
	Total     int64
}

func (s *Server) collectDashboard(r *http.Request) dashboard {
	data := s.loadRegistry()
	users := data.Users
	if _, ok := users["root"]; !ok && pop3Exists("root") {
		users["root"] = map[string]any{"created": time.Now().Unix()}
		s.regMu.Lock()
		fresh := s.loadRegistry()
		if _, ok := fresh.Users["root"]; !ok {
			fresh.Users["root"] = users["root"]
			_ = s.saveRegistry(fresh)
		}
		s.regMu.Unlock()
	}
	re := userRe
	var rows []adminUser
	var logins []string
	for login := range users {
		if re.MatchString(login) {
			logins = append(logins, login)
		}
	}
	sort.Strings(logins)
	for _, login := range logins {
		entry := users[login]
		if entry == nil {
			entry = map[string]any{}
		}
		var recvCount, recvBytes int
		status := "nopass"
		pop3 := s.entryCredential(entry, "pop3")
		if pop3 != "" && pop3 != "<nil>" {
			var err error
			recvCount, recvBytes, err = pop3Stats(login, pop3)
			if err == nil {
				status = "ok"
			} else {
				status = "err"
			}
		} else if !pop3Exists(login) {
			status = "err"
		}
		sentItems := s.loadSent(login)
		var sentBytes int64
		for _, it := range sentItems {
			b, _ := json.Marshal(it)
			sentBytes += int64(len(b))
		}
		rows = append(rows, adminUser{
			Login: login, Exit: fmt.Sprint(entry["exit"]), LastLogin: int64(intVal(entry["last_login"])),
			RecvCount: recvCount, RecvBytes: int64(recvBytes), SentCount: len(sentItems), SentBytes: sentBytes,
			Status: status, HasPass: pop3 != "",
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].RecvBytes+rows[i].SentBytes > rows[j].RecvBytes+rows[j].SentBytes
	})
	inboxSize := dirSize(s.MailDir + "/inbox")
	sentSize := dirSize(s.SentDir)
	userSize := dirSize(s.MailDir+"/usr") + dirSize(s.MailDir+"/keys")
	if fi, err := os.Stat(s.Registry); err == nil {
		userSize += fi.Size()
	}
	totalAll := dirSize(s.MailDir) + dirSize("/var/lib/onionmail") + dirSize("/etc/onionmail")
	service := totalAll - inboxSize - sentSize - userSize
	if service < 0 {
		service = 0
	}
	var diskTotal, diskFree int64
	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		diskTotal = int64(fs.Blocks) * int64(fs.Bsize)
		diskFree = int64(fs.Bavail) * int64(fs.Bsize)
	}
	stat := s.controlStat(8 * time.Second)
	if stat == nil {
		stat = map[string]any{}
	}
	if v, ok := stat["UpTime"]; !ok || v == nil || fmt.Sprint(v) == "0" {
		stat["UpTime"] = processUptime()
	}
	threads, _ := s.adminAPI(r, "/admin/threads.api").(map[string]any)
	web, _ := s.adminAPI(r, "/admin/stats.api").(map[string]any)
	ver, _ := s.adminAPI(r, "/admin/ver.api").(map[string]any)
	exits := countList(s.adminAPI(r, "/admin/elist.api"))
	friends := countList(s.adminAPI(r, "/admin/friends.api"))
	version := s.controlVer()
	if version == "" {
		if ver != nil {
			version = fmt.Sprint(ver["ver"])
			if av, ok := ver["apiVer"]; ok && fmt.Sprint(av) != "" {
				version += " / api " + fmt.Sprint(av)
			}
		} else {
			version = "—"
		}
	}
	if version == "" || version == "<nil>" {
		version = "—"
	}
	if threads == nil {
		threads = map[string]any{}
	}
	if web == nil {
		web = map[string]any{}
	}
	return dashboard{
		Users: rows, UserTotal: len(rows),
		Stat: stat, Uptime: processUptime(), Threads: threads, Web: web,
		Version: version, Exits: exits, Friends: friends,
		DiskTotal: diskTotal, DiskFree: diskFree,
		Received: inboxSize, Sent: sentSize, UserData: userSize, Service: service, Total: totalAll,
	}
}

func (s *Server) countKnown() int { return 0 }

func (d dashboard) known() int {
	n := 0
	for _, u := range d.Users {
		if u.HasPass {
			n++
		}
	}
	return n
}

func (s *Server) storedSMTP(data registryData, login string) bool {
	return strings.TrimSpace(s.entryCredential(data.Users[login], "smtp")) != ""
}

func countList(value any) int {
	switch v := value.(type) {
	case []any:
		return len(v)
	case map[string]any:
		for _, key := range []string{"elist", "list", "friends", "items"} {
			if arr, ok := v[key].([]any); ok {
				return len(arr)
			}
		}
		return len(v)
	}
	return 0
}

func (s *Server) adminDashboard(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		s.redirect(w, r, "/logon.html", nil)
		return
	}
	message, errMsg := "", ""
	if r.Method == http.MethodPost {
		f := s.form(r)
		if firstVal(f, "csrf") != s.AdminToken {
			errMsg = ctx.t("CSRF error: reload the page and try again.")
		} else {
			login := strings.ToLower(strings.TrimSpace(firstVal(f, "login")))
			pop3 := strings.TrimSpace(firstVal(f, "pop3"))
			smtp := strings.TrimSpace(firstVal(f, "smtp"))
			exitlogin := strings.ToLower(strings.TrimSpace(firstVal(f, "exit")))
			if firstVal(f, "noexit") != "" {
				exitlogin = ""
			}
			if !userRe.MatchString(login) {
				errMsg = ctx.t("Login: 3–16 characters, Latin letters/digits/._-, no @.")
			} else if exitlogin != "" && !validExitLogin(exitlogin) {
				errMsg = ctx.t("Exit login: 3–30 characters, Latin letters/digits/._-.")
			} else if smtp == "" && !s.storedSMTP(s.loadRegistry(), login) {
				errMsg = ctx.t("Enter the SMTP password for a new mailbox.")
			} else if _, _, err := pop3Stats(login, pop3); err != nil {
				errMsg = ctx.tf("POP3 did not accept login “{login}” with this password.", map[string]string{"login": login})
			} else if smtp != "" {
				sm, err := dialSMTP(login, smtp, 30*time.Second)
				if err != nil {
					errMsg = ctx.tf("SMTP did not accept login “{login}” with this password.", map[string]string{"login": login})
				} else {
					sm.quit()
				}
			}
			if errMsg == "" {
				s.registerUser(login, smtp, pop3, exitlogin, 0)
				s.redirect(w, r, "/admin/index.html?ok=1", nil)
				return
			}
		}
	} else if r.URL.Query().Get("ok") != "" {
		message = ctx.t("Mailbox verified via POP3 and saved.")
	}
	s.sendHTML(w, r, 200, s.renderDashboard(ctx, s.collectDashboard(r), message, errMsg), nil)
}

type segment struct {
	Name  string
	Value int64
	Color string
}

func chartPalette(dark bool) (string, string, string, string, []string, string, string, string) {
	if dark {
		return "#b794ff", "#8b5cf6", "#6d4ac9", "#4a3563", []string{"#b794ff", "#9d6cff", "#8353e6", "#6d4ac9", "#4a3563"}, "#4a3563", "#b794ff", "#d99a5c"
	}
	return C_RECV, C_SENT, C_USER, C_SERVICE, []string{"#004488", "#2f7fd1", "#5b9bd5", "#86b7e4", "#b3d1ec"}, "#c9d5e2", C_IN, C_OUT
}

func donutSVG(segments []segment, centerTop, centerBottom string, dark bool, valueFmt func(int64) string) string {
	var total int64
	for _, s := range segments {
		if s.Value > 0 {
			total += s.Value
		}
	}
	ring := "#e7edf3"
	textMain := "#16232f"
	textSub := "#748394"
	if dark {
		ring, textMain, textSub = "#332145", "#f4ecff", "#a99bc0"
	}
	var parts strings.Builder
	if total <= 0 {
		fmt.Fprintf(&parts, `<circle cx="95" cy="95" r="64" fill="none" stroke="%s" stroke-width="26"/>`, ring)
	} else {
		offset := 0.0
		for _, s := range segments {
			if s.Value <= 0 {
				continue
			}
			pct := float64(s.Value) * 100.0 / float64(total)
			fmt.Fprintf(&parts, `<circle cx="95" cy="95" r="64" fill="none" stroke="%s" stroke-width="26" pathLength="100" stroke-dasharray="%.3f %.3f" stroke-dashoffset="%.3f" transform="rotate(-90 95 95)"><title>%s: %s (%.1f%%)</title></circle>`,
				s.Color, pct, 100-pct, -offset, html.EscapeString(s.Name), valueFmt(s.Value), pct)
			offset += pct
		}
	}
	svg := `<svg class="donut" viewBox="0 0 190 190" width="190" height="190" role="img" aria-label="` + html.EscapeString(centerTop) + `">` +
		parts.String() +
		fmt.Sprintf(`<text x="95" y="91" text-anchor="middle" font-size="15" font-weight="700" fill="%s">%s</text>`, textMain, html.EscapeString(centerTop)) +
		fmt.Sprintf(`<text x="95" y="110" text-anchor="middle" font-size="10.5" fill="%s">%s</text>`, textSub, html.EscapeString(centerBottom)) +
		`</svg>`
	var legend strings.Builder
	legend.WriteString(`<div class="legend">`)
	for _, s := range segments {
		pct := 0.0
		if total > 0 {
			pct = float64(s.Value) * 100.0 / float64(total)
		}
		fmt.Fprintf(&legend, `<div class="lg"><span class="dot" style="background:%s"></span><span class="nm">%s</span><b>%s</b><span class="pc">%.0f%%</span></div>`,
			s.Color, html.EscapeString(s.Name), valueFmt(s.Value), pct)
	}
	legend.WriteString(`</div>`)
	return `<div class="chart">` + svg + legend.String() + `</div>`
}

func adminCard(label, value, sub string) string {
	subHTML := ""
	if sub != "" {
		subHTML = `<div class="s">` + html.EscapeString(sub) + `</div>`
	}
	return `<div class="card"><div class="k">` + html.EscapeString(label) + `</div><div class="v">` + html.EscapeString(value) + `</div>` + subHTML + `</div>`
}

func (s *Server) renderDashboard(ctx *reqCtx, data dashboard, message, errMsg string) string {
	alerts := ""
	if message != "" {
		alerts += `<div class="alert ok">` + html.EscapeString(message) + `</div>`
	}
	if errMsg != "" {
		alerts += `<div class="alert err">` + html.EscapeString(errMsg) + `</div>`
	}
	stat := data.Stat
	dark := ctx.theme == "dark"
	recvC, sentC, userC, serviceC, topColors, otherC, inC, outC := chartPalette(dark)
	storageSegments := []segment{
		{ctx.t("Received mail"), data.Received, recvC},
		{ctx.t("Sent mail"), data.Sent, sentC},
		{ctx.t("User data"), data.UserData, userC},
		{ctx.t("Service files"), data.Service, serviceC},
	}
	users := append([]adminUser{}, data.Users...)
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].RecvBytes+users[i].SentBytes > users[j].RecvBytes+users[j].SentBytes
	})
	var top []adminUser
	for _, u := range users {
		if u.RecvBytes+u.SentBytes > 0 {
			top = append(top, u)
			if len(top) == 5 {
				break
			}
		}
	}
	var topSegments []segment
	for i, u := range top {
		topSegments = append(topSegments, segment{u.Login, u.RecvBytes + u.SentBytes, topColors[i%5]})
	}
	var topSum, allSum int64
	for _, seg := range topSegments {
		topSum += seg.Value
	}
	for _, u := range users {
		allSum += u.RecvBytes + u.SentBytes
	}
	if rest := allSum - topSum; rest > 0 {
		topSegments = append(topSegments, segment{ctx.t("Other"), rest, otherC})
	}
	var mailboxCount, sentCopies int64
	for _, u := range users {
		mailboxCount += int64(u.RecvCount)
		sentCopies += int64(u.SentCount)
	}
	mailSegments := []segment{
		{ctx.t("Incoming"), mailboxCount, inC},
		{ctx.t("Outgoing"), sentCopies, outC},
	}
	inCount := statInt(stat, "Messages-In")
	outCount := statInt(stat, "Messages-Out")
	cards := adminCard(ctx.t("Users"), fmtInt(int64(data.UserTotal)),
		ctx.tf("with POP3 password: {n}", map[string]string{"n": fmtInt(int64(data.known()))})) +
		adminCard(ctx.t("Messages in mailboxes"), fmtInt(mailboxCount),
			ctx.tf("server received: {n} · {size}", map[string]string{"n": fmtInt(inCount), "size": fmtSize(data.Received)})) +
		adminCard(ctx.t("Sent"), fmtInt(sentCopies),
			ctx.tf("server sent: {n} · {size}", map[string]string{"n": fmtInt(outCount), "size": fmtSize(data.Sent)})) +
		adminCard(ctx.t("User data"), fmtSize(data.UserData), ctx.t("usr, keys, registry")) +
		adminCard(ctx.t("Service files"), fmtSize(data.Service), ctx.t("indexes, Tor, logs, www")) +
		adminCard(ctx.t("Free disk"), fmtSize(data.DiskFree), ctx.tf("of {size}", map[string]string{"size": fmtSize(data.DiskTotal)})) +
		adminCard(ctx.t("Server uptime"), fmtUptime(data.Uptime),
			ctx.tf("threads: {n}", map[string]string{"n": fmtInt(int64(intVal(data.Threads["thr"])))})) +
		adminCard(ctx.t("Exit / Friends"), fmtInt(int64(data.Exits))+` / `+fmtInt(int64(data.Friends)),
			ctx.tf("spam: {n}", map[string]string{"n": fmtInt(statInt(stat, "Spam-Blocked"))}))
	var rows strings.Builder
	for _, u := range users {
		badge := ""
		switch u.Status {
		case "ok":
			badge = `<span class="badge ok">` + html.EscapeString(ctx.t("ok")) + `</span>`
		case "nopass":
			badge = `<span class="badge warn">` + html.EscapeString(ctx.t("no password")) + `</span>`
		default:
			badge = `<span class="badge err">` + html.EscapeString(ctx.t("error")) + `</span>`
		}
		lastS := "—"
		if u.LastLogin > 0 {
			lastS = time.Unix(u.LastLogin, 0).Format("02.01.2006 15:04")
		}
		exit := u.Exit
		if exit == "" {
			exit = "—"
		}
		fmt.Fprintf(&rows, `<tr><td><span class="login">%s</span></td><td class="mut">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="num">%s</td><td class="mut">%s</td><td>%s</td></tr>`,
			html.EscapeString(u.Login), html.EscapeString(exit), fmtInt(int64(u.RecvCount)), fmtSize(u.RecvBytes),
			fmtInt(int64(u.SentCount)), fmtSize(u.SentBytes), html.EscapeString(lastS), badge)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<tr><td colspan="8" class="empty">` + html.EscapeString(ctx.t("No mailboxes in the registry yet.")) + `</td></tr>`)
	}
	webHits := int64(intVal(data.Web["httph"]))
	webErr := int64(intVal(data.Web["httpe"]))
	footer := ctx.tf("Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}",
		map[string]string{"version": data.Version, "hits": fmtInt(webHits), "errors": fmtInt(webErr),
			"new": fmtInt(statInt(stat, "NewUsrThisDay")), "errs": fmtInt(statInt(stat, "Errors"))})
	privacy := ctx.tf("POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.",
		map[string]string{"path": s.Registry})
	storageTotal := data.Total
	topHTML := `<div class="empty">` + html.EscapeString(ctx.t("No mailbox size data.")) + `</div>`
	var topVal int64
	for _, seg := range topSegments {
		topVal += seg.Value
	}
	if len(topSegments) > 0 {
		topHTML = donutSVG(topSegments, fmtSize(topVal), ctx.t("of users"), dark, fmtSize)
	}
	return `<!doctype html>
<html lang="` + ctx.lang + `"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>` + html.EscapeString(s.DisplayName) + ` · sysop</title>
<style>` + ctx.themeCSS(ADMIN_CSS) + `</style></head><body><div class="wrap">
<div class="topbar">` + s.prefsSelector(ctx) + `<div class="brand">` + html.EscapeString(s.DisplayName) + `<span>OnionMail · sysop panel</span></div>
<nav><a class="on" href="/admin/index.html">` + html.EscapeString(ctx.t("Statistics")) + `</a>
<a href="/admin/server.html">` + html.EscapeString(ctx.t("Server")) + `</a>
<a href="/admin/elist.html">` + html.EscapeString(ctx.t("Net")) + `</a>
<a href="/admin/voucher.html">` + html.EscapeString(ctx.t("Vouchers")) + `</a>
<a class="refresh" href="/admin/index.html" title="` + html.EscapeString(ctx.t("Refresh")) + `">↻ ` + html.EscapeString(ctx.t("Refresh")) + `</a>
<a href="/admin/logout.html">` + html.EscapeString(ctx.t("Log out")) + `</a></nav></div>
` + alerts + `
<div class="cards">` + cards + `</div>
<div class="charts">
<div class="panel"><h2>` + html.EscapeString(ctx.t("Storage")) + `</h2><div class="hint">` + html.EscapeString(ctx.tf("Total: {total} · free {free}", map[string]string{"total": fmtSize(storageTotal), "free": fmtSize(data.DiskFree)})) + `</div>
` + donutSVG(storageSegments, fmtSize(storageTotal), ctx.t("occupied by data"), dark, fmtSize) + `</div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("Top mailboxes by size")) + `</h2><div class="hint">` + html.EscapeString(ctx.t("Received + sent")) + `</div>
` + topHTML + `</div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("Mail")) + `</h2><div class="hint">` + html.EscapeString(ctx.t("OnionMail counters")) + `</div>
` + donutSVG(mailSegments, fmtInt(mailboxCount+sentCopies), ctx.t("messages total"), dark, func(v int64) string { return fmtInt(v) }) + `</div>
</div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("Mailboxes")) + `</h2>
<div class="hint">` + html.EscapeString(ctx.t("OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below.")) + `</div>
<div class="tblscroll"><table><thead><tr><th>` + html.EscapeString(ctx.t("Mailbox")) + `</th><th>` + html.EscapeString(ctx.t("Exit login")) + `</th><th>` + html.EscapeString(ctx.t("Inbox")) + `</th><th>` + html.EscapeString(ctx.t("Size")) + `</th><th>` + html.EscapeString(ctx.t("Sent")) + `</th><th>` + html.EscapeString(ctx.t("Size")) + `</th><th>` + html.EscapeString(ctx.t("Last login")) + `</th><th>` + html.EscapeString(ctx.t("Status")) + `</th></tr></thead>
<tbody>` + rows.String() + `</tbody></table></div></div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("Add / update mailbox")) + `</h2>
<form class="add" method="post" action="/admin/storage.html">
<input type="hidden" name="csrf" value="` + html.EscapeString(s.AdminToken) + `">
<div><label for="login">` + html.EscapeString(ctx.t("Login (no @)")) + `</label><input id="login" name="login" required maxlength="16" placeholder="user"></div>
<div><label for="pop3">` + html.EscapeString(ctx.t("POP3 password")) + `</label><input id="pop3" name="pop3" type="password" required autocomplete="new-password"></div>
<div><label for="smtp">` + html.EscapeString(ctx.t("SMTP password")) + `</label><input id="smtp" name="smtp" type="password" autocomplete="new-password"><small style="display:block;margin-top:4px;color:#5b6b7c">` + html.EscapeString(ctx.t("Required for a new mailbox; leave blank to keep the stored SMTP password.")) + `</small></div>
<div><label for="exit">` + html.EscapeString(ctx.t("Exit login (optional)")) + `</label><input id="exit" name="exit" maxlength="30" placeholder="alias"></div>
<div style="grid-column:1/-1"><label style="display:flex;gap:8px;align-items:center;text-transform:none;letter-spacing:0;font-size:12.5px;color:#5b6b7c"><input type="checkbox" name="noexit" value="1" style="width:auto"> ` + html.EscapeString(ctx.t("No exit login (only the .onion address)")) + `</label></div>
<div><button type="submit">` + html.EscapeString(ctx.t("Check and save")) + `</button></div>
</form></div>
<div class="foot">` + html.EscapeString(footer) + `<br>
` + html.EscapeString(privacy) + `<br>
` + html.EscapeString(ctx.t("The Logon page and admin panel use the POP3 password; the SMTP password is only for sending mail.")) + `</div>
</div></body></html>`
}

func statInt(stat map[string]any, key string) int64 {
	if stat == nil {
		return 0
	}
	if v, ok := stat[key]; ok {
		return int64(intVal(v))
	}
	return 0
}

func _unusedStrconv() { _ = strconv.Atoi }
