package front

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const regLoginRow = `<tr><td>Login:</td><td><input type="text" name="om-localpart" size="40"></td></tr>`

func (s *Server) regExitRow(ctx *reqCtx) string {
	hint := ctx.tf("Only for the @{domain} address. To sign in at /web use the main login (without @).", map[string]string{"domain": ExitDomain})
	return `<tr><td>` + html.EscapeString(ctx.t("Exit login")) + `:</td><td><input type="text" name="om-exitlogin" size="40" placeholder="for @` + ExitDomain + `"></td></tr>` +
		`<tr><td></td><td><label style="display:flex;gap:7px;align-items:center;font-size:12px;color:#5b6b7c"><input type="checkbox" name="om-noexit" value="1"> ` + html.EscapeString(ctx.t("No exit login (only the .onion address)")) + `</label></td></tr>` +
		`<tr><td></td><td><span style="font-size:12px;color:#748394">` + html.EscapeString(hint) + `</span></td></tr>`
}

func (s *Server) smallPage(ctx *reqCtx, title, message string) string {
	return `<!doctype html><html lang="` + ctx.lang + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + ctx.themeCSS(SMALL_CSS) + `</style></head><body><div class="b"><h1>` + html.EscapeString(title) +
		`</h1><div class="err">` + html.EscapeString(message) + `</div><p><a href="/register.html">` + html.EscapeString(ctx.t("Back")) + `</a></p></div></body></html>`
}

func (s *Server) registerPage(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	status, headers, data, err := s.fetchUpstream(r, nil, "", http.MethodGet)
	if err != nil {
		s.logf("upstream error: %v", err)
		s.sendHTML(w, r, 502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>", nil)
		return
	}
	reg := s.registration()
	if status == 200 {
		if omcoreBoolDefault(reg, "exit_name_enabled", true) && bytes.Contains(data, []byte(regLoginRow)) {
			data = bytes.Replace(data, []byte(regLoginRow), []byte(regLoginRow+"\n\t"+s.regExitRow(ctx)), 1)
		}
		if !omcoreBoolDefault(reg, "exit_enabled", true) {
			data = regexp.MustCompile(`(?s)<tr><td>Exit Server:</td>.*?</tr>`).ReplaceAll(data, nil)
		}
		if omcoreBoolDefault(reg, "voucher_required", false) {
			data = bytes.ReplaceAll(data, []byte("* = Not required."), []byte("* = Required: enter a valid voucher code."))
		}
		headers.Del("Content-Length")
	}
	s.relay(ctx, w, r, status, headers, data)
}

func omcoreBoolDefault(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		return omcoreBool(v, def)
	}
	return def
}

func omcoreBool(v any, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "yes" || x == "true" || x == "1"
	case float64:
		return x != 0
	}
	return def
}

func (s *Server) registerPost(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	raw := s.readBody(r, 300_000)
	fields, _ := parseQuery(string(raw))
	login := strings.ToLower(strings.TrimSpace(firstVal(fields, "om-localpart")))
	reg := s.registration()
	if !omcoreBoolDefault(reg, "exit_name_enabled", true) {
		delete(fields, "om-exitlogin")
	}
	if !omcoreBoolDefault(reg, "exit_enabled", true) {
		delete(fields, "om-exit")
	}
	noExit := firstVal(fields, "om-noexit") != ""
	delete(fields, "om-noexit")
	exitlogin := strings.ToLower(strings.TrimSpace(firstVal(fields, "om-exitlogin")))
	delete(fields, "om-exitlogin")
	if noExit {
		delete(fields, "om-exit")
		exitlogin = ""
	}
	voucherCode := strings.TrimSpace(firstVal(fields, "om-voucher"))
	if omcoreBoolDefault(reg, "voucher_required", false) && voucherCode == "" {
		s.sendHTML(w, r, 400, s.smallPage(ctx, ctx.t("Voucher required"), ctx.t("Registration is available only with a valid voucher code.")), nil)
		return
	}
	if omcoreBoolDefault(reg, "voucher_required", false) && voucherCode != "" {
		if reply, err := s.controlCommand("voucherchk "+voucherCode, 10*time.Second); err == nil && !strings.HasPrefix(reply, "+OK") {
			s.sendHTML(w, r, 400, s.smallPage(ctx, ctx.t("Voucher required"), ctx.t("This voucher code is invalid or expired. Ask the administrator for a new code.")), nil)
			return
		}
	}
	if exitlogin != "" && exitlogin != login && !validExitLogin(exitlogin) {
		s.sendHTML(w, r, 400, s.smallPage(ctx, ctx.t("Invalid exit login"),
			ctx.t("Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list.")), nil)
		return
	}
	var pairs [][2]string
	for k, vs := range fields {
		for _, v := range vs {
			pairs = append(pairs, [2]string{k, v})
		}
	}
	body := []byte(formEncode(pairs))
	status, headers, data, err := s.fetchUpstream(r, body, "", http.MethodPost)
	if err != nil {
		s.logf("upstream error: %v", err)
		s.sendHTML(w, r, 502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>", nil)
		return
	}
	success := status == 200 && bytes.Contains(data, []byte("Subscription complete"))
	if exitlogin != "" && exitlogin != login && success {
		aliasOK := false
		reply, err := s.controlCommand("addalias "+exitlogin+" "+login, 10*time.Second)
		if err == nil {
			aliasOK = strings.HasPrefix(reply, "+OK")
			s.logf("exit login %s -> %s: %s", exitlogin, login, reply)
		} else {
			s.logf("exit login %s -> %s failed: %v", exitlogin, login, err)
		}
		if aliasOK {
			data = bytes.ReplaceAll(data, []byte(login+"@"+ExitDomain), []byte(exitlogin+"@"+ExitDomain))
		} else {
			warnText := ctx.tf("Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.",
				map[string]string{"exit": exitlogin, "login": login, "domain": ExitDomain})
			warn := []byte(`<div class="om-warn">` + html.EscapeString(warnText) + `</div>`)
			if bytes.Contains(data, []byte("</body>")) {
				data = bytes.Replace(data, []byte("</body>"), append(warn, []byte("</body>")...), 1)
			} else {
				data = append(data, warn...)
			}
		}
		if exitlogin != "" {
			noteText := ctx.tf("Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.",
				map[string]string{"login": login, "exit": exitlogin, "domain": ExitDomain})
			note := []byte(`<div class="om-note">` + html.EscapeString(noteText) + `</div>`)
			if bytes.Contains(data, []byte("</body>")) {
				data = bytes.Replace(data, []byte("</body>"), append(note, []byte("</body>")...), 1)
			} else {
				data = append(data, note...)
			}
		}
		headers.Del("Content-Length")
	}
	if success {
		if omcoreBoolDefault(reg, "voucher_required", false) && voucherCode != "" {
			state := s.omState()
			if arr, ok := state["vouchers"].([]any); ok {
				var kept []any
				for _, v := range arr {
					if m, ok := v.(map[string]any); ok && fmt.Sprint(m["code"]) == voucherCode {
						continue
					}
					kept = append(kept, v)
				}
				state["vouchers"] = kept
				s.omStateSave(state)
			}
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.logf("registry registration update failed: %v", rec)
				}
			}()
			sm, po := parseSuccessCreds(string(data))
			if sm != "" && po != "" {
				s.registerUser(login, sm, po, exitlogin, 0)
			}
			if noExit {
				s.clearUserExit(login)
			}
		}()
	}
	s.relay(ctx, w, r, status, headers, data)
}

func firstVal(m map[string][]string, key string) string {
	if v, ok := m[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

func (s *Server) loginPage(ctx *reqCtx, errMsg string) string {
	errHTML := ""
	if errMsg != "" {
		errHTML = `<div class="err">` + html.EscapeString(errMsg) + `</div>`
	}
	return `<!doctype html>
<html lang="` + ctx.lang + `"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>` + html.EscapeString(s.DisplayName) + ` — ` + html.EscapeString(ctx.t("Mail sign-in")) + `</title>
<style>` + ctx.themeCSS(CSS) + `</style></head><body>
` + s.smallLogoHTML() + `
<div class="login"><div class="card">
  ` + s.prefsSelector(ctx) + `
  <div class="brand"><b>` + html.EscapeString(s.DisplayName) + `</b><span>` + html.EscapeString(ctx.t("OnionMail webmail")) + `</span></div>
  <p>` + html.EscapeString(ctx.t("Sign in to your mailbox on this server.")) + `</p>
  ` + errHTML + `
  <form method="post" action="/web/login">
    <label for="u">` + html.EscapeString(ctx.t("Username (main login, no @)")) + `</label>
    <input id="u" name="username" type="text" autocomplete="username" spellcheck="false" placeholder="` + html.EscapeString(ctx.t("e.g. ivan")) + `" required>
    <label for="s">` + html.EscapeString(ctx.t("SMTP password")) + `</label>
    <input id="s" name="smtp" type="password" autocomplete="off" autocapitalize="none" autocorrect="off" spellcheck="false" required>
    <label for="p">` + html.EscapeString(ctx.t("POP3 password")) + `</label>
    <input id="p" name="pop3" type="password" autocomplete="off" autocapitalize="none" autocorrect="off" spellcheck="false" required>
    <div style="margin-top:18px"><button class="btn" type="submit">` + html.EscapeString(ctx.t("Sign in")) + `</button></div>
  </form>
  <p style="margin-top:10px">` + html.EscapeString(ctx.t("Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration). The Logon/admin page uses the POP3 password.")) + `</p>
  <p style="margin-top:14px"><a href="/register.html">` + html.EscapeString(ctx.t("No account? Register with a voucher code")) + `</a></p>
</div></div></body></html>`
}

func (s *Server) pageShell(ctx *reqCtx, sess *Session, active, body string) string {
	user, addr, csrf := "", "", ""
	if sess != nil {
		user = html.EscapeString(sess.User)
		addr = html.EscapeString(sess.User) + "@" + html.EscapeString(s.Onion)
		csrf = html.EscapeString(sess.CSRF)
	}
	on := func(name string) string {
		if active == name {
			return "on"
		}
		return ""
	}
	return `<!doctype html>
<html lang="` + ctx.lang + `"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>` + html.EscapeString(s.DisplayName) + `</title>
<style>` + ctx.themeCSS(CSS) + `</style></head><body>
` + s.smallLogoHTML() + `
<div class="shell">
<aside class="sb">
  ` + s.prefsSelector(ctx) + `
  <div class="brand"><b>` + html.EscapeString(s.DisplayName) + `</b><span>` + html.EscapeString(ctx.t("OnionMail webmail")) + `</span></div>
  <nav class="nav">
    <a class="` + on("inbox") + `" href="/web/inbox">` + html.EscapeString(ctx.t("Inbox")) + `</a>
    <a class="` + on("sent") + `" href="/web/sent">` + html.EscapeString(ctx.t("Sent")) + `</a>
    <a class="` + on("compose") + `" href="/web/compose">` + html.EscapeString(ctx.t("New message")) + `</a>
  </nav>
  <div class="acc">
    <div class="who">` + user + `<br><span>` + addr + `</span></div>
    <form method="post" action="/web/logout">
      <input type="hidden" name="csrf" value="` + csrf + `">
      <button type="submit">` + html.EscapeString(ctx.t("Log out")) + `</button>
    </form>
  </div>
</aside>
<main class="main">` + body + `</main>
</div></body></html>`
}

func (s *Server) inboxPage(ctx *reqCtx, sess *Session) string {
	items, err := s.inboxItems(sess.User, sess.POP3, 300)
	if err != nil {
		body := `<div class="err">` + html.EscapeString(ctx.tf("Could not open mailbox: {err}", map[string]string{"err": err.Error()})) + `</div>`
		return s.pageShell(ctx, sess, "inbox", `<div class="top"><h1>`+html.EscapeString(ctx.t("Inbox"))+`</h1></div>`+body)
	}
	var rows strings.Builder
	if len(items) == 0 {
		rows.WriteString(`<div class="row"><div class="from">` + html.EscapeString(ctx.t("No messages")) + `</div><div class="subj"></div><div></div><div></div></div>`)
	} else {
		for _, it := range items {
			n := it["n"]
			fmt.Fprintf(&rows, `<a class="row" href="/web/message?n=%v">
<div class="from">%s</div>
<div class="subj">%s%s</div>
<div class="date">%s</div>
<div class="size">%s</div></a>`, n, html.EscapeString(fmt.Sprint(it["from"])), html.EscapeString(fmt.Sprint(it["subject"])),
				pgpBadge(ctx, fmt.Sprint(it["pgp"])), html.EscapeString(fmt.Sprint(it["date"])), humanSize(intVal(it["size"])))
		}
	}
	return s.pageShell(ctx, sess, "inbox", fmt.Sprintf(`<div class="top"><h1>%s</h1>
<span class="muted">%s</span></div><div class="list">%s</div>`, html.EscapeString(ctx.t("Inbox")),
		html.EscapeString(ctx.tf("Messages: {n}", map[string]string{"n": strconv.Itoa(len(items))})), rows.String()))
}

func pgpBadge(ctx *reqCtx, kind string) string {
	class, label := "", ""
	switch kind {
	case "keys":
		class, label = "pgp-keys", "Key exchange"
	case "encrypted":
		class, label = "pgp-encrypted", "Encrypted"
	case "signed":
		class, label = "pgp-signed", "Signed"
	default:
		return ""
	}
	return `<span class="pgp-badge ` + class + `">` + html.EscapeString(ctx.t(label)) + `</span>`
}

func intVal(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	n, _ := strconv.Atoi(fmt.Sprint(v))
	return n
}

func (s *Server) messagePage(ctx *reqCtx, sess *Session, n int) string {
	m, err := s.fetchMessage(sess.User, sess.POP3, n)
	if err != nil {
		return s.pageShell(ctx, sess, "inbox", `<div class="err">`+html.EscapeString(ctx.tf("Could not read message: {err}", map[string]string{"err": err.Error()}))+`</div>`)
	}
	atts := ""
	if list, ok := m["attachments"].([]emailAttachment); ok && len(list) > 0 {
		var b strings.Builder
		b.WriteString(`<div class="att"><div class="att-title">` + html.EscapeString(ctx.t("Attachments")) + `</div><ul class="attlist">`)
		for i, a := range list {
			fmt.Fprintf(&b, `<li><a class="btn ghost small" href="/web/attachment?n=%d&i=%d" download>%s</a></li>`,
				n, i, html.EscapeString(a.Name))
		}
		b.WriteString(`</ul></div>`)
		atts = b.String()
	}
	body := `<a class="back" href="/web/inbox">&larr; ` + html.EscapeString(ctx.t("Back")) + `</a>
<div class="card"><div class="msghead"><h2>` + html.EscapeString(fmt.Sprint(m["subject"])) + pgpBadge(ctx, fmt.Sprint(m["pgp"])) + `</h2><a class="btn ghost small" href="/web/message/download?n=` + strconv.Itoa(n) + `" download>` + html.EscapeString(ctx.t("Download message (.eml)")) + `</a></div>
<div class="meta">` + html.EscapeString(ctx.t("From")) + `: ` + html.EscapeString(fmt.Sprint(m["from"])) + `<br>` + html.EscapeString(ctx.t("To")) + `: ` + html.EscapeString(fmt.Sprint(m["to"])) + `<br>` + html.EscapeString(ctx.t("Date")) + `: ` + html.EscapeString(fmt.Sprint(m["date"])) + `</div>
<pre class="body">` + html.EscapeString(fmt.Sprint(m["body"])) + `</pre>` + atts + `</div>`
	return s.pageShell(ctx, sess, "inbox", body)
}

func (s *Server) sentPage(ctx *reqCtx, sess *Session) string {
	items := s.loadSent(sess.User)
	var rows strings.Builder
	if len(items) == 0 {
		rows.WriteString(`<div class="row"><div class="from">` + html.EscapeString(ctx.t("No sent messages")) + `</div><div class="subj"></div><div></div><div></div></div>`)
	} else {
		for _, it := range items {
			subj := fmt.Sprint(it["subject"])
			if subj == "" {
				subj = ctx.t("(no subject)")
			}
			fmt.Fprintf(&rows, `<a class="row" href="/web/sent/view?id=%s">
<div class="from">%s</div>
<div class="subj">%s</div>
<div class="date">%s</div>
<div class="size"></div></a>`, html.EscapeString(fmt.Sprint(it["id"])),
				html.EscapeString(ctx.tf("To: {to}", map[string]string{"to": fmt.Sprint(it["to"])})),
				html.EscapeString(subj), html.EscapeString(fmt.Sprint(it["date"])))
		}
	}
	return s.pageShell(ctx, sess, "sent", fmt.Sprintf(`<div class="top"><h1>%s</h1>
<span class="muted">%s</span></div><div class="list">%s</div>`, html.EscapeString(ctx.t("Sent")),
		html.EscapeString(ctx.tf("Messages: {n}", map[string]string{"n": strconv.Itoa(len(items))})), rows.String()))
}

func (s *Server) sentView(ctx *reqCtx, sess *Session, id string) string {
	for _, it := range s.loadSent(sess.User) {
		if fmt.Sprint(it["id"]) == id {
			subj := fmt.Sprint(it["subject"])
			if subj == "" {
				subj = ctx.t("(no subject)")
			}
			body := `<a class="back" href="/web/sent">&larr; ` + html.EscapeString(ctx.t("Back")) + `</a>
<div class="card"><h2>` + html.EscapeString(subj) + `</h2>
<div class="meta">` + html.EscapeString(ctx.t("To")) + `: ` + html.EscapeString(fmt.Sprint(it["to"])) + `<br>` + html.EscapeString(ctx.t("Date")) + `: ` + html.EscapeString(fmt.Sprint(it["date"])) + `</div>
<pre class="body">` + html.EscapeString(fmt.Sprint(it["body"])) + `</pre></div>`
			return s.pageShell(ctx, sess, "sent", body)
		}
	}
	return s.pageShell(ctx, sess, "sent", `<div class="err">`+html.EscapeString(ctx.t("Message not found."))+`</div>`)
}

func (s *Server) composePage(ctx *reqCtx, sess *Session, ok, errMsg string) string {
	note := ""
	if ok != "" {
		note = `<div class="ok">` + html.EscapeString(ctx.t("Message sent.")) + `</div>`
	}
	if errMsg != "" {
		note += `<div class="err">` + html.EscapeString(errMsg) + `</div>`
	}
	limit := humanSize(int(s.maxAttachmentBytes()))
	body := `<div class="top"><h1>` + html.EscapeString(ctx.t("New message")) + `</h1></div>` + note + `
<div class="card"><form method="post" action="/web/send" enctype="multipart/form-data">
<input type="hidden" name="csrf" value="` + html.EscapeString(sess.CSRF) + `">
<label for="to">` + html.EscapeString(ctx.t("To")) + `</label><input id="to" type="text" name="to" placeholder="user@some.onion" required>
<label for="su">` + html.EscapeString(ctx.t("Subject")) + `</label><input id="su" type="text" name="subject" maxlength="200">
<label for="bo">` + html.EscapeString(ctx.t("Message")) + `</label><textarea id="bo" name="body" maxlength="100000"></textarea>
<label for="at">` + html.EscapeString(ctx.t("Attachment")) + `</label><input id="at" type="file" name="attachment">
<div class="muted">` + html.EscapeString(ctx.tf("Maximum attachment size: {size}.", map[string]string{"size": limit})) + `</div>
<div style="margin-top:16px"><button class="btn" type="submit">` + html.EscapeString(ctx.t("Send")) + `</button></div>
</form></div>`
	return s.pageShell(ctx, sess, "compose", body)
}

func (s *Server) webGet(ctx *reqCtx, w http.ResponseWriter, r *http.Request, path string) {
	_, sess := s.getSession(r)
	if sess == nil {
		s.sendHTML(w, r, 200, s.loginPage(ctx, ""), nil)
		return
	}
	s.setEmptyTranslations(ctx)
	switch path {
	case "/web/inbox":
		s.sendHTML(w, r, 200, s.inboxPage(ctx, sess), nil)
	case "/web/message":
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n <= 0 {
			s.redirect(w, r, "/web/inbox", nil)
			return
		}
		s.sendHTML(w, r, 200, s.messagePage(ctx, sess, n), nil)
	case "/web/message/download":
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n <= 0 {
			s.sendHTML(w, r, 400, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("Message not found."))+`</div>`), nil)
			return
		}
		raw, err := s.fetchRawMessage(sess.User, sess.POP3, n)
		if err != nil {
			s.sendHTML(w, r, 404, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.tf("Could not read message: {err}", map[string]string{"err": err.Error()}))+`</div>`), nil)
			return
		}
		s.sendDownload(w, fmt.Sprintf("message-%d.eml", n), "message/rfc822", raw)
	case "/web/attachment":
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		idx := 0
		if v := r.URL.Query().Get("i"); v != "" {
			idx, _ = strconv.Atoi(v)
		}
		if n <= 0 || idx < 0 {
			s.sendHTML(w, r, 400, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("Message not found."))+`</div>`), nil)
			return
		}
		raw, err := s.fetchRawMessage(sess.User, sess.POP3, n)
		if err != nil {
			s.sendHTML(w, r, 404, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.tf("Could not read message: {err}", map[string]string{"err": err.Error()}))+`</div>`), nil)
			return
		}
		att, err := extractAttachment(raw, idx)
		if err != nil {
			s.sendHTML(w, r, 404, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("Attachment not found."))+`</div>`), nil)
			return
		}
		s.sendDownload(w, att.Name, att.ContentType, att.Data)
	case "/web/sent":
		s.sendHTML(w, r, 200, s.sentPage(ctx, sess), nil)
	case "/web/sent/view":
		s.sendHTML(w, r, 200, s.sentView(ctx, sess, r.URL.Query().Get("id")), nil)
	case "/web/compose":
		s.sendHTML(w, r, 200, s.composePage(ctx, sess, r.URL.Query().Get("ok"), ""), nil)
	default:
		s.sendHTML(w, r, 404, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("Not found."))+`</div>`), nil)
	}
}

func (s *Server) webPost(ctx *reqCtx, w http.ResponseWriter, r *http.Request, path string) {
	token, sess := s.getSession(r)
	if path == "/web/login" {
		f := s.form(r)
		user := strings.ToLower(strings.TrimSpace(firstVal(f, "username")))
		if i := strings.IndexByte(user, '@'); i >= 0 {
			user = user[:i]
		}
		smtp := firstVal(f, "smtp")
		pop3 := firstVal(f, "pop3")
		if !userRe.MatchString(user) || smtp == "" || pop3 == "" {
			s.sendHTML(w, r, 400, s.loginPage(ctx, ctx.t("Fill in all fields and check the username.")), nil)
			return
		}
		p, popPass, popErr := dialPOP3Any(user, pop3)
		if popErr == nil {
			p.quit()
		}
		var smtpPass string
		var smtpErr error
		if popErr == nil {
			var sm *smtpConn
			sm, smtpPass, smtpErr = dialSMTPAny(user, smtp)
			if smtpErr == nil {
				sm.quit()
			}
		}
		if popErr != nil || smtpErr != nil {
			time.Sleep(time.Second)
			stage := "pop3"
			if popErr == nil {
				stage = "smtp"
			}
			s.logf("web login failed: user=%q stage=%s", user, stage)
			if popErr != nil {
				hint := ""
				if reply, e := s.controlCommand("alias "+user, 10*time.Second); e == nil && strings.HasPrefix(reply, "+OK ") {
					mainUser := strings.ToLower(strings.TrimSpace(reply[4:]))
					if userRe.MatchString(mainUser) {
						hint = " " + ctx.tf(`It looks like “{user}” is an exit login. The main login for signing in is “{main}”.`, map[string]string{"user": user, "main": mainUser})
					}
				}
				s.sendHTML(w, r, 403, s.loginPage(ctx,
					ctx.t("POP3 did not accept this login and password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login.")+hint), nil)
				return
			}
			msg := ctx.t("POP3 accepted the login, but the SMTP password was rejected. SMTP and POP3 passwords are different: enter the SMTP password in the first password field.")
			if smtp == pop3 {
				msg = ctx.t("The SMTP and POP3 fields contain the same password. Enter the SMTP password separately — it is different from the POP3 password.")
			}
			s.sendHTML(w, r, 403, s.loginPage(ctx, msg), nil)
			return
		}
		if popPass != "" {
			pop3 = popPass
		}
		if smtpPass != "" {
			smtp = smtpPass
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.logf("registry login update failed: %v", rec)
				}
			}()
			s.registerUser(user, smtp, pop3, "", time.Now().Unix())
		}()
		token = s.newSession(user, smtp, pop3)
		ck := sessionCookie(token)
		s.redirect(w, r, "/web/inbox", [][2]string{{ck[0], ck[1]}})
		return
	}
	if sess == nil {
		s.redirect(w, r, "/web", nil)
		return
	}
	s.setEmptyTranslations(ctx)
	if path == "/web/logout" {
		f := s.form(r)
		if firstVal(f, "csrf") != sess.CSRF {
			s.sendHTML(w, r, 403, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("CSRF error."))+`</div>`), nil)
			return
		}
		s.dropSession(token)
		ck := clearCookie()
		s.redirect(w, r, "/web", [][2]string{{ck[0], ck[1]}})
		return
	}
	if path == "/web/send" {
		var f map[string][]string
		var att *outboundAttachment
		maxAtt := s.maxAttachmentBytes()
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, maxAtt+2*1024*1024)
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				if r.MultipartForm != nil {
					_ = r.MultipartForm.RemoveAll()
				}
				s.sendHTML(w, r, 413, s.composePage(ctx, sess, "", ctx.tf("Attachment upload failed. Maximum size: {size}.", map[string]string{"size": humanSize(int(maxAtt))})), nil)
				return
			}
			if r.MultipartForm != nil {
				defer r.MultipartForm.RemoveAll()
				f = r.MultipartForm.Value
			}
			file, fh, err := r.FormFile("attachment")
			if err != nil && err != http.ErrMissingFile {
				s.sendHTML(w, r, 400, s.composePage(ctx, sess, "", ctx.tf("Attachment upload failed. Maximum size: {size}.", map[string]string{"size": humanSize(int(maxAtt))})), nil)
				return
			}
			if err == nil {
				defer file.Close()
				if fh.Size > maxAtt {
					s.sendHTML(w, r, 413, s.composePage(ctx, sess, "", ctx.tf("Attachment is too large. Maximum size: {size}.", map[string]string{"size": humanSize(int(maxAtt))})), nil)
					return
				}
				att = &outboundAttachment{Name: fh.Filename, ContentType: fh.Header.Get("Content-Type"), Reader: file}
			}
		} else {
			f = s.form(r)
		}
		if firstVal(f, "csrf") != sess.CSRF {
			s.sendHTML(w, r, 403, s.composePage(ctx, sess, "", ctx.t("CSRF error.")), nil)
			return
		}
		to := strings.TrimSpace(firstVal(f, "to"))
		subject := strings.TrimSpace(firstVal(f, "subject"))
		body := firstVal(f, "body")
		if to == "" || len(to) > 320 || strings.ContainsAny(to, "\r\n") {
			s.sendHTML(w, r, 400, s.composePage(ctx, sess, "", ctx.t("Specify a valid recipient.")), nil)
			return
		}
		if strings.ContainsAny(subject, "\r\n") {
			s.sendHTML(w, r, 400, s.composePage(ctx, sess, "", ctx.t("Invalid subject.")), nil)
			return
		}
		if len(body) > 100_000 {
			s.sendHTML(w, r, 400, s.composePage(ctx, sess, "", ctx.t("Message is too large.")), nil)
			return
		}
		if err := s.smtpSendAttachment(sess.User, sess.SMTP, sess.User+"@"+s.Onion, to, subject, body, att); err != nil {
			s.logf("send error: %v", err)
			s.sendHTML(w, r, 502, s.composePage(ctx, sess, "", ctx.tf("Could not send: {err}", map[string]string{"err": err.Error()})), nil)
			return
		}
		s.addSent(sess.User, map[string]any{
			"id": randomToken(12), "to": to, "subject": subject,
			"date": time.Now().Format("2006-01-02 15:04"), "body": body,
		})
		s.redirect(w, r, "/web/sent?ok=1", nil)
		return
	}
	s.sendHTML(w, r, 404, s.pageShell(ctx, sess, "", `<div class="err">`+html.EscapeString(ctx.t("Not found."))+`</div>`), nil)
}
