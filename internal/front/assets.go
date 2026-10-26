package front

// Code generated from front.py; DO NOT EDIT.

const CSS = `
*{box-sizing:border-box}
body{margin:0;background:#eef2f6;color:#1b2733;font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
a{color:#004488;text-decoration:none}
.shell{display:flex;min-height:100vh}
.sb{width:236px;flex:0 0 236px;background:#fff;border-right:1px solid #d5dde6;display:flex;flex-direction:column;padding:18px 14px;gap:18px;position:sticky;top:0;height:100vh}
.brand{padding:4px 10px 12px;border-bottom:1px solid #e4eaf0}
.brand b{display:block;font-size:20px;letter-spacing:.2px;color:#004488}
.brand span{display:block;font-size:12px;color:#748394;margin-top:2px}
.nav{display:flex;flex-direction:column;gap:4px}
.nav a{padding:10px 12px;border-radius:8px;color:#2a3948;font-weight:600;font-size:14px}
.nav a:hover{background:#eef4fb}
.nav a.on{background:#e3edf9;color:#004488}
.acc{margin-top:auto;border-top:1px solid #e4eaf0;padding-top:12px;font-size:13px}
.acc .who{color:#425466;word-break:break-all;padding:0 4px 8px}
.acc .who span{color:#8695a6;font-size:11px}
.acc button{width:100%;background:#fff;border:1px solid #c9d5e2;color:#2a3948;border-radius:8px;padding:8px;cursor:pointer;font-size:13px}
.acc button:hover{background:#f4f8fc}
.main{flex:1;min-width:0;padding:26px 34px 60px}
.top{display:flex;align-items:baseline;justify-content:space-between;gap:12px;margin-bottom:18px}
.top h1{font-size:21px;margin:0;color:#16232f}
.muted{color:#748394;font-size:12px}
.list{background:#fff;border:1px solid #d5dde6;border-radius:12px;overflow:hidden}
.row{display:grid;grid-template-columns:230px 1fr 150px 70px;gap:12px;padding:12px 16px;border-bottom:1px solid #edf1f5;color:#233241;align-items:center}
.row:last-child{border-bottom:0}
.row:hover{background:#f7fafd}
.row .from{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.row .subj{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.row .date,.row .size{color:#7b8a9b;font-size:12px;text-align:right;white-space:nowrap}
.pgp-badge{display:inline-block;margin-left:8px;padding:2px 7px;border-radius:999px;font-size:10.5px;font-weight:700;line-height:1.6;vertical-align:1px;white-space:nowrap}
.pgp-encrypted,.pgp-signed{background:#eef1f4;color:#4b5b6b;border:1px solid #d5dde6}
.pgp-keys{background:#e7f7ec;color:#1f6b3a;border:1px solid #9fd5b4;box-shadow:0 0 0 2px #eefaf2}
.row.unread .from,.row.unread .subj{font-weight:700}
.card{background:#fff;border:1px solid #d5dde6;border-radius:12px;padding:22px;max-width:900px}
.card h2{margin:0 0 10px;font-size:19px;color:#16232f}
.meta{color:#5b6b7c;font-size:12.5px;line-height:1.7;border-bottom:1px solid #e7edf3;padding-bottom:12px;margin-bottom:16px}
pre.body{white-space:pre-wrap;word-break:break-word;font:14px/1.55 ui-monospace,SFMono-Regular,Menlo,monospace;margin:0;color:#1f2d3a}
label{display:block;font-size:12px;color:#5b6b7c;margin:12px 0 5px;font-weight:600}
input[type=text],input[type=password],textarea{width:100%;border:1px solid #c9d5e2;border-radius:8px;padding:10px 12px;font:14px system-ui;background:#fbfdff;color:#1b2733}
input[type=file]{font:14px system-ui;color:#31414f;padding:6px 0}
textarea{min-height:260px;resize:vertical}
.btn{background:#004488;color:#fff;border:0;border-radius:8px;padding:11px 18px;font-size:14px;font-weight:600;cursor:pointer}
.btn:hover{background:#0058ad}
.btn.ghost{background:#fff;color:#004488;border:1px solid #c9d5e2}
.err{background:#fdecef;border:1px solid #f2b9c4;color:#8c2334;border-radius:8px;padding:10px 12px;margin:12px 0;font-size:13px}
.ok{background:#eaf6ee;border:1px solid #b7dfc3;color:#1f6b3a;border-radius:8px;padding:10px 12px;margin:12px 0;font-size:13px}
.login{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}
.login .card{width:100%;max-width:420px}
.login .brand{border:0;padding:0 0 14px}
.login p{color:#5b6b7c;font-size:13px;margin:0 0 14px}
.att{font-size:12px;color:#5b6b7c;margin-top:14px;border-top:1px solid #e7edf3;padding-top:10px}
.att-title{font-weight:600;color:#425466;margin-bottom:7px}
.attlist{list-style:none;padding:0;margin:0;display:flex;flex-wrap:wrap;gap:8px}
.btn.small{padding:7px 11px;font-size:12.5px}
.msghead{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap}
.msghead h2{margin:0 0 10px;flex:1 1 320px}
.back{display:inline-block;margin-bottom:14px;font-size:13px}
.langbox{display:flex;align-items:center;gap:7px;flex-wrap:wrap;margin:0 0 6px}
.langbox label{font-size:10.5px;font-weight:700;color:#748394;text-transform:uppercase;letter-spacing:.08em;margin:0}
.langbox select{border:1px solid #c9d5e2;border-radius:8px;background:#fff;color:#31414f;font:600 12.5px system-ui;padding:6px 8px;cursor:pointer}
.login .om-prefs{justify-content:flex-end;margin-bottom:12px}
.om-toplogo{position:fixed;top:14px;right:18px;z-index:30;display:block;line-height:0}
.om-toplogo img{display:block;width:auto;height:34px;max-width:120px;object-fit:contain}
@media(max-width:800px){.om-toplogo{top:10px;right:12px}.om-toplogo img{height:28px}.shell{flex-direction:column}.sb{width:auto;height:auto;position:static;flex-direction:row;flex-wrap:wrap;align-items:center}.nav{flex-direction:row;flex-wrap:wrap}.acc{margin:0;border:0;padding:0}.row{grid-template-columns:1fr 90px}.row .size{display:none}.main{padding:18px}}
`

const THEME = `
html,body{background:#eef2f6!important;color:#1b2733!important;font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif!important}
body{padding:0!important;min-height:100vh}
::selection{background:#cfe2f7}
a{color:#004488!important;text-decoration:none!important}
a:hover{color:#0058ad!important}
h1,h2,h3,h4,h5,h6{color:#16232f!important;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif!important}
h5,h6,.section,.sezione{color:#16232f!important;font-weight:700!important;font-size:16px!important}
h5:before,h5:after,.section:before,.section:after,.sezione:before,.sezione:after{content:none!important}
pre{font-family:ui-monospace,SFMono-Regular,Menlo,monospace!important;color:#1f2d3a!important}
#mmpreback{background:#fff!important;background-image:none!important;border-bottom:1px solid #d5dde6!important}
#mmenucont{padding:12px 22px!important}
#toptable{height:auto!important;border-collapse:collapse!important}
#toplogo{width:auto!important;height:auto!important;padding-right:18px!important}
#toplogo img{width:60px!important;height:60px!important;object-fit:contain!important;display:block}
#topsez{height:auto!important;padding:0!important;vertical-align:middle!important}
.topsez div{height:auto!important;margin:4px 6px 4px 0!important;float:none!important;display:inline-block!important}
.topsez div a{display:inline-block!important;background:#fff!important;border:1px solid #d5dde6!important;border-radius:999px!important;padding:8px 14px!important;font:600 14px/1 system-ui!important;color:#2a3948!important}
.topsez div a:before,.topsez div a:after{content:none!important}
.topsez div a:hover{background:#e3edf9!important;color:#004488!important;border-color:#c9d5e2!important}
.secondtop div a{font:600 13px/1 system-ui!important;color:#5b6b7c!important;background:none!important;border:0!important;padding:4px!important}
.secondtop div a:before,.secondtop div a:after{content:none!important}
.oline,#bottomoline{height:0!important;background:none!important;border:0!important;border-top:1px solid #d5dde6!important;margin:0!important}
.pagetit{max-width:1000px!important;margin:28px auto 16px!important;padding:0 24px!important;text-align:left!important;font:700 26px/1.25 system-ui!important;color:#16232f!important}
.pagetit:before,.pagetit:after{content:none!important}
.mytext{max-width:1000px!important;margin:0 auto 44px!important;padding:0 24px!important;font:15px/1.55 system-ui!important;color:#1b2733!important}
.mytext p{margin:0 0 12px!important}
.mytext > table,table.extrainfotn{width:100%!important;background:#fff!important;border:1px solid #d5dde6!important;border-radius:12px!important;border-collapse:separate!important;border-spacing:0!important;overflow:hidden!important}
.mytext > table td,table.extrainfotn td{padding:11px 16px!important;border-bottom:1px solid #edf1f5!important;font-size:14px!important;text-align:left!important;vertical-align:top!important;color:#31414f!important}
.mytext > table tr:last-child td,table.extrainfotn tr:last-child td{border-bottom:0!important}
td.titleinfo{background:#f7fafd!important;color:#004488!important;font-weight:700!important;font-size:13px!important;letter-spacing:.02em}
.mytext img,.cappa img,#bottompage img{max-width:100%;height:auto}
.cappa{width:auto!important;max-width:760px!important;margin:0 auto!important;padding:22px!important;background:#fff!important;border:1px solid #d5dde6!important;border-radius:12px!important}
.cappa pre{background:#fbfdff!important;border:1px solid #e4eaf0!important;border-radius:8px!important;padding:12px!important;overflow:auto!important;line-height:1.15!important;font-size:11px!important}
.cappa div{border:0!important;padding:0!important;background:none!important}
.cappa p{text-align:left!important;font-weight:700!important;color:#16232f!important;margin:14px 0 6px!important}
.cappa p:before,.cappa p:after{content:none!important}
#minfo,.minfo{color:#748394!important;font-size:12px!important;line-height:1.5!important;padding:10px 0 0!important}
.posta{width:100%!important;border:0!important;background:none!important}
.posta td{border:0!important;padding:7px 8px 7px 0!important;font:14px system-ui!important;text-align:left!important;color:#425466!important;width:auto!important}
.posta td:first-child{width:30%!important;font-weight:600!important;color:#5b6b7c!important}
.posta input[type=text],.posta input[type=password],.posta select{width:100%!important;border:1px solid #c9d5e2!important;border-radius:8px!important;padding:10px 12px!important;background:#fbfdff!important;color:#1b2733!important;font:14px system-ui!important}
input[type=submit],input[type=button],button{background:#004488!important;color:#fff!important;border:0!important;border-radius:8px!important;padding:10px 18px!important;font:600 14px system-ui!important;cursor:pointer!important}
input[type=submit]:hover,input[type=button]:hover,button:hover{background:#0058ad!important}
.perro{max-width:900px;margin:12px auto!important;padding:10px 14px!important;border:1px solid #f2b9c4!important;border-radius:8px!important;background:#fdecef!important;color:#8c2334!important;font-size:13px!important;text-align:left!important}
.perro:empty{display:none!important}
.om-warn{background:#fdecef!important;border:1px solid #f2b9c4!important;color:#8c2334!important;border-radius:8px!important;padding:10px 12px!important;margin:16px 0!important;font-size:13px!important}
.om-note{max-width:760px!important;margin:18px auto!important;padding:12px 14px!important;background:#eef4fb!important;border:1px solid #c9d5e2!important;border-radius:8px!important;font-size:13px!important;color:#31414f!important}
#disclaim{color:#748394!important}
#aspetta{background:#eef2f6!important;color:#1b2733!important}
#aspetta b{color:#004488}
.tblcont,.tblgraf{background:#fff!important;border:1px solid #d5dde6!important;border-radius:12px!important;overflow:hidden!important}
.tbltitle{background:#f7fafd!important;color:#004488!important;font:700 13px system-ui!important;padding:10px 14px!important;border-bottom:1px solid #e7edf3!important}
.tblcapts{padding:10px 14px!important;border-bottom:1px solid #edf1f5!important;font-size:14px!important}
#bottompage{max-height:none!important;padding:18px 24px!important;color:#748394!important;font-size:12px!important;border-top:1px solid #d5dde6!important;margin:0!important;background:#fff!important}
#bottompage div{font-family:system-ui!important;color:#748394!important}
#prmenu{left:0!important;right:0!important;width:100%!important;padding:12px!important;text-align:center!important;box-sizing:border-box!important}
#prmenu div{float:none!important;display:inline-block!important;height:auto!important;margin:4px!important}
#prmenu div a{display:inline-block!important;background:#fff!important;border:1px solid #d5dde6!important;border-radius:999px!important;padding:9px 15px!important;font:600 14px/1 system-ui!important;color:#2a3948!important;box-shadow:0 1px 2px rgba(16,42,67,.06)!important}
#prmenu div a:before,#prmenu div a:after{content:none!important}
#prmenu div a:hover{background:#e3edf9!important;color:#004488!important}
#infoiaten a{font:600 13px system-ui!important;color:#004488!important}
#dwn div{font-family:system-ui!important;font-size:12px!important;color:#5b6b7c!important}
`

const DARK_EXTRA = `
input[type=submit],input[type=button],button,.btn{background:#7c3aed!important;color:#fff!important;border-color:#7c3aed!important}
input[type=submit]:hover,input[type=button]:hover,button:hover,.btn:hover{background:#9b5cff!important;border-color:#9b5cff!important}
nav a.on,.nav a.on{background:#7c3aed!important;border-color:#7c3aed!important;color:#fff!important}
.themebox img{filter:brightness(1.45) saturate(1.2)}
`

const TOGGLE_CSS = `
.om-prefs{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.om-prefs .langbox{margin:0}
.themebox{margin:0;display:flex}
.themebox button{display:flex;align-items:center;justify-content:center;width:34px;height:34px;padding:0!important;border:1px solid #c9d5e2!important;border-radius:8px!important;background:#fff!important;color:#004488!important;cursor:pointer}
.themebox button:hover{background:#f4f8fc!important}
.themebox img{width:18px;height:18px;display:block}
`

const UPSTREAM_PREFS_CSS = `
#om-prefs{position:fixed;top:10px;right:10px;z-index:2147483647;background:#fff;border:1px solid #d5dde6;border-radius:10px;padding:6px 9px;box-shadow:0 2px 10px rgba(16,42,67,.14);font:13px system-ui,sans-serif;display:flex;align-items:center;gap:8px;max-width:calc(100vw - 20px)}
#om-prefs form{display:flex;align-items:center;gap:7px;margin:0}
#om-prefs label{font:700 10px system-ui,sans-serif;color:#748394;text-transform:uppercase;letter-spacing:.08em;margin:0}
#om-prefs select{border:1px solid #c9d5e2;border-radius:7px;background:#fff;color:#31414f;font:600 12px system-ui,sans-serif;padding:5px 7px;cursor:pointer}
#om-prefs .themebox button{width:30px;height:30px;padding:0!important;border:1px solid #c9d5e2!important;border-radius:7px!important;background:#fff!important;color:#004488!important;cursor:pointer}
#om-prefs .themebox button:hover{background:#f4f8fc!important}
#om-prefs .themebox img{width:17px;height:17px;display:block}
@media(max-width:560px){#om-prefs label{display:none}}
`

const SMALL_CSS = `
body{font:15px/1.5 system-ui,sans-serif;background:#eef2f6;color:#1b2733;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.b{background:#fff;border:1px solid #d5dde6;border-radius:12px;padding:22px;max-width:520px}
h1{font-size:18px;margin:0 0 10px;color:#004488}a{color:#004488}
.err{background:#fdecef;border:1px solid #f2b9c4;color:#8c2334;border-radius:8px;padding:10px 12px;margin:10px 0;font-size:13px}
.om-prefs{display:flex;gap:7px;align-items:center;justify-content:flex-end;margin-bottom:14px}
.langbox{display:flex;align-items:center;gap:7px;margin:0}
.langbox label{font-size:10.5px;font-weight:700;color:#748394;text-transform:uppercase}
.langbox select{border:1px solid #c9d5e2;border-radius:8px;padding:6px;background:#fff;color:#31414f}
`

const ADMIN_CSS = `
*{box-sizing:border-box}
body{margin:0;background:#eef2f6;color:#1b2733;font:14px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif}
a{color:#004488;text-decoration:none}a:hover{text-decoration:underline}
.wrap{max-width:1200px;margin:0 auto;padding:20px 18px 40px}
.langbox{display:flex;align-items:center;gap:7px;flex-wrap:wrap}
.langbox label{font-size:10.5px;font-weight:700;color:#748394;text-transform:uppercase;letter-spacing:.08em;margin:0}
.langbox select{border:1px solid #c9d5e2;border-radius:8px;background:#fff;color:#31414f;font:600 12.5px system-ui;padding:6px 8px;cursor:pointer}
.topbar{display:flex;align-items:center;justify-content:space-between;gap:14px;flex-wrap:wrap;margin-bottom:18px}
.topbar .brand{order:0}.topbar nav{order:1}.topbar .om-prefs{order:2;margin-left:auto;margin-bottom:0;justify-content:flex-end}
.brand{font-size:18px;font-weight:750;color:#0d2233;line-height:1.15}
.brand span{display:block;font-size:11px;font-weight:600;color:#748394;text-transform:uppercase;letter-spacing:.08em}
nav{display:flex;gap:6px;flex-wrap:wrap}
nav a{padding:7px 12px;border:1px solid #d5dde6;border-radius:999px;background:#fff;color:#31414f;font-size:13px;font-weight:600}
nav a.on{background:#004488;border-color:#004488;color:#fff}
.alert{border-radius:10px;padding:10px 13px;margin:0 0 14px;font-size:13px}
.alert.ok{background:#eaf6ee;border:1px solid #b9dfc5;color:#23633a}
.alert.err{background:#fdecef;border:1px solid #f2b9c4;color:#8c2334}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:12px;margin-bottom:16px}
.card{background:#fff;border:1px solid #d5dde6;border-radius:12px;padding:13px 14px;min-height:86px}
.k{font-size:10.5px;font-weight:700;color:#748394;text-transform:uppercase;letter-spacing:.06em}
.v{font-size:20px;font-weight:750;color:#16232f;margin-top:5px;line-height:1.2;overflow-wrap:anywhere}
.s{font-size:11.5px;color:#748394;margin-top:3px}
.charts{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:14px;margin-bottom:16px}
.panel{background:#fff;border:1px solid #d5dde6;border-radius:12px;padding:15px 16px;margin-bottom:16px;min-width:0}
.panel h2{font-size:14px;margin:0 0 12px;color:#0d2233;letter-spacing:.01em}
.panel .hint{font-size:12px;color:#748394;margin:-6px 0 12px}
.chart{display:flex;gap:16px;align-items:center;flex-wrap:wrap}
.donut{flex:0 0 190px}
.legend{flex:1 1 170px;min-width:160px}
.lg{display:flex;align-items:center;gap:8px;padding:4px 0;font-size:12.5px;border-bottom:1px dashed #eef1f5}
.lg:last-child{border-bottom:0}
.dot{width:9px;height:9px;border-radius:3px;flex:0 0 9px}
.lg .nm{flex:1;color:#31414f;overflow-wrap:anywhere}
.lg b{font-weight:700;white-space:nowrap}
.lg .pc{color:#748394;width:48px;text-align:right;white-space:nowrap}
.empty{padding:18px 8px;color:#748394;font-size:13px;text-align:center}
table{width:100%;border-collapse:collapse;font-size:12.5px}
th{text-align:left;font-size:10.5px;text-transform:uppercase;letter-spacing:.05em;color:#748394;padding:8px 9px;border-bottom:1px solid #dfe5eb;white-space:nowrap}
td{padding:9px;border-bottom:1px solid #eef1f5;vertical-align:middle}
tr:last-child td{border-bottom:0}
td.num{text-align:right;white-space:nowrap;font-variant-numeric:tabular-nums}
.login{font-weight:700;color:#16232f}
.mut{color:#748394}
.badge{display:inline-block;padding:2px 7px;border-radius:999px;font-size:10.5px;font-weight:700}
.badge.ok{background:#eaf6ee;color:#23633a}
.badge.warn{background:#fff4e5;color:#8a5a12}
.badge.err{background:#fdecef;color:#8c2334}
.tblscroll{overflow-x:auto}
form.add{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px;align-items:end}
label{display:block;font-size:10.5px;font-weight:700;color:#5f7182;text-transform:uppercase;letter-spacing:.05em;margin-bottom:4px}
input{width:100%;padding:8px 10px;border:1px solid #cfd8e2;border-radius:8px;background:#fff;color:#1b2733;font:13px system-ui,sans-serif}
input:focus{outline:2px solid #bcd3ea;border-color:#7ba7cf}
button{cursor:pointer;border:0;border-radius:8px;padding:9px 15px;background:#004488;color:#fff;font:600 13px system-ui,sans-serif}
button:hover{background:#00366b}
.foot{color:#748394;font-size:11.5px;margin-top:18px;line-height:1.55}
@media(max-width:620px){.wrap{padding:14px 10px 30px}.donut{flex-basis:auto;margin:0 auto}.cards{grid-template-columns:repeat(auto-fit,minmax(145px,1fr))}}
`

const SERVER_CSS = `
.ranges{display:flex;gap:6px;flex-wrap:wrap;margin:0 0 14px}
.ranges a{padding:5px 10px;border:1px solid #d5dde6;border-radius:999px;background:#fff;color:#31414f;font-size:12px;font-weight:600}
.ranges a:hover{text-decoration:none;background:#f4f8fc}
.ranges a.on{background:#004488;border-color:#004488;color:#fff}
.dchart{width:100%;height:auto;display:block;overflow:visible}
.chartlegend{display:flex;gap:14px;flex-wrap:wrap;margin-top:10px}
.chartlegend span{display:inline-flex;gap:6px;align-items:center;font-size:12px;color:#5b6b7c}
.chartlegend i{width:9px;height:9px;border-radius:3px;display:inline-block}
.two{display:grid;grid-template-columns:minmax(0,1.8fr) minmax(280px,1fr);gap:14px}
@media(max-width:900px){.two{grid-template-columns:1fr}}
`

const C_RECV = "#004488"
const C_SENT = "#2f7fd1"
const C_USER = "#7fb2e5"
const C_SERVICE = "#c9d5e2"
const C_IN = "#004488"
const C_OUT = "#e08a3c"

var darkMap = map[string]string{
	"#eef2f6": "#1a1025",
	"#ffffff": "#241631",
	"#fff":    "#241631",
	"#f7fafd": "#2b1a3d",
	"#fbfdff": "#2b1a3d",
	"#f4f8fc": "#322046",
	"#eef4fb": "#2f1d42",
	"#e3edf9": "#3b2459",
	"#cfe2f7": "#5b3a80",
	"#e7edf3": "#3a2850",
	"#e4eaf0": "#3a2850",
	"#edf1f5": "#332145",
	"#dfe5eb": "#3a2850",
	"#d5dde6": "#4a3563",
	"#cfd8e2": "#4a3563",
	"#c9d5e2": "#4a3563",
	"#bcd3ea": "#6d4a99",
	"#7ba7cf": "#9d7ad1",
	"#16232f": "#f4ecff",
	"#1b2733": "#f4ecff",
	"#1f2d3a": "#f4ecff",
	"#0d2233": "#f4ecff",
	"#233241": "#e6daf5",
	"#2a3948": "#e6daf5",
	"#31414f": "#e6daf5",
	"#425466": "#d9cbea",
	"#5b6b7c": "#c3b3d9",
	"#5f7182": "#c3b3d9",
	"#748394": "#a99bc0",
	"#7b8a9b": "#a99bc0",
	"#8695a6": "#a99bc0",
	"#004488": "#b794ff",
	"#0058ad": "#d0b5ff",
	"#00366b": "#7c3aed",
	"#eaf6ee": "#1e3b2d",
	"#b7dfc3": "#2f6b4a",
	"#b9dfc5": "#2f6b4a",
	"#1f6b3a": "#8ee0ad",
	"#23633a": "#8ee0ad",
	"#fdecef": "#3d1c2b",
	"#f2b9c4": "#7a3b50",
	"#8c2334": "#ff9fb4",
	"#fff4e5": "#3d2c16",
	"#8a5a12": "#ffc46b",
	"#e08a3c": "#d99a5c",
}
