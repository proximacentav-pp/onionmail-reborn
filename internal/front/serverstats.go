package front

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const serverSampleEvery = 15 * time.Minute

type serverSample struct {
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

func (s *Server) serverHistoryPath() string {
	return filepath.Join(filepath.Dir(s.Registry), "server-history.jsonl")
}

func (s *Server) statsUpdatePath() string {
	return filepath.Join(filepath.Dir(s.Registry), ".stats-update")
}

func (s *Server) serverStatsLoop() {
	s.maybeWriteServerSample()
	last := time.Now()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for now := range t.C {
		if _, err := os.Stat(s.statsUpdatePath()); err == nil {
			_ = os.Remove(s.statsUpdatePath())
			s.writeServerSample()
			last = now
			continue
		}
		if now.Sub(last) >= serverSampleEvery {
			s.writeServerSample()
			last = now
		}
	}
}

func (s *Server) maybeWriteServerSample() {
	if last := s.lastServerSample(); last == 0 || time.Since(time.Unix(last, 0)) >= serverSampleEvery {
		s.writeServerSample()
	}
}

func (s *Server) lastServerSample() int64 {
	data, err := os.ReadFile(s.serverHistoryPath())
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var sm serverSample
		if json.Unmarshal([]byte(lines[i]), &sm) == nil && sm.T > 0 {
			return sm.T
		}
	}
	return 0
}

func (s *Server) readServerHistory() []serverSample {
	data, err := os.ReadFile(s.serverHistoryPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	out := make([]serverSample, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var sm serverSample
		if json.Unmarshal([]byte(line), &sm) == nil && sm.T > 0 {
			out = append(out, sm)
		}
	}
	return out
}

func (s *Server) writeServerSample() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	recv, sent, service, other := s.diskCategories()
	s.httpMu.Lock()
	h2, h403, h4, h5 := s.http2, s.http403, s.http4, s.http5
	s.http2, s.http403, s.http4, s.http5 = 0, 0, 0, 0
	s.httpMu.Unlock()
	sm := serverSample{
		T: time.Now().Unix(), Recv: recv, Sent: sent, Service: service, Other: other,
		H2: h2, H403: h403, H4: h4, H5: h5,
	}
	path := s.serverHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		s.logf("server stats: %v", err)
		return
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 8<<20 {
		s.rotateServerHistory(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		s.logf("server stats: %v", err)
		return
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(sm); err != nil {
		s.logf("server stats: %v", err)
	}
}

func (s *Server) rotateServerHistory(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	const keep = 20000
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	tmp := path + ".rot"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return
	}
	_ = os.Chmod(tmp, 0o600)
	_ = os.Rename(tmp, path)
}

func (s *Server) diskCategories() (recv, sent, service, other int64) {
	recv = dirSize(s.MailDir + "/inbox")
	sent = dirSize(s.SentDir)
	rootfs := duBytes("/")
	libAll := dirSize("/var/lib/onionmail")
	var registry int64
	if fi, err := os.Stat(s.Registry); err == nil {
		registry = fi.Size()
	}
	libService := libAll - sent - registry
	if libService < 0 {
		libService = 0
	}
	etcAll := dirSize("/etc/onionmail")
	logAll := dirSize("/var/log/onionmail")
	service = rootfs + libService + etcAll + logAll + dirSize(s.MailDir+"/log")
	total := rootfs + dirSize(s.MailDir) + libAll + etcAll + logAll + dirSize("/var/lib/tor")
	other = total - recv - sent - service
	if other < 0 {
		other = 0
	}
	return
}

var duCache struct {
	mu    sync.Mutex
	at    time.Time
	bytes int64
}

func duBytes(path string) int64 {
	duCache.mu.Lock()
	if time.Since(duCache.at) < time.Minute {
		v := duCache.bytes
		duCache.mu.Unlock()
		return v
	}
	duCache.mu.Unlock()
	out, _ := exec.Command("du", "-s", "-x", "-B1", path).Output()
	fields := strings.Fields(string(out))
	var n int64
	if len(fields) > 0 {
		n, _ = strconv.ParseInt(fields[0], 10, 64)
	}
	duCache.mu.Lock()
	duCache.at = time.Now()
	duCache.bytes = n
	duCache.mu.Unlock()
	return n
}

type serverRange struct {
	Key   string
	Label string
	Dur   time.Duration
}

var serverRanges = []serverRange{
	{"all", "All time", 0},
	{"1y", "1 year", 365 * 24 * time.Hour},
	{"90d", "90 days", 90 * 24 * time.Hour},
	{"30d", "30 days", 30 * 24 * time.Hour},
	{"7d", "7 days", 7 * 24 * time.Hour},
	{"5d", "5 days", 5 * 24 * time.Hour},
	{"3d", "3 days", 3 * 24 * time.Hour},
	{"1d", "1 day", 24 * time.Hour},
	{"5h", "5 hours", 5 * time.Hour},
	{"1h", "1 hour", time.Hour},
	{"15m", "15 minutes", 15 * time.Minute},
}

type aggSample struct {
	T       int64
	Recv    int64
	Sent    int64
	Service int64
	Other   int64
	H2      int64
	H403    int64
	H4      int64
	H5      int64
}

func parseServerRange(key string) serverRange {
	for _, r := range serverRanges {
		if r.Key == key {
			return r
		}
	}
	return serverRanges[7]
}

func pickBucket(dur, span time.Duration) int64 {
	if dur <= 0 {
		dur = span
	}
	if dur <= 0 {
		return 900
	}
	for _, c := range []int64{900, 3600, 6 * 3600, 24 * 3600, 7 * 24 * 3600, 30 * 24 * 3600} {
		if int64(dur/time.Second) <= c*120 {
			return c
		}
	}
	return 30 * 24 * 3600
}

func aggregateServerSamples(samples []serverSample, r serverRange) ([]aggSample, int64) {
	cutoff := int64(0)
	if r.Dur > 0 {
		cutoff = time.Now().Add(-r.Dur).Unix()
	}
	var filtered []serverSample
	for _, sm := range samples {
		if cutoff == 0 || sm.T >= cutoff {
			filtered = append(filtered, sm)
		}
	}
	if len(filtered) == 0 {
		if r.Dur > 0 && r.Dur <= 15*time.Minute && len(samples) > 0 {
			filtered = []serverSample{samples[len(samples)-1]}
		} else {
			return nil, 0
		}
	}
	span := time.Duration(filtered[len(filtered)-1].T-filtered[0].T) * time.Second
	bucket := pickBucket(r.Dur, span)
	var out []aggSample
	idx := map[int64]int{}
	for _, sm := range filtered {
		b := sm.T - sm.T%bucket
		if i, ok := idx[b]; ok {
			a := &out[i]
			a.H2 += sm.H2
			a.H403 += sm.H403
			a.H4 += sm.H4
			a.H5 += sm.H5
			a.Recv, a.Sent, a.Service, a.Other = sm.Recv, sm.Sent, sm.Service, sm.Other
			continue
		}
		idx[b] = len(out)
		out = append(out, aggSample{
			T: b, Recv: sm.Recv, Sent: sm.Sent, Service: sm.Service, Other: sm.Other,
			H2: sm.H2, H403: sm.H403, H4: sm.H4, H5: sm.H5,
		})
	}
	return out, bucket
}

type stackPart struct {
	Name  string
	Color string
	Value int64
}

type stackBar struct {
	Label string
	Title string
	Parts []stackPart
	Total int64
}

func stackedSVG(bars []stackBar, dark bool, valueFmt func(int64) string) string {
	if len(bars) == 0 {
		return `<div class="empty">No samples in the selected period.</div>`
	}
	var max int64
	for _, b := range bars {
		if b.Total > max {
			max = b.Total
		}
	}
	if max <= 0 {
		max = 1
	}
	const w, h = 1200, 380
	const ml, mr, mt, mb = 82, 18, 22, 52
	plotW := float64(w - ml - mr)
	plotH := float64(h - mt - mb)
	slot := plotW / float64(len(bars))
	barW := slot * 0.78
	if barW < 1 {
		barW = 1
	}
	if barW > 54 {
		barW = 54
	}
	grid := "#e7edf3"
	axis := "#748394"
	text := "#5b6b7c"
	if dark {
		grid, axis, text = "#3a2850", "#a99bc0", "#c3b3d9"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="dchart" viewBox="0 0 %d %d" role="img" xmlns="http://www.w3.org/2000/svg">`, w, h)
	for i := 0; i <= 4; i++ {
		y := float64(mt) + plotH*float64(i)/4
		val := max - max*int64(i)/4
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="%s" stroke-width="1"/>`, ml, y, w-mr, y, grid)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" text-anchor="end" font-size="11" fill="%s">%s</text>`, ml-8, y+4, axis, html.EscapeString(valueFmt(val)))
	}
	xStep := len(bars) / 8
	if xStep < 1 {
		xStep = 1
	}
	for i, bar := range bars {
		cx := float64(ml) + slot*float64(i) + slot/2
		y := float64(mt) + plotH
		title := bar.Label
		if bar.Title != "" {
			title += " · " + bar.Title
		}
		fmt.Fprintf(&b, `<g><title>%s</title>`, html.EscapeString(title))
		for _, p := range bar.Parts {
			if p.Value <= 0 {
				continue
			}
			ph := plotH * float64(p.Value) / float64(max)
			if ph < 0.7 {
				ph = 0.7
			}
			y -= ph
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="1.5"><title>%s: %s</title></rect>`,
				cx-barW/2, y, barW, ph, p.Color, html.EscapeString(p.Name), html.EscapeString(valueFmt(p.Value)))
		}
		b.WriteString(`</g>`)
		if len(bars) <= 18 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="9.5" fill="%s">%s</text>`,
				cx, y-4, text, html.EscapeString(valueFmt(bar.Total)))
		}
		if i%xStep == 0 || i == len(bars)-1 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%d" text-anchor="middle" font-size="10" fill="%s">%s</text>`,
				cx, h-22, axis, html.EscapeString(bar.Label))
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func chartLegend(parts []stackPart) string {
	var b strings.Builder
	b.WriteString(`<div class="chartlegend">`)
	for _, p := range parts {
		fmt.Fprintf(&b, `<span><i style="background:%s"></i>%s</span>`, p.Color, html.EscapeString(p.Name))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (s *Server) adminServer(ctx *reqCtx, w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		s.redirect(w, r, "/logon.html", nil)
		return
	}
	if r.URL.Query().Get("refresh") != "" {
		s.writeServerSample()
	}
	page := s.renderServerPage(ctx, parseServerRange(r.URL.Query().Get("range")), s.readServerHistory())
	s.sendHTML(w, r, 200, page, nil)
}

func (s *Server) renderServerPage(ctx *reqCtx, sel serverRange, samples []serverSample) string {
	agg, bucket := aggregateServerSamples(samples, sel)
	dark := ctx.theme == "dark"
	recvC, sentC, _, serviceC, _, otherC, _, _ := chartPalette(dark)
	if dark {
		otherC = "#6d4ac9"
	} else {
		otherC = "#8c9bab"
	}
	diskParts := []stackPart{
		{ctx.t("Received mail"), recvC, 0},
		{ctx.t("Sent mail"), sentC, 0},
		{ctx.t("Service files"), serviceC, 0},
		{ctx.t("Other"), otherC, 0},
	}
	var diskBars []stackBar
	var recvTotal, sentTotal, serviceTotal, otherTotal int64
	var lastDisk int64
	for _, a := range agg {
		recvTotal += a.Recv
		sentTotal += a.Sent
		serviceTotal += a.Service
		otherTotal += a.Other
		lastDisk = a.Recv + a.Sent + a.Service + a.Other
		bar := stackBar{Label: time.Unix(a.T, 0).Format("02.01 15:04")}
		bar.Parts = []stackPart{
			{diskParts[0].Name, recvC, a.Recv},
			{diskParts[1].Name, sentC, a.Sent},
			{diskParts[2].Name, serviceC, a.Service},
			{diskParts[3].Name, otherC, a.Other},
		}
		bar.Total = a.Recv + a.Sent + a.Service + a.Other
		bar.Title = fmt.Sprintf("%s: %s · %s: %s · %s: %s · %s: %s",
			diskParts[0].Name, fmtSize(a.Recv), diskParts[1].Name, fmtSize(a.Sent),
			diskParts[2].Name, fmtSize(a.Service), diskParts[3].Name, fmtSize(a.Other))
		diskBars = append(diskBars, bar)
	}
	h2C, h403C, h4C, h5C := "#2f9e5f", "#e0a03c", "#d97706", "#c0392b"
	if dark {
		h2C, h403C, h4C, h5C = "#4ade80", "#fbbf24", "#fb923c", "#f87171"
	}
	httpParts := []stackPart{
		{"2xx", h2C, 0}, {"403", h403C, 0}, {"4xx", h4C, 0}, {"5xx", h5C, 0},
	}
	var httpBars []stackBar
	var t2, t403, t4, t5 int64
	for _, a := range agg {
		t2 += a.H2
		t403 += a.H403
		t4 += a.H4
		t5 += a.H5
		bar := stackBar{Label: time.Unix(a.T, 0).Format("02.01 15:04")}
		bar.Parts = []stackPart{
			{httpParts[0].Name, h2C, a.H2},
			{httpParts[1].Name, h403C, a.H403},
			{httpParts[2].Name, h4C, a.H4},
			{httpParts[3].Name, h5C, a.H5},
		}
		bar.Total = a.H2 + a.H403 + a.H4 + a.H5
		bar.Title = fmt.Sprintf("2xx: %s · 403: %s · 4xx: %s · 5xx: %s", fmtInt(a.H2), fmtInt(a.H403), fmtInt(a.H4), fmtInt(a.H5))
		httpBars = append(httpBars, bar)
	}
	httpSegments := []segment{
		{"2xx", t2, h2C}, {"403", t403, h403C}, {"4xx", t4, h4C}, {"5xx", t5, h5C},
	}
	var ranges strings.Builder
	for _, rr := range serverRanges {
		cls := "range"
		if rr.Key == sel.Key {
			cls += " on"
		}
		fmt.Fprintf(&ranges, `<a class="%s" href="/admin/server.html?range=%s">%s</a>`, cls, rr.Key, html.EscapeString(ctx.t(rr.Label)))
	}
	bucketLabel := "15 min"
	if bucket > 900 {
		bucketLabel = fmtDuration(int(bucket))
	}
	emptyHistory := len(samples) == 0
	historyHint := ctx.tf("15-minute samples · bucket: {b} · stored in {path}", map[string]string{"b": bucketLabel, "path": s.serverHistoryPath()})
	if emptyHistory {
		historyHint = ctx.tf("No history yet. Samples are written every 15 minutes to {path}.", map[string]string{"path": s.serverHistoryPath()})
	}
	returnPage := `<!doctype html>
<html lang="` + ctx.lang + `"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>` + html.EscapeString(s.DisplayName) + ` · server</title>
<style>` + ctx.themeCSS(ADMIN_CSS+SERVER_CSS) + `</style></head><body><div class="wrap">
<div class="topbar">` + s.prefsSelector(ctx) + `<div class="brand">` + html.EscapeString(s.DisplayName) + `<span>OnionMail · server</span></div>
<nav><a href="/admin/index.html">` + html.EscapeString(ctx.t("Statistics")) + `</a>
<a class="on" href="/admin/server.html">` + html.EscapeString(ctx.t("Server")) + `</a>
<a href="/admin/elist.html">` + html.EscapeString(ctx.t("Net")) + `</a>
<a href="/admin/voucher.html">` + html.EscapeString(ctx.t("Vouchers")) + `</a>
<a class="refresh" href="/admin/server.html?range=` + html.EscapeString(sel.Key) + `&refresh=1" title="` + html.EscapeString(ctx.t("Refresh")) + `">↻ ` + html.EscapeString(ctx.t("Refresh")) + `</a>
<a href="/admin/logout.html">` + html.EscapeString(ctx.t("Log out")) + `</a></nav></div>
<div class="cards">` +
		adminCard(ctx.t("Requests"), fmtInt(t2+t403+t4+t5), ctx.tf("for the selected period: {range}", map[string]string{"range": ctx.t(sel.Label)})) +
		adminCard("2xx", fmtInt(t2), ctx.t("successful responses")) +
		adminCard("403", fmtInt(t403), ctx.t("forbidden, shown separately")) +
		adminCard("4xx", fmtInt(t4), ctx.t("client errors except 403")) +
		adminCard("5xx", fmtInt(t5), ctx.t("server errors")) +
		adminCard(ctx.t("Disk used"), fmtSize(lastDisk), ctx.t("latest 15-minute sample")) +
		`</div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("Disk usage")) + `</h2>
<div class="hint">` + html.EscapeString(historyHint) + `</div>
<div class="ranges">` + ranges.String() + `</div>` +
		stackedSVG(diskBars, dark, fmtSize) +
		chartLegend([]stackPart{
			{ctx.t("Received mail"), recvC, recvTotal},
			{ctx.t("Sent mail"), sentC, sentTotal},
			{ctx.t("Service files"), serviceC, serviceTotal},
			{ctx.t("Other"), otherC, otherTotal},
		}) + `
</div>
<div class="two">
<div class="panel"><h2>` + html.EscapeString(ctx.t("HTTP statuses")) + `</h2>
<div class="hint">` + html.EscapeString(ctx.t("403 is shown separately from other 4xx errors. Stacked bars per time bucket.")) + ` ` + html.EscapeString(ctx.t("Click Refresh to record the current counters immediately.")) + `</div>` +
		stackedSVG(httpBars, dark, func(v int64) string { return fmtInt(v) }) +
		chartLegend(httpParts) + `
</div>
<div class="panel"><h2>` + html.EscapeString(ctx.t("HTTP status share")) + `</h2>
<div class="hint">` + html.EscapeString(ctx.t("Same period, share of responses.")) + `</div>` +
		donutSVG(httpSegments, fmtInt(t2+t403+t4+t5), ctx.t("requests"), dark, func(v int64) string { return fmtInt(v) }) + `
</div>
</div>
<div class="foot">` + html.EscapeString(ctx.tf("Data source: front HTTP responses (health checks and theme assets excluded). History starts when this build is deployed.", nil)) + `</div>
</div></body></html>`
	return returnPage
}

func fmtDuration(sec int) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm", sec/60)
	}
	if sec < 86400 {
		return fmt.Sprintf("%dh", sec/3600)
	}
	return fmt.Sprintf("%dd", sec/86400)
}
