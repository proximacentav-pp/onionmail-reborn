#!/usr/bin/env python3
import base64
import datetime
import glob
import json
import os
import re
import shlex
import shutil
import signal
import socket
import subprocess
import sys
import time
try:
    import curses
except Exception:
    curses = None

STATE_PATH = "/etc/onionmail/om.json"
SERVERS_DIR = "/etc/onionmail/servers.d"
CORE_CONF = "/etc/onionmail/core.conf"
HTTP_CONF = "/etc/onionmail/http.conf"
POP3_CONF = "/etc/onionmail/pop3.conf"
CONTROL_CONF = "/etc/onionmail/control.conf"
TORRC = "/etc/tor/torrc"
TOR_LOG = "/var/log/onionmail/tor.log"
SERVICE_PASS = "/etc/onionmail/.service_pass"
WWWROOT = "/var/lib/onionmail/wwwroot"
VOUCHER_LOG = "/var/log/onionmail/vouchers-tui.log"
DEFAULT_LOGO = "/img/OM-BL.gif"
LOGO_MAX_BYTES = 5 * 1024 * 1024
LANGS = ("ru", "de", "fr", "en", "es")

T = {
"Choose language": {"ru":"Выберите язык","de":"Sprache wählen","fr":"Choisir la langue","en":"Choose language","es":"Elegir idioma"},
"OnionMail Control": {"ru":"Управление OnionMail","de":"OnionMail Steuerung","fr":"Contrôle OnionMail","en":"OnionMail Control","es":"Control de OnionMail"},
"1. Main": {"ru":"1. Основные","de":"1. Allgemein","fr":"1. Général","en":"1. Main","es":"1. General"},
"2. Admin access": {"ru":"2. Админ-доступ","de":"2. Admin-Zugang","fr":"2. Accès admin","en":"2. Admin access","es":"2. Acceso admin"},
"3. Limits": {"ru":"3. Лимиты","de":"3. Limits","fr":"3. Limites","en":"3. Limits","es":"3. Límites"},
"4. Server friends": {"ru":"4. Друзья сервера","de":"4. Server-Freunde","fr":"4. Amis du serveur","en":"4. Server friends","es":"4. Amigos del servidor"},
"5. Registration": {"ru":"5. Регистрация","de":"5. Registrierung","fr":"5. Inscription","en":"5. Registration","es":"5. Registro"},
"6. Tor": {"ru":"6. Tor","de":"6. Tor","fr":"6. Tor","en":"6. Tor","es":"6. Tor"},
"7. Design": {"ru":"7. Дизайн","de":"7. Design","fr":"7. Design","en":"7. Design","es":"7. Diseño"},
"Default theme": {"ru":"Тема по умолчанию","de":"Standarddesign","fr":"Thème par défaut","en":"Default theme","es":"Tema predeterminado"},
"Logo": {"ru":"Логотип","de":"Logo","fr":"Logo","en":"Logo","es":"Logotipo"},
"Add logo": {"ru":"Добавить логотип","de":"Logo hinzufügen","fr":"Ajouter un logo","en":"Add logo","es":"Añadir logotipo"},
"Reset logo": {"ru":"Сбросить логотип","de":"Logo zurücksetzen","fr":"Réinitialiser le logo","en":"Reset logo","es":"Restablecer logotipo"},
"Logo path": {"ru":"Путь к логотипу","de":"Pfad zum Logo","fr":"Chemin du logo","en":"Logo path","es":"Ruta del logotipo"},
"Logo updated.": {"ru":"Логотип обновлён.","de":"Logo aktualisiert.","fr":"Logo mis à jour.","en":"Logo updated.","es":"Logotipo actualizado."},
"Logo reset.": {"ru":"Логотип сброшен.","de":"Logo zurückgesetzt.","fr":"Logo réinitialisé.","en":"Logo reset.","es":"Logotipo restablecido."},
"Custom": {"ru":"Свой","de":"Eigenes","fr":"Personnalisé","en":"Custom","es":"Personalizado"},
"Default": {"ru":"Стандартный","de":"Standard","fr":"Par défaut","en":"Default","es":"Predeterminado"},
"Image not found": {"ru":"Файл не найден","de":"Datei nicht gefunden","fr":"Fichier introuvable","en":"Image not found","es":"Archivo no encontrado"},
"Unsupported image format (PNG/JPG only).": {"ru":"Нужен PNG или JPG.","de":"Nur PNG/JPG wird unterstützt.","fr":"Format PNG/JPG uniquement.","en":"Unsupported image format (PNG/JPG only).","es":"Formato no admitido (solo PNG/JPG)."},
"Image is too large (max 5 MB).": {"ru":"Файл слишком большой (макс. 5 МБ).","de":"Bild ist zu groß (max. 5 MB).","fr":"Image trop volumineuse (max. 5 Mo).","en":"Image is too large (max 5 MB).","es":"Imagen demasiado grande (máx. 5 MB)."},
"Light": {"ru":"Светлая","de":"Hell","fr":"Clair","en":"Light","es":"Claro"},
"Dark": {"ru":"Тёмная","de":"Dunkel","fr":"Sombre","en":"Dark","es":"Oscuro"},
"Server name": {"ru":"Название сервера","de":"Servername","fr":"Nom du serveur","en":"Server name","es":"Nombre del servidor"},
"Onion address": {"ru":"Onion-адрес","de":"Onion-Adresse","fr":"Adresse Onion","en":"Onion address","es":"Dirección Onion"},
"Get from daemon": {"ru":"Получить у демона","de":"Vom Daemon holen","fr":"Obtenir du démon","en":"Get from daemon","es":"Obtener del demonio"},
"Hidden name (NICK)": {"ru":"Скрытое имя (NICK)","de":"Versteckter Name (NICK)","fr":"Nom caché (NICK)","en":"Hidden name (NICK)","es":"Nombre oculto (NICK)"},
"Encoding (LANG)": {"ru":"Кодировка (LANG)","de":"Kodierung (LANG)","fr":"Encodage (LANG)","en":"Encoding (LANG)","es":"Codificación (LANG)"},
"Relay mode": {"ru":"Relay-режим","de":"Relay-Modus","fr":"Mode relais","en":"Relay mode","es":"Modo relay"},
"Admin login": {"ru":"Логин админа","de":"Admin-Login","fr":"Login admin","en":"Admin login","es":"Login admin"},
"Admin password": {"ru":"Пароль админа","de":"Admin-Passwort","fr":"Mot de passe admin","en":"Admin password","es":"Contraseña admin"},
"Admin panel": {"ru":"Админ-панель","de":"Admin-Panel","fr":"Panneau admin","en":"Admin panel","es":"Panel admin"},
"Admin CAPTCHA": {"ru":"CAPTCHA админа","de":"Admin-CAPTCHA","fr":"CAPTCHA admin","en":"Admin CAPTCHA","es":"CAPTCHA admin"},
"Max messages in inbox": {"ru":"Макс. писем в ящике","de":"Max. Nachrichten im Postfach","fr":"Messages max. dans la boîte","en":"Max messages in inbox","es":"Mensajes máx. en buzón"},
"Max message size (MB)": {"ru":"Макс. размер письма (МБ)","de":"Max. Nachrichtengröße (MB)","fr":"Taille max. message (Mo)","en":"Max message size (MB)","es":"Tamaño máx. mensaje (MB)"},
"Mailbox size (MB)": {"ru":"Размер ящика (МБ)","de":"Postfachgröße (MB)","fr":"Taille de la boîte (Mo)","en":"Mailbox size (MB)","es":"Tamaño del buzón (MB)"},
"Max mailing list size": {"ru":"Макс. размер рассылки","de":"Max. Mailinglistengröße","fr":"Taille max. liste de diffusion","en":"Max mailing list size","es":"Tamaño máx. lista de correo"},
"Max multi delivery addresses": {"ru":"Макс. адресов мультидоставки","de":"Max. Multidelivery-Adressen","fr":"Adresses multi-diffusion max.","en":"Max multi delivery addresses","es":"Direcciones multi-entrega máx."},
"Max spam list size": {"ru":"Макс. размер спам-листа","de":"Max. Spamlistengröße","fr":"Taille max. liste spam","en":"Max spam list size","es":"Tamaño máx. lista spam"},
"Debug mode": {"ru":"Режим отладки","de":"Debug-Modus","fr":"Mode débogage","en":"Debug mode","es":"Modo depuración"},
"Delete read messages": {"ru":"Удалять прочитанные","de":"Gelesene löschen","fr":"Supprimer les lus","en":"Delete read messages","es":"Eliminar leídos"},
"EXIT/ENTER server": {"ru":"Сервер EXIT/ENTER","de":"EXIT/ENTER-Server","fr":"Serveur EXIT/ENTER","en":"EXIT/ENTER server","es":"Servidor EXIT/ENTER"},
"Log voucher codes": {"ru":"Логировать ваучеры","de":"Voucher-Codes protokollieren","fr":"Journaliser les vouchers","en":"Log voucher codes","es":"Registrar códigos voucher"},
"Add friend": {"ru":"Добавить друга","de":"Freund hinzufügen","fr":"Ajouter un ami","en":"Add friend","es":"Añadir amigo"},
"Friend": {"ru":"Друг","de":"Freund","fr":"Ami","en":"Friend","es":"Amigo"},
"Delete": {"ru":"Удалить","de":"Löschen","fr":"Supprimer","en":"Delete","es":"Eliminar"},
"Registration CAPTCHA": {"ru":"CAPTCHA регистрации","de":"Registrierungs-CAPTCHA","fr":"CAPTCHA inscription","en":"Registration CAPTCHA","es":"CAPTCHA de registro"},
"Require vouchers": {"ru":"Обязательные ваучеры","de":"Voucher erforderlich","fr":"Vouchers obligatoires","en":"Require vouchers","es":"Vouchers obligatorios"},
"Voucher manager": {"ru":"Менеджер ваучеров","de":"Voucher-Verwaltung","fr":"Gestion des vouchers","en":"Voucher manager","es":"Gestor de vouchers"},
"Exit name field": {"ru":"Поле exit.name","de":"Feld exit.name","fr":"Champ exit.name","en":"Exit name field","es":"Campo exit.name"},
"Exit connection": {"ru":"Подключение к exit","de":"Exit-Verbindung","fr":"Connexion exit","en":"Exit connection","es":"Conexión exit"},
"Exit name": {"ru":"Имя exit","de":"Exit-Name","fr":"Nom exit","en":"Exit name","es":"Nombre exit"},
"Tor mode": {"ru":"Режим Tor","de":"Tor-Modus","fr":"Mode Tor","en":"Tor mode","es":"Modo Tor"},
"External Tor": {"ru":"Внешний Tor","de":"Externer Tor","fr":"Tor externe","en":"External Tor","es":"Tor externo"},
"Internal Tor": {"ru":"Tor в образе","de":"Tor im Image","fr":"Tor interne","en":"Internal Tor","es":"Tor interno"},
"Disabled": {"ru":"Отключён","de":"Deaktiviert","fr":"Désactivé","en":"Disabled","es":"Desactivado"},
"External address": {"ru":"Внешний адрес","de":"Externe Adresse","fr":"Adresse externe","en":"External address","es":"Dirección externa"},
"Internal address": {"ru":"Внутренний адрес","de":"Interne Adresse","fr":"Adresse interne","en":"Internal address","es":"Dirección interna"},
"Use bridges": {"ru":"Использовать мосты","de":"Brücken verwenden","fr":"Utiliser des ponts","en":"Use bridges","es":"Usar puentes"},
"Bridges": {"ru":"Мосты","de":"Brücken","fr":"Ponts","en":"Bridges","es":"Puentes"},
"Tor status": {"ru":"Состояние Tor","de":"Tor-Status","fr":"État de Tor","en":"Tor status","es":"Estado de Tor"},
"Tor log": {"ru":"Лог Tor","de":"Tor-Log","fr":"Journal Tor","en":"Tor log","es":"Registro Tor"},
"Restart Tor": {"ru":"Перезапустить Tor","de":"Tor neu starten","fr":"Redémarrer Tor","en":"Restart Tor","es":"Reiniciar Tor"},
"Save": {"ru":"Сохранить","de":"Speichern","fr":"Enregistrer","en":"Save","es":"Guardar"},
"Back": {"ru":"Назад","de":"Zurück","fr":"Retour","en":"Back","es":"Volver"},
"Generate": {"ru":"Сгенерировать","de":"Erzeugen","fr":"Générer","en":"Generate","es":"Generar"},
"Close": {"ru":"Закрыть","de":"Schließen","fr":"Fermer","en":"Close","es":"Cerrar"},
"Vouchers": {"ru":"Ваучеры","de":"Voucher","fr":"Vouchers","en":"Vouchers","es":"Vouchers"},
"Created": {"ru":"Создан","de":"Erstellt","fr":"Créé","en":"Created","es":"Creado"},
"No vouchers.": {"ru":"Ваучеров нет.","de":"Keine Voucher.","fr":"Aucun voucher.","en":"No vouchers.","es":"No hay vouchers."},
"ON": {"ru":"ВКЛ","de":"EIN","fr":"ON","en":"ON","es":"ON"},
"OFF": {"ru":"ВЫКЛ","de":"AUS","fr":"OFF","en":"OFF","es":"OFF"},
"Saved.": {"ru":"Сохранено.","de":"Gespeichert.","fr":"Enregistré.","en":"Saved.","es":"Guardado."},
"Validation error": {"ru":"Ошибка проверки","de":"Prüfungsfehler","fr":"Erreur de validation","en":"Validation error","es":"Error de validación"},
"Restart required for some settings.": {"ru":"Для части настроек нужен перезапуск.","de":"Einige Einstellungen benötigen einen Neustart.","fr":"Certains réglages nécessitent un redémarrage.","en":"Restart required for some settings.","es":"Algunos ajustes requieren reinicio."},
"Exit with saving?": {"ru":"Выйти с сохранением?","de":"Mit Speichern beenden?","fr":"Quitter en enregistrant ?","en":"Exit with saving?","es":"¿Salir guardando?"},
"Save and exit": {"ru":"Сохранить и выйти","de":"Speichern und beenden","fr":"Enregistrer et quitter","en":"Save and exit","es":"Guardar y salir"},
"Exit without saving": {"ru":"Выйти без сохранения","de":"Ohne Speichern beenden","fr":"Quitter sans enregistrer","en":"Exit without saving","es":"Salir sin guardar"},
"Cancel": {"ru":"Отмена","de":"Abbrechen","fr":"Annuler","en":"Cancel","es":"Cancelar"},
"Confirm": {"ru":"Подтвердить","de":"Bestätigen","fr":"Confirmer","en":"Confirm","es":"Confirmar"},
"Add": {"ru":"Добавить","de":"Hinzufügen","fr":"Ajouter","en":"Add","es":"Añadir"},
"Delete this friend?": {"ru":"Удалить этого друга?","de":"Diesen Freund löschen?","fr":"Supprimer cet ami ?","en":"Delete this friend?","es":"¿Eliminar este amigo?"},
"Delete this voucher?": {"ru":"Удалить этот ваучер?","de":"Diesen Voucher löschen?","fr":"Supprimer ce voucher ?","en":"Delete this voucher?","es":"¿Eliminar este voucher?"},
"Active": {"ru":"Активен","de":"Aktiv","fr":"Actif","en":"Active","es":"Activo"},
"Code": {"ru":"Код","de":"Code","fr":"Code","en":"Code","es":"Código"},
"Status": {"ru":"Статус","de":"Status","fr":"État","en":"Status","es":"Estado"},
"Generate voucher": {"ru":"Создать ваучер","de":"Voucher erzeugen","fr":"Générer un voucher","en":"Generate voucher","es":"Generar voucher"},
"Saved and returned.": {"ru":"Сохранено, возврат.","de":"Gespeichert, zurück.","fr":"Enregistré, retour.","en":"Saved and returned.","es":"Guardado y vuelta."},
"Value": {"ru":"Значение","de":"Wert","fr":"Valeur","en":"Value","es":"Valor"},
"Language": {"ru":"Язык","de":"Sprache","fr":"Langue","en":"Language","es":"Idioma"},
"Enter": {"ru":"Ввод","de":"Eingabe","fr":"Entrée","en":"Enter","es":"Entrar"},
"Arrows/Tab": {"ru":"Стрелки/Tab","de":"Pfeile/Tab","fr":"Flèches/Tab","en":"Arrows/Tab","es":"Flechas/Tab"},
"Ctrl+C": {"ru":"Ctrl+C","de":"Ctrl+C","fr":"Ctrl+C","en":"Ctrl+C","es":"Ctrl+C"},
"Validity (days)": {"ru":"Срок действия (дней)","de":"Gültigkeit (Tage)","fr":"Validité (jours)","en":"Validity (days)","es":"Validez (días)"},
"Days": {"ru":"Дней","de":"Tage","fr":"Jours","en":"Days","es":"Días"},
"Expires": {"ru":"Истекает","de":"Läuft ab","fr":"Expire","en":"Expires","es":"Caduca"},
"Expired": {"ru":"Истёк","de":"Abgelaufen","fr":"Expiré","en":"Expired","es":"Caducado"},
"Working...": {"ru":"Выполняется...","de":"Arbeite...","fr":"En cours...","en":"Working...","es":"Procesando..."},
"Days must be a number.": {"ru":"Дней должно быть числом.","de":"Tage müssen eine Zahl sein.","fr":"Les jours doivent être un nombre.","es":"Los días deben ser un número."},
"Days must be 1..36500.": {"ru":"Дней должно быть 1..36500.","de":"Tage müssen 1..36500 sein.","fr":"Les jours doivent être 1..36500.","es":"Los días deben ser 1..36500."},
"Voucher created": {"ru":"Ваучер создан","de":"Voucher erstellt","fr":"Voucher créé","en":"Voucher created","es":"Voucher creado"},
"Voucher deleted": {"ru":"Ваучер удалён","de":"Voucher gelöscht","fr":"Voucher supprimé","en":"Voucher deleted","es":"Voucher eliminado"},
"Delete failed": {"ru":"Не удалось удалить","de":"Löschen fehlgeschlagen","fr":"Échec de la suppression","en":"Delete failed","es":"Error al eliminar"},
"Selected voucher": {"ru":"Выбранный ваучер","de":"Ausgewählter Voucher","fr":"Voucher sélectionné","en":"Selected voucher","es":"Voucher seleccionado"},
"Copy": {"ru":"Копировать","de":"Kopieren","fr":"Copier","en":"Copy","es":"Copiar"},
"Voucher copied": {"ru":"Ваучер скопирован","de":"Voucher kopiert","fr":"Voucher copié","en":"Voucher copied","es":"Voucher copiado"},
"Clipboard unavailable": {"ru":"Буфер обмена недоступен","de":"Zwischenablage nicht verfügbar","fr":"Presse-papiers indisponible","en":"Clipboard unavailable","es":"Portapapeles no disponible"},
}

def clipboard_copy(text):
    try:
        data = base64.b64encode(str(text).encode("utf-8")).decode("ascii")
        sys.stdout.write("\033]52;c;%s\a" % data)
        sys.stdout.flush()
        return True
    except Exception:
        return False


def t(text):
    return T.get(text, {}).get(ui_lang, text)

ui_lang = "ru"

def read(path, default=""):
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as fh:
            return fh.read()
    except OSError:
        return default

def write_keep(path, text, mode=None):
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    old = None
    try:
        st = os.stat(path)
        old = (st.st_mode & 0o7777, st.st_uid, st.st_gid)
    except OSError:
        pass
    tmp = path + ".omtmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(text)
    if mode is None:
        mode = old[0] if old else 0o644
    os.chmod(tmp, mode)
    if old:
        try:
            os.chown(tmp, old[1], old[2])
        except OSError:
            pass
    os.replace(tmp, path)

def state_load():
    try:
        with open(STATE_PATH, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        if isinstance(data, dict):
            return data
    except Exception:
        pass
    return None

def state_save(st):
    st["updated"] = datetime.datetime.now().isoformat(timespec="seconds")
    os.makedirs(os.path.dirname(STATE_PATH), exist_ok=True)
    tmp = STATE_PATH + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(st, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    os.chmod(tmp, 0o640)
    try:
        shutil.chown(tmp, user="root", group="debian-onionmail")
    except (LookupError, PermissionError, OSError):
        os.chmod(tmp, 0o644)
    os.replace(tmp, STATE_PATH)

def find_server_conf(nick=None):
    if nick:
        p = os.path.join(SERVERS_DIR, "1-%s.conf" % nick)
        if os.path.exists(p):
            return p
    env = os.environ.get("ONIONMAIL_NICK", "")
    if env:
        p = os.path.join(SERVERS_DIR, "1-%s.conf" % env)
        if os.path.exists(p):
            return p
    files = sorted(glob.glob(os.path.join(SERVERS_DIR, "1-*.conf")))
    return files[0] if files else ""

def parse_kv(text, key):
    m = re.search(r"(?im)^\s*%s\s+([^\s#]+)" % re.escape(key), text)
    return m.group(1) if m else ""

def parse_server(path):
    text = read(path)
    nick = ""
    m = re.search(r"(?im)^\s*SMTPServer\s+(\S+)\s*\{", text)
    if m:
        nick = m.group(1)
    out = {"text": text, "nick": nick, "onion": parse_kv(text, "Onion")}
    return out

def parse_friends(text):
    m = re.search(r"(?is)\bFriends\s*\{(.*?)\}", text)
    if not m:
        return []
    return [x for x in re.findall(r"[a-z2-7]{16,56}\.onion", m.group(1))]

def derive_state():
    conf = find_server_conf()
    srv = parse_server(conf) if conf else {"nick":"", "onion":"", "text":""}
    nick = srv["nick"] or os.environ.get("ONIONMAIL_NICK", "onionmail")
    http = read(HTTP_CONF)
    display = parse_kv(http, "HTTPServerName")
    display = re.sub(r"\s+onionmail\s+server\s*$", "", display, flags=re.I) or os.environ.get("ONIONMAIL_DISPLAY_NAME", "example")
    text = srv["text"]
    def num(key, default, mult=1):
        v = parse_kv(text, key)
        m = re.match(r"([0-9]+)\s*([MGK]?)", v, re.I)
        if not m:
            return default
        n = int(m.group(1))
        unit = m.group(2).upper()
        if unit == "G": n *= 1024
        elif unit == "K": n = max(1, n // 1024)
        return n * mult
    debug = parse_kv(read(CORE_CONF), "Debug").lower() in ("yes","true","y","1")
    autodel = parse_kv(read(POP3_CONF), "AutoDeleteReaded").lower() in ("yes","true","y","1")
    srvtype = parse_kv(text, "ServerType").lower()
    relay = parse_kv(text, "CanRelay").lower() in ("yes","true","y","1")
    logv = bool(parse_kv(text, "LogVoucherTo")) or ("LogVoucherTo" in text and not re.search(r"(?im)^\s*#\s*LogVoucherTo", text))
    cden_text = read(os.path.join(WWWROOT, "config.denied.conf"))
    captcha_disabled = parse_kv(cden_text, "captcha-disable").lower() == "yes"
    logo = parse_kv(cden_text, "mainlogo") or DEFAULT_LOGO
    env1 = read("/proc/1/environ").split("\0")
    envmap = dict(x.split("=",1) for x in env1 if "=" in x)
    host_tor = envmap.get("USE_HOST_TOR", os.environ.get("USE_HOST_TOR", "1")) == "1"
    core = read(CORE_CONF)
    tip = parse_kv(core, "TorIP") or "127.0.0.1"
    tport = parse_kv(core, "TorPort") or "9050"
    passwd = read(SERVICE_PASS).strip()
    st = {
        "lang": "ru",
        "server_name": display,
        "onion": srv["onion"] or envmap.get("ONION_ADDRESS", ""),
        "nick": nick,
        "locale": os.environ.get("LANG", "C.UTF-8"),
        "relay": relay,
        "admin_password": passwd,
        "admin_enabled": True,
        "admin_captcha": not captcha_disabled,
        "limits": {
            "max_messages": num("MaxMsgXuser", 256),
            "max_message_mb": num("MaxMsgSize", 256),
            "box_mb": 4096,
            "max_ml": num("MaxMailingListSize", 4096),
            "max_rcpt": num("MultiDeliverMaxRCPTTo", 24),
            "max_spam": num("MaxSpamEntryXUser", 196),
            "debug": debug,
            "autodelete": autodel,
            "exit": srvtype == "exit",
            "logvoucher": logv,
        },
        "friends": parse_friends(read("/etc/onionmail/feed.conf")),
        "registration": {
            "captcha": not captcha_disabled,
            "voucher_required": False,
            "exit_name_enabled": True,
            "exit_enabled": True,
            "exit_name": "",
        },
        "design": {
            "theme": "light",
            "logo": logo,
        },
        "tor": {
            "mode": "external" if host_tor else "internal",
            "ip": tip,
            "port": int(tport) if str(tport).isdigit() else 9050,
            "internal_ip": "127.0.0.1",
            "internal_port": 19050,
            "bridges": False,
            "bridge_lines": "",
        },
        "vouchers": [],
    }
    return st

def normalize_state(st, base):
    def merge(a, b):
        if not isinstance(b, dict):
            return a
        out = dict(a)
        for k, v in b.items():
            if isinstance(v, dict) and isinstance(out.get(k), dict):
                out[k] = merge(out[k], v)
            else:
                out[k] = v
        return out
    result = merge(base, st)
    design = result.get("design")
    if not isinstance(design, dict):
        design = {}
    if design.get("theme") not in ("light", "dark"):
        design["theme"] = "light"
    logo = design.get("logo")
    if not isinstance(logo, str) or not logo.strip() or re.search(r"\s", logo):
        design["logo"] = DEFAULT_LOGO
    result["design"] = design
    return result

def load_state():
    base = derive_state()
    st = state_load()
    if st:
        base = normalize_state(st, base)
    return base

def set_key_flat(text, key, value):
    pat = re.compile(r"(?im)^(\s*%s\s+).*$" % re.escape(key))
    if pat.search(text):
        return pat.sub(lambda m: m.group(1) + str(value), text)
    if text and not text.endswith("\n"):
        text += "\n"
    return text + "%s\t%s\n" % (key, value)

def set_key_server(text, key, value):
    pat = re.compile(r"(?im)^(\s*%s\s+).*$" % re.escape(key))
    if pat.search(text):
        return pat.sub(lambda m: m.group(1) + str(value), text)
    lines = text.splitlines()
    idx = None
    for i in range(len(lines)-1, -1, -1):
        if lines[i].strip().startswith("}"):
            idx = i
            break
    ins = "\t%s\t%s" % (key, value)
    if idx is None:
        lines.append(ins)
    else:
        lines.insert(idx, ins)
    return "\n".join(lines) + ("\n" if text.endswith("\n") else "")

def set_nick_paths(text, old, new):
    text = text.replace(old, new)
    text = re.sub(r"(?m)^(\s*SMTPServer\s+)\S+", r"\g<1>" + new, text)
    return text

def migrate_nick(st):
    new = st.get("nick", "").strip()
    if not new:
        return
    newconf = os.path.join(SERVERS_DIR, "1-%s.conf" % new)
    if os.path.exists(newconf):
        return
    files = [p for p in glob.glob(os.path.join(SERVERS_DIR, "1-*.conf")) if p != newconf]
    if not files:
        return
    oldconf = files[0]
    old = parse_server(oldconf)["nick"]
    if not old or old == new:
        return
    try:
        os.rename(oldconf, newconf)
    except OSError:
        return
    text = read(newconf)
    text = set_nick_paths(text, old, new)
    write_keep(newconf, text)
    oldmail = "/var/onionmail/" + old
    newmail = "/var/onionmail/" + new
    if os.path.isdir(oldmail) and not os.path.exists(newmail):
        try:
            shutil.move(oldmail, newmail)
        except Exception:
            pass
    oldhs = "/var/lib/onionmail/HiddenServiceDir-1-" + old
    newhs = "/var/lib/onionmail/HiddenServiceDir-1-" + new
    if os.path.isdir(oldhs) and not os.path.exists(newhs):
        try:
            shutil.move(oldhs, newhs)
        except Exception:
            pass

def captcha_disable_value(st):
    reg = bool(st.get("registration", {}).get("captcha", True))
    adm = bool(st.get("admin_captcha", True))
    return "no" if (reg or adm) else "yes"

def apply_state(st, do_migrate=True):
    if do_migrate:
        migrate_nick(st)
    conf = find_server_conf(st.get("nick"))
    if not conf:
        conf = find_server_conf()
    if conf:
        text = read(conf)
        lim = st.get("limits", {})
        text = set_key_server(text, "SMTPServer", st.get("nick", "")) if False else text
        text = set_key_server(text, "Onion", st.get("onion", ""))
        text = set_key_server(text, "MaxMsgXuser", int(lim.get("max_messages", 256)))
        text = set_key_server(text, "MaxMsgSize", "%dM" % int(lim.get("max_message_mb", 256)))
        text = set_key_server(text, "MaxMailingListSize", int(lim.get("max_ml", 4096)))
        text = set_key_server(text, "MultiDeliverMaxRCPTTo", int(lim.get("max_rcpt", 24)))
        text = set_key_server(text, "MaxSpamEntryXUser", int(lim.get("max_spam", 196)))
        text = set_key_server(text, "CanRelay", "yes" if st.get("relay") else "no")
        text = set_key_server(text, "ServerType", "Exit" if lim.get("exit") else "normal")
        text = set_key_server(text, "CanPOP3Register", "no")
        vlog = "/var/log/onionmail/%s-vouchers.log" % st.get("nick", "onionmail")
        vlist = "/var/log/onionmail/%s-vouchers.lst" % st.get("nick", "onionmail")
        if lim.get("logvoucher"):
            text = set_key_server(text, "LogVoucherTo", vlog)
        else:
            text = re.sub(r"(?im)^(\s*)LogVoucherTo\s+.*$", r"\1#LogVoucherTo disabled", text)
        text = set_key_server(text, "VoucherListTo", vlist)
        text = re.sub(r"(?m)^(\s*MailDir\s+).*$", r"\g<1>/var/onionmail/" + st.get("nick","onionmail"), text)
        text = re.sub(r"(?m)^(\s*StatFile\s+).*$", r"\g<1>/var/log/onionmail/" + st.get("nick","onionmail") + "-stats.csv", text)
        text = re.sub(r"(?m)^(\s*BinStatFile\s+).*$", r"\g<1>/var/log/onionmail/" + st.get("nick","onionmail") + "-stats.dat", text)
        write_keep(conf, text)
    core = read(CORE_CONF)
    core = re.sub(r"(?im)^(\s*Debug\s+).*$", r"\g<1>" + ("yes" if st.get("limits",{}).get("debug") else "no"), core)
    write_keep(CORE_CONF, core)
    http = read(HTTP_CONF)
    http = re.sub(r"(?im)^(\s*HTTPServerName\s+).*$", r"\g<1>" + st.get("server_name","example") + " onionmail server", http)
    write_keep(HTTP_CONF, http)
    pop = read(POP3_CONF)
    pop = re.sub(r"(?im)^(\s*AutoDeleteReaded\s+).*$", r"\g<1>" + ("yes" if st.get("limits",{}).get("autodelete") else "no"), pop)
    write_keep(POP3_CONF, pop)
    cden = os.path.join(WWWROOT, "config.denied.conf")
    if os.path.exists(cden):
        c = read(cden)
        c = re.sub(r"(?im)^(\s*captcha-disable\s+).*$", r"\g<1>" + captcha_disable_value(st), c)
        logo = str(st.get("design", {}).get("logo") or DEFAULT_LOGO)
        if re.search(r"\s", logo):
            logo = DEFAULT_LOGO
        c = set_key_flat(c, "mainlogo", logo)
        write_keep(cden, c)
    p = st.get("admin_password", "")
    if p:
        write_keep(SERVICE_PASS, p, 0o600)
        text = read(conf) if conf else ""
        if text:
            text = set_key_server(text, "PassWD", p)
            write_keep(conf, text)
    if st.get("friends") is not None:
        text = read(conf) if conf else ""
        if text:
            text = re.sub(r"(?is)\n\s*Friends\s*\{.*?\}\s*", "\n", text, count=1)
            write_keep(conf, text)
        feed_path = "/etc/onionmail/feed.conf"
        feed = read(feed_path)
        block = "Friends {\n" + "".join("\t%s\n" % x for x in st.get("friends", [])) + "\t}"
        if re.search(r"(?is)\bFriends\s*\{.*?\}", feed):
            feed = re.sub(r"(?is)\bFriends\s*\{.*?\}", block, feed, count=1)
        else:
            feed = feed.rstrip() + "\n\n" + block + "\n"
        write_keep(feed_path, feed)
    env_path = "/etc/environment"
    env_text = read(env_path)
    env_text = re.sub(r"(?m)^LANG=.*\n?", "", env_text)
    env_text = env_text.rstrip() + "\nLANG=%s\n" % st.get("locale", "C.UTF-8")
    write_keep(env_path, env_text)
    old_display = st.get("brand_name")
    new_display = st.get("server_name", "")
    if old_display and new_display and old_display != new_display:
        brand_pages(old_display, new_display)
    st["brand_name"] = new_display
    state_save(st)
    return conf

def brand_pages(old, new):
    if not old or not new or old == new:
        return
    for path in glob.glob(os.path.join(WWWROOT, "**", "*.html"), recursive=True):
        text = read(path)
        if old in text:
            write_keep(path, text.replace(old, new))

def write_torrc(st):
    tor = st.get("tor", {})
    nick = st.get("nick", "onionmail")
    ip = tor.get("internal_ip", "127.0.0.1")
    port = int(tor.get("internal_port", 19050))
    hsdir = "/var/lib/onionmail/HiddenServiceDir-1-" + nick
    lines = [
        "User debian-tor",
        "SocksPort %s:%d" % (ip, port),
        "DataDirectory /var/lib/tor",
        "Log notice file " + TOR_LOG,
        "HiddenServiceDir " + hsdir,
        "HiddenServiceVersion 3",
        "HiddenServicePort 25 127.0.0.1:20025",
        "HiddenServicePort 110 127.0.0.1:20110",
        "HiddenServicePort 80 127.0.0.1:18000",
    ]
    bridges = [x.strip() for x in str(tor.get("bridge_lines", "")).replace(",", "\n").splitlines() if x.strip()]
    if tor.get("bridges") and bridges:
        lines.append("UseBridges 1")
        if any("obfs4" in b for b in bridges):
            lines.append("ClientTransportPlugin obfs4 exec /usr/bin/obfs4proxy")
        for b in bridges:
            lines.append(b if b.lower().startswith("bridge ") else "Bridge " + b)
    write_keep(TORRC, "\n".join(lines) + "\n", 0o644)
    os.makedirs("/var/lib/tor", exist_ok=True)
    os.makedirs("/run/tor", exist_ok=True)
    os.makedirs(hsdir, exist_ok=True)
    for d in ("/var/lib/tor", "/run/tor", hsdir):
        try:
            shutil.chown(d, "debian-tor", "debian-tor")
            os.chmod(d, 0o700)
        except Exception:
            pass
    logdir = os.path.dirname(TOR_LOG)
    os.makedirs(logdir, exist_ok=True)
    try:
        shutil.chown(logdir, group="debian-tor")
        os.chmod(logdir, 0o750)
    except Exception:
        pass
    try:
        open(TOR_LOG, "a").close()
        shutil.chown(TOR_LOG, "debian-tor", "debian-tor")
        os.chmod(TOR_LOG, 0o640)
    except Exception:
        pass
    return hsdir

def run(cmd, timeout=20):
    try:
        p = subprocess.run(cmd, shell=isinstance(cmd, str), stdout=subprocess.PIPE,
                           stderr=subprocess.STDOUT, timeout=timeout, text=True)
        return p.returncode, p.stdout
    except Exception as exc:
        return 1, str(exc)

def tor_pids():
    rc, out = run("pgrep -x tor")
    return [int(x) for x in out.split() if x.isdigit()] if rc == 0 else []

def stop_tor():
    for pid in tor_pids():
        try:
            os.kill(pid, 15)
        except OSError:
            pass
    time.sleep(0.5)
    for pid in tor_pids():
        try:
            os.kill(pid, 9)
        except OSError:
            pass
    run("service tor stop")

def start_tor(st):
    write_torrc(st)
    stop_tor()
    rc, out = run("service tor start", timeout=20)
    time.sleep(1)
    if not tor_pids():
        log = open(TOR_LOG, "ab")
        subprocess.Popen(["tor", "-f", TORRC], stdout=log, stderr=log, start_new_session=True)
        time.sleep(1)
    return bool(tor_pids())

def tor_progress():
    text = read(TOR_LOG)
    pcts = re.findall(r"Bootstrapped\s+(\d+)%", text)
    if pcts:
        return pcts[-1] + "%"
    if tor_pids():
        return "starting"
    if text:
        lines = [x for x in text.splitlines() if x.strip()]
        return lines[-1][-80:] if lines else "no data"
    return "inactive"

def tor_socket_ok(ip, port):
    try:
        s = socket.create_connection((ip, int(port)), timeout=2)
        s.close()
        return True
    except Exception:
        return False

def get_onion(st):
    paths = [
        "/var/lib/onionmail/HiddenServiceDir-1-%s/hostname" % st.get("nick", "onionmail"),
        "/var/tor/onionmail/hostname",
    ]
    for p in paths:
        v = read(p).strip()
        if re.match(r"^[a-z2-7]{16,56}\.onion$", v):
            return v
    conf = find_server_conf()
    if conf:
        v = parse_server(conf)["onion"]
        if v:
            return v
    return ""

def control_command(cmd, timeout=8):
    nick = load_state().get("nick", "onionmail")
    passwd = read(SERVICE_PASS).strip()
    s = socket.create_connection(("127.0.0.1", 9010), timeout=timeout)
    try:
        s.settimeout(timeout)
        f = s.makefile("rwb", buffering=0)
        f.readline()
        f.write(("server %s %s\n" % (nick, passwd)).encode())
        f.readline()
        f.write((cmd + "\n").encode())
        line = f.readline().decode("utf-8", "replace").strip()
        f.write(b"quit\n")
        return line
    finally:
        s.close()

def validate(st):
    errs = []
    nick = st.get("nick","")
    if not re.match(r"^[a-z][a-z0-9_-]{0,30}$", nick):
        errs.append("NICK: 1-31 chars, lowercase a-z, digits, _ or -")
    name = st.get("server_name","").strip()
    if not name or len(name) > 48 or "\n" in name or "\r" in name:
        errs.append("server name: 1-48 chars, single line")
    onion = st.get("onion","").strip()
    if not re.match(r"^(?:[a-z2-7]{16}|[a-z2-7]{56})\.onion$", onion):
        errs.append("onion address: 16 or 56 base32 chars + .onion")
    if not re.match(r"^[A-Za-z0-9_.@-]{1,40}$", st.get("locale","")):
        errs.append("LANG: invalid")
    if len(st.get("admin_password","")) < 8:
        errs.append("admin password: at least 8 chars")
    lim = st.get("limits", {})
    ranges = {"max_messages":(1,1000000),"max_message_mb":(1,2048),"box_mb":(16,1048576),
              "max_ml":(1,1000000),"max_rcpt":(1,10000),"max_spam":(1,1000000)}
    for k,(lo,hi) in ranges.items():
        try:
            v = int(lim.get(k))
        except Exception:
            errs.append("%s: number required" % k); continue
        if not lo <= v <= hi:
            errs.append("%s: range %d..%d" % (k,lo,hi))
    for f in st.get("friends", []):
        if not re.match(r"^[a-z2-7]{16,56}\.onion$", f):
            errs.append("friend: invalid onion " + f)
    reg = st.get("registration", {})
    en = reg.get("exit_name","").strip()
    if reg.get("exit_name_enabled") and en and not re.match(r"^[a-z0-9][a-z0-9._-]{2,30}[a-z0-9]$", en):
        errs.append("exit name: 4-32 chars latin/digits/._-")
    tor = st.get("tor", {})
    if tor.get("mode") not in ("external","internal","off"):
        errs.append("tor mode: invalid")
    for key in ("ip","internal_ip"):
        v = str(tor.get(key,""))
        if v and not re.match(r"^[A-Za-z0-9_.:-]+$", v):
            errs.append("tor address: invalid")
    for key in ("port","internal_port"):
        try:
            v = int(tor.get(key))
        except Exception:
            errs.append("tor port: number required"); continue
        if not 1 <= v <= 65535:
            errs.append("tor port: 1..65535")
    logo = str(st.get("design", {}).get("logo", DEFAULT_LOGO))
    if not logo or re.search(r"\s", logo):
        errs.append("logo: invalid path")
    return errs

def tor_config_errors(st):
    tor = st.get("tor", {})
    if tor.get("mode") != "internal":
        return []
    try:
        write_torrc(st)
    except Exception as exc:
        return ["tor config: " + str(exc)]
    rc, out = run(["tor", "--verify-config", "-f", TORRC], timeout=20)
    if rc == 0:
        return []
    lines = [x.strip() for x in out.splitlines() if x.strip()]
    warns = [x for x in lines if "[warn]" in x]
    if not warns:
        warns = [x for x in lines if "[err]" in x]
    return ["tor config: " + (warns[-1] if warns else (lines[-1] if lines else "invalid"))]

def save(st, section=""):
    errs = validate(st)
    if errs:
        return False, errs
    try:
        apply_state(st, do_migrate=False)
        if section == "tor":
            tor = st.get("tor", {})
            if tor.get("mode") == "internal":
                ok = start_tor(st)
                if not ok:
                    return False, ["Tor did not start; see Tor log"]
            else:
                stop_tor()
        return True, []
    except Exception as exc:
        return False, [str(exc)]

def boot_env():
    st = load_state()
    migrate_nick(st)
    try:
        apply_state(st)
    except Exception:
        pass
    tor = st.get("tor", {})
    mode = tor.get("mode", "external")
    if mode == "internal":
        write_torrc(st)
        tip, tp = tor.get("internal_ip","127.0.0.1"), tor.get("internal_port",19050)
    elif mode == "external":
        tip, tp = tor.get("ip","127.0.0.1"), tor.get("port",9050)
    else:
        tip, tp = "127.0.0.1", 1
    vals = {
        "NICK": st.get("nick","onionmail"),
        "DISPLAY_NAME": st.get("server_name","example"),
        "ONION": st.get("onion",""),
        "LOCALE": st.get("locale","C.UTF-8"),
        "TOR_MODE": mode,
        "TOR_IP": str(tip),
        "TOR_PORT": str(tp),
        "USE_HOST_TOR": "1" if mode == "external" else "0",
        "CAPTCHA_DISABLE": captcha_disable_value(st),
        "DEFAULT_THEME": str(st.get("design", {}).get("theme", "light")),
    }
    for k,v in vals.items():
        print("%s=%s" % (k, shlex.quote(str(v))))

def install_logo(st, path):
    src = os.path.abspath(os.path.expanduser(str(path).strip().strip('"').strip("'")))
    if not os.path.isfile(src):
        return False, t("Image not found") + ": " + src
    ext = os.path.splitext(src)[1].lower()
    if ext not in (".png", ".jpg", ".jpeg"):
        return False, t("Unsupported image format (PNG/JPG only).")
    try:
        size = os.path.getsize(src)
    except OSError:
        return False, t("Image not found") + ": " + src
    if size <= 0:
        return False, t("Image not found") + ": " + src
    if size > LOGO_MAX_BYTES:
        return False, t("Image is too large (max 5 MB).")
    with open(src, "rb") as fh:
        head = fh.read(8)
    if ext == ".png" and not head.startswith(b"\x89PNG\r\n\x1a\n"):
        return False, t("Unsupported image format (PNG/JPG only).")
    if ext in (".jpg", ".jpeg") and not head.startswith(b"\xff\xd8\xff"):
        return False, t("Unsupported image format (PNG/JPG only).")
    imgdir = os.path.join(WWWROOT, "img")
    dst = os.path.join(imgdir, "om-custom-logo" + ext)
    try:
        os.makedirs(imgdir, exist_ok=True)
        if os.path.abspath(src) != os.path.abspath(dst):
            tmp = dst + ".omtmp"
            shutil.copyfile(src, tmp)
            os.chmod(tmp, 0o644)
            try:
                shutil.chown(tmp, user="debian-onionmail", group="debian-onionmail")
            except (LookupError, PermissionError, OSError):
                pass
            os.replace(tmp, dst)
        for old in glob.glob(os.path.join(imgdir, "om-custom-logo.*")):
            if os.path.abspath(old) != os.path.abspath(dst):
                try:
                    os.remove(old)
                except OSError:
                    pass
        st.setdefault("design", {})["logo"] = "/img/" + os.path.basename(dst)
        apply_state(st, do_migrate=False)
    except Exception as exc:
        return False, str(exc)
    return True, t("Logo updated.")

def reset_logo(st):
    for old in glob.glob(os.path.join(WWWROOT, "img", "om-custom-logo.*")):
        try:
            os.remove(old)
        except OSError:
            pass
    default_file = os.path.join(WWWROOT, "img", "OM-BL.gif")
    seed_file = "/opt/seed/var-lib-onionmail/wwwroot/img/OM-BL.gif"
    if not os.path.isfile(default_file) and os.path.isfile(seed_file):
        try:
            shutil.copyfile(seed_file, default_file)
            os.chmod(default_file, 0o644)
        except OSError:
            pass
    st.setdefault("design", {})["logo"] = DEFAULT_LOGO
    try:
        apply_state(st, do_migrate=False)
    except Exception as exc:
        return False, str(exc)
    return True, t("Logo reset.")

class UI:
    def __init__(self, screen, st):
        self.scr = screen
        self.st = st
        self.cat = 0
        self.sel = 0
        self.scroll = 0
        self.msg = ""
        self.msg_ok = True
        self.cats = ["1. Main","2. Admin access","3. Limits","4. Server friends","5. Registration","6. Tor","7. Design"]
        self.init_colors()

    def init_colors(self):
        try:
            curses.start_color()
            curses.use_default_colors()
        except Exception:
            return
        if curses.COLORS >= 256:
            bg, bg2, fg, acc, sel = -1, -1, 183, 141, 135
            ok, err, mut = 141, 203, 97
        else:
            bg, bg2, fg, acc, sel = -1, -1, curses.COLOR_MAGENTA, curses.COLOR_MAGENTA, curses.COLOR_MAGENTA
            ok, err, mut = curses.COLOR_MAGENTA, curses.COLOR_RED, curses.COLOR_MAGENTA
        pairs = [(1,fg,bg),(2,acc,bg2),(3,sel,bg),(4,acc,bg),(5,ok,bg),(6,err,bg),(7,mut,bg),(8,bg,acc)]
        for i,a,b in pairs:
            try: curses.init_pair(i,a,b)
            except Exception: pass

    def c(self,n): return curses.color_pair(n)
    def paint(self, y, x, text, attr=0, maxw=None):
        h,w = self.scr.getmaxyx()
        if y < 0 or y >= h or x >= w: return
        if maxw is None: maxw = w-x
        text = str(text)
        if len(text) > maxw: text = text[:max(0,maxw)]
        try: self.scr.addstr(y,x,text,attr)
        except Exception: pass
    def fill(self, attr):
        h,w = self.scr.getmaxyx()
        line = " " * w
        for y in range(h):
            try: self.scr.addstr(y,0,line,attr)
            except Exception: pass

    def draw(self):
        self.scr.erase()
        self.fill(self.c(1))
        h,w = self.scr.getmaxyx()
        narrow = w < 120
        title = t("OnionMail Control")
        self.paint(0,0," " * w, self.c(2))
        self.paint(0,2,title, self.c(2)|curses.A_BOLD)
        nick = str(self.st.get("nick",""))
        full = nick + "@" + str(self.st.get("onion",""))
        info = full if (len(full) + len(title) + 8) <= w else nick
        if info:
            ix = w - len(info) - 2
            if ix >= len(title) + 4:
                self.paint(0,ix,info,self.c(2))
        if h > 2:
            self.paint(1,0," " * w, self.c(4))
        if narrow:
            groups = (range(0,3), range(3,6), range(6,len(self.cats)))
            for row, group in enumerate(groups):
                x = 2
                for i in group:
                    label = t(self.cats[i])
                    if i == self.cat:
                        text = "<" + label + ">"
                        attr = self.c(3) | curses.A_BOLD
                    else:
                        text = label
                        attr = self.c(1)
                    self.paint(2+row,x,text,attr)
                    x += len(text) + 2
            sep_y = 2 + len(groups)
            self.paint(sep_y,0," " * w,self.c(4))
            items_y = sep_y + 1
            x = 2
        else:
            left = min(28, max(20, w//3))
            for i,name in enumerate(self.cats):
                label = t(name)
                if i == self.cat:
                    text = "<" + label + ">"
                    attr = self.c(3) | curses.A_BOLD
                else:
                    text = " " + label
                    attr = self.c(1)
                self.paint(3+i,1,text.ljust(left-2),attr)
            self.paint(2,1," " * (left-2),self.c(4))
            self.paint(2,2,"MENU",self.c(4)|curses.A_BOLD)
            items_y = 3
            x = left + 1
        items = self.items()
        total = len(items)
        vis = max(1, h - (8 if narrow else 7))
        if self.sel < self.scroll: self.scroll = self.sel
        if self.sel >= self.scroll + vis: self.scroll = self.sel - vis + 1
        for idx in range(self.scroll, min(total, self.scroll+vis)):
            it = items[idx]
            y = items_y + idx - self.scroll
            label, kind, key = it[0], it[1], it[2]
            val = self.value_for(it)
            if narrow and kind == "friend":
                val = ""
            selected = idx == self.sel
            shown = "<" + t(label) + ">" if selected else t(label)
            if kind == "action":
                line = ("<" + t(label) + ">" if selected else "[ " + t(label) + " ]")
            else:
                lead = "" if narrow else " "
                gap = " " if narrow else "  "
                free = w - x
                avail = free - len(str(val)) - len(gap) - len(lead)
                label_w = min(34, max(len(shown), avail))
                line = lead + shown.ljust(label_w) + gap + str(val)
            attr = (self.c(3) | curses.A_BOLD) if selected else self.c(1)
            self.paint(y,x,line.ljust(max(0,w-x)),attr)
        fh = h-2
        self.paint(fh,0," " * w, self.c(4))
        keys = "↑↓/Tab  Enter  Ctrl+S " + t("Save") + "  Ctrl+C " + t("Cancel")
        self.paint(fh,2,keys,self.c(4))
        if self.msg:
            self.paint(h-1,2,self.msg,self.c(5) if self.msg_ok else self.c(6)|curses.A_BOLD)
        self.scr.refresh()

    def items(self):
        st = self.st
        if self.cat == 0:
            return [("Server name","text","server_name"),("Onion address","text","onion"),
                    ("Get from daemon","action","get_onion"),("Hidden name (NICK)","text","nick"),
                    ("Encoding (LANG)","text","locale"),("Relay mode","bool","relay"),("Save","action","save")]
        if self.cat == 1:
            return [("Admin login","text","admin_login"),("Admin password","pass","admin_password"),
                    ("Admin panel","bool","admin_enabled"),("Admin CAPTCHA","bool","admin_captcha"),("Save","action","save")]
        if self.cat == 2:
            lim = st["limits"]
            return [("Max messages in inbox","int","max_messages"),("Max message size (MB)","int","max_message_mb"),
                    ("Mailbox size (MB)","int","box_mb"),("Max mailing list size","int","max_ml"),
                    ("Max multi delivery addresses","int","max_rcpt"),("Max spam list size","int","max_spam"),
                    ("Debug mode","bool","debug"),("Delete read messages","bool","autodelete"),
                    ("EXIT/ENTER server","bool","exit"),("Log voucher codes","bool","logvoucher"),("Save","action","save")]
        if self.cat == 3:
            items = [("Add friend","action","add_friend")]
            items += [(("Friend") + " " + f,"friend",f) for f in st.get("friends",[])]
            items.append(("Save","action","save"))
            return items
        if self.cat == 4:
            reg = st["registration"]
            return [("Registration CAPTCHA","bool","reg_captcha"),("Require vouchers","bool","voucher_required"),
                    ("Voucher manager","action","vouchers"),("Exit name field","bool","exit_name_enabled"),
                    ("Exit name","text","exit_name"),("Exit connection","bool","exit_enabled"),("Save","action","save")]
        if self.cat == 6:
            return [("Default theme","choice","default_theme"),("Logo","logo","logo_status"),
                    ("Add logo","action","add_logo"),("Reset logo","action","reset_logo"),("Save","action","save")]
        tor = st["tor"]
        items = [("Tor mode","choice","tor_mode")]
        if tor.get("mode") == "external":
            items += [("External address","text","tor_ip"),("External address","int","tor_port")]
        elif tor.get("mode") == "internal":
            items += [("Internal address","text","tor_internal_ip"),("Internal address","int","tor_internal_port"),
                      ("Use bridges","bool","tor_bridges"),("Bridges","text","tor_bridge_lines")]
        items += [("Tor status","info","tor_status"),("Tor log","action","tor_log"),
                  ("Restart Tor","action","tor_restart"),("Save","action","save")]
        return items

    def value_for(self, it):
        label, kind, key = it[0], it[1], it[2]
        st = self.st
        if kind == "action": return ""
        if kind == "logo":
            logo = st.get("design", {}).get("logo", DEFAULT_LOGO)
            if logo == DEFAULT_LOGO:
                return t("Default")
            return t("Custom") + ": " + os.path.basename(logo)
        if kind == "info":
            tor = st["tor"]
            if tor.get("mode") == "internal":
                return ("%s  %s:%s  %s" % (t("Internal Tor"), tor.get("internal_ip"), tor.get("internal_port"), tor_progress()))
            if tor.get("mode") == "external":
                ok = "OK" if tor_socket_ok(tor.get("ip","127.0.0.1"), tor.get("port",9050)) else "unreachable"
                return "%s  %s:%s  %s" % (t("External Tor"), tor.get("ip"), tor.get("port"), ok)
            return t("Disabled")
        if key == "admin_login": return st.get("nick","")
        if key in ("server_name","onion","nick","locale"): return st.get(key,"")
        if key == "admin_password": return "*" * min(20, len(st.get(key,"")))
        if key in ("relay","admin_enabled","admin_captcha"): return t("ON") if st.get(key) else t("OFF")
        if key.startswith("reg_"):
            return t("ON") if st["registration"].get(key[4:]) else t("OFF")
        if key in ("voucher_required","exit_name_enabled","exit_enabled"):
            return t("ON") if st["registration"].get(key) else t("OFF")
        if key == "exit_name": return st["registration"].get("exit_name","") or "-"
        if key in ("debug","autodelete","exit","logvoucher"):
            return t("ON") if st["limits"].get(key) else t("OFF")
        if key in st.get("limits",{}): return str(st["limits"].get(key,""))
        if key in ("tor_ip","tor_port","tor_internal_ip","tor_internal_port"):
            return str(st["tor"].get(key[4:] if key.startswith("tor_") else key,""))
        if key == "tor_mode":
            return {"external":t("External Tor"),"internal":t("Internal Tor"),"off":t("Disabled")}.get(st["tor"].get("mode"),"?")
        if key == "default_theme":
            return t("Light") if st.get("design", {}).get("theme", "light") == "light" else t("Dark")
        if key == "tor_bridges": return t("ON") if st["tor"].get("bridges") else t("OFF")
        if key == "tor_bridge_lines":
            v = st["tor"].get("bridge_lines","").replace("\n", " ; ")
            return v or "-"
        if kind == "friend": return "Enter: " + t("Delete")
        return ""

    def edit(self, label, current, secret=False):
        h,w = self.scr.getmaxyx()
        s = list(str(current))
        pos = len(s)
        title = t(label) + ": "
        while True:
            shown = ("*" * len(s)) if secret else "".join(s)
            self.paint(h-2,0," " * w, self.c(4))
            self.paint(h-2,2,title[:max(0,w-4)],self.c(4)|curses.A_BOLD)
            self.paint(h-1,0," " * w, self.c(4))
            maxv = max(1, w-4)
            off = max(0, pos - maxv + 1) if len(shown) > maxv else 0
            view = shown[off:off+maxv]
            self.paint(h-1,2,view,self.c(4))
            try: self.scr.move(h-1, 2+pos-off)
            except Exception: pass
            self.scr.refresh()
            try: ch = self.scr.get_wch()
            except curses.error: continue
            if ch in ("\n","\r",curses.KEY_ENTER): return "".join(s)
            if ch == "\x1b": return None
            if ch in ("\x7f",curses.KEY_BACKSPACE, "\b"):
                if pos > 0: pos -= 1; s.pop(pos)
                continue
            if ch == curses.KEY_DC:
                if pos < len(s): s.pop(pos)
                continue
            if ch == curses.KEY_LEFT: pos = max(0,pos-1); continue
            if ch == curses.KEY_RIGHT: pos = min(len(s),pos+1); continue
            if ch == curses.KEY_HOME: pos = 0; continue
            if ch == curses.KEY_END: pos = len(s); continue
            if isinstance(ch, str) and ch.isprintable():
                s.insert(pos, ch); pos += 1

    def confirm(self, text):
        h,w = self.scr.getmaxyx()
        self.paint(h-1,0," " * w, self.c(4))
        self.paint(h-1,2,text + "  [y/N]",self.c(4))
        self.scr.refresh()
        while True:
            ch = self.scr.getch()
            if ch in (ord('y'),ord('Y')): return True
            if ch in (ord('n'),ord('N'),27,10,13): return False

    def choose(self, title, options, current=0):
        h,w = self.scr.getmaxyx()
        self.fill(self.c(1))
        self.paint(2,2,title,self.c(4)|curses.A_BOLD)
        for i,o in enumerate(options):
            attr = (self.c(3) | curses.A_BOLD) if i==current else self.c(1)
            text = "<" + t(o) + ">" if i==current else "  " + t(o)
            self.paint(4+i,4,text,attr)
        self.paint(h-1,2,"↑↓ Enter Esc",self.c(4))
        self.scr.refresh()
        while True:
            ch = self.scr.getch()
            if ch in (curses.KEY_UP,ord('k')): current=(current-1)%len(options)
            elif ch in (curses.KEY_DOWN,ord('j')): current=(current+1)%len(options)
            elif ch in (10,13,curses.KEY_ENTER): return current
            elif ord('1') <= ch <= ord('9') and ch - ord('1') < len(options): return ch - ord('1')
            elif ch == 27: return None
            self.paint(4,4," "*(w-5))
            for i,o in enumerate(options):
                for yy in range(4,4+len(options)):
                    self.paint(yy,4," "*(w-5))
            self.paint(2,2,title,self.c(4)|curses.A_BOLD)
            for i,o in enumerate(options):
                attr = (self.c(3) | curses.A_BOLD) if i==current else self.c(1)
                text = "<" + t(o) + ">" if i==current else "  " + t(o)
                self.paint(4+i,4,text,attr)
            self.paint(h-1,2,"↑↓ Enter Esc",self.c(4))
            self.scr.refresh()

    def language_screen(self):
        opts = ["Русский","Deutsch","Français","English","Español"]
        codes = ["ru","de","fr","en","es"]
        cur = codes.index(self.st.get("lang","ru")) if self.st.get("lang") in codes else 0
        r = self.choose("Choose language / Выберите язык", opts, cur)
        if r is not None:
            self.st["lang"] = codes[r]
            global ui_lang
            ui_lang = codes[r]
            state_save(self.st)

    def voucher_screen(self):
        global ui_lang
        sel = 0
        off = 0
        msg = ""
        msg_ok = True
        while True:
            self.fill(self.c(1))
            h,w = self.scr.getmaxyx()
            self.paint(2,2,t("Voucher manager"),self.c(4)|curses.A_BOLD)
            vs = self.st.setdefault("vouchers", [])
            if not isinstance(vs, list):
                vs = self.st["vouchers"] = []
            if sel >= len(vs):
                sel = max(0, len(vs)-1)
            vis = max(1, h-8)
            if sel < off:
                off = sel
            if sel >= off + vis:
                off = sel - vis + 1
            if off > max(0, len(vs)-vis):
                off = max(0, len(vs)-vis)
            code_w = max(12, min(40, w-56))
            header = t("Code").ljust(code_w) + " " + t("Created").ljust(16) + " " + t("Days").rjust(5) + " " + t("Expires").ljust(11) + " " + t("Status")
            self.paint(3,2,header[:w-3],self.c(2))
            today = datetime.datetime.now().strftime("%Y-%m-%d")
            for pos in range(off, min(len(vs), off+vis)):
                v = vs[pos] if isinstance(vs[pos], dict) else {}
                code = str(v.get("code",""))[:code_w].ljust(code_w)
                created = str(v.get("created",""))[:16].ljust(16)
                days = v.get("days","")
                exp = str(v.get("expires",""))
                if not exp and days:
                    try:
                        dt = datetime.datetime.strptime(str(v.get("created","")), "%Y-%m-%d %H:%M")
                        exp = (dt + datetime.timedelta(days=int(days))).strftime("%Y-%m-%d")
                    except Exception:
                        exp = ""
                status = t("Expired") if exp and exp < today else t("Active")
                line = code + " " + created + " " + str(days or "-").rjust(5) + " " + (exp or "-").ljust(11) + " " + status
                attr = (self.c(3) | curses.A_BOLD) if pos == sel else self.c(1)
                self.paint(4+pos-off,2,line[:w-3],attr)
            if not vs:
                self.paint(4,2,t("No vouchers."),self.c(7))
            self.paint(h-3,2,"[G] "+t("Generate")+"    [D] "+t("Delete")+"    [T] "+t("Copy")+"    [S] "+t("Save")+"    [Q] "+t("Close"),self.c(4))
            self.paint(h-2,2,"↑↓/Tab "+t("Selected voucher"),self.c(7))
            if msg:
                self.paint(h-1,2,str(msg)[:w-3],self.c(5) if msg_ok else self.c(6)|curses.A_BOLD)
            self.scr.refresh()
            ch = self.scr.getch()
            if ch in (ord('q'),ord('Q'),27):
                return
            if ch == 20:
                if not vs:
                    msg, msg_ok = t("No vouchers."), False
                    continue
                v = vs[sel] if isinstance(vs[sel], dict) else {}
                code = str(v.get("code",""))
                if not code:
                    msg, msg_ok = t("No vouchers."), False
                    continue
                if clipboard_copy(code):
                    msg, msg_ok = t("Voucher copied") + ": " + code, True
                else:
                    msg, msg_ok = t("Clipboard unavailable"), False
                continue
            if ch in (curses.KEY_UP,ord('k')):
                sel = max(0, sel-1); msg = ""; continue
            if ch in (curses.KEY_DOWN,ord('j'),9):
                sel = min(max(0,len(vs)-1), sel+1); msg = ""; continue
            if ch in (ord('g'),ord('G')):
                ans = self.edit(t("Validity (days)"), "")
                if ans is None:
                    continue
                try:
                    days = int(str(ans).strip() or "30")
                except Exception:
                    msg, msg_ok = t("Days must be a number."), False
                    continue
                if not 1 <= days <= 36500:
                    msg, msg_ok = t("Days must be 1..36500."), False
                    continue
                msg, msg_ok = t("Working..."), True
                self.paint(h-1,2,msg,self.c(7)); self.scr.refresh()
                line = control_command("vouchermk %d" % days, timeout=15)
                if line.startswith("+OK"):
                    code = line[3:].strip()
                    now = datetime.datetime.now()
                    self.st["vouchers"].append({
                        "code": code,
                        "created": now.strftime("%Y-%m-%d %H:%M"),
                        "days": days,
                        "expires": (now + datetime.timedelta(days=days)).strftime("%Y-%m-%d"),
                    })
                    state_save(self.st)
                    sel = len(self.st["vouchers"])-1
                    msg, msg_ok = t("Voucher created") + ": " + code, True
                else:
                    msg, msg_ok = line or "vouchermk failed", False
            elif ch in (ord('d'),ord('D'),curses.KEY_DC) and vs:
                v = vs[sel] if isinstance(vs[sel], dict) else {}
                code = str(v.get("code",""))
                if not code:
                    continue
                if not self.confirm(t("Delete this voucher?")):
                    continue
                msg, msg_ok = t("Working..."), True
                self.paint(h-1,2,msg,self.c(7)); self.scr.refresh()
                line = control_command("voucher " + code, timeout=15)
                if line.startswith("+OK"):
                    vs.pop(sel)
                    state_save(self.st)
                    msg, msg_ok = t("Voucher deleted") + ": " + code, True
                else:
                    msg, msg_ok = t("Delete failed") + ": " + (line or "-"), False
            elif ch in (ord('s'),ord('S')):
                state_save(self.st)
                msg, msg_ok = t("Saved."), True

    def log_screen(self):
        text = read(TOR_LOG)
        lines = text.splitlines()[-300:]
        off = max(0, len(lines)-20)
        while True:
            self.fill(self.c(1))
            h,w = self.scr.getmaxyx()
            self.paint(1,2,t("Tor log"),self.c(4)|curses.A_BOLD)
            for i,line in enumerate(lines[off:off+max(1,h-5)]):
                self.paint(3+i,2,line[:w-3],self.c(1))
            self.paint(h-1,2,"↑↓ PgUp/PgDn  Q",self.c(4))
            self.scr.refresh()
            ch = self.scr.getch()
            if ch in (ord('q'),ord('Q'),27): return
            if ch in (curses.KEY_UP,ord('k')): off=max(0,off-1)
            elif ch in (curses.KEY_DOWN,ord('j')): off=min(max(0,len(lines)-1),off+1)
            elif ch == curses.KEY_PPAGE: off=max(0,off-20)
            elif ch == curses.KEY_NPAGE: off=min(max(0,len(lines)-1),off+20)

    def set_value(self, it, val):
        st = self.st
        key = it[2]
        if key in ("server_name","onion","nick","locale","admin_password"): st[key] = val
        elif key in ("relay","admin_enabled","admin_captcha"): st[key] = bool(val)
        elif key in st["limits"]:
            if key in ("debug","autodelete","exit","logvoucher"): st["limits"][key] = bool(val)
            else:
                try: st["limits"][key] = int(val)
                except Exception: pass
        elif key.startswith("reg_"): st["registration"][key[4:]] = bool(val)
        elif key in ("voucher_required","exit_name_enabled","exit_enabled"): st["registration"][key] = bool(val)
        elif key == "exit_name": st["registration"][key] = val
        elif key == "default_theme": st.setdefault("design", {})["theme"] = val
        elif key.startswith("tor_"):
            k = key[4:]
            if k == "mode": st["tor"]["mode"] = val
            elif k == "bridges": st["tor"]["bridges"] = bool(val)
            elif k == "bridge_lines": st["tor"]["bridge_lines"] = val
            elif k in ("ip","internal_ip"): st["tor"][k] = val
            elif k in ("port","internal_port"):
                try: st["tor"][k] = int(val)
                except Exception: pass

    def handle(self, it):
        label, kind, key = it[0], it[1], it[2]
        if kind == "action":
            if key == "save":
                ok, errs = save(self.st, "tor" if self.cat == 5 else "")
                if ok:
                    self.msg, self.msg_ok = t("Saved.") + " " + t("Restart required for some settings."), True
                else:
                    self.msg, self.msg_ok = t("Validation error") + ": " + "; ".join(errs[:4]), False
            elif key == "get_onion":
                v = get_onion(self.st)
                if v:
                    self.st["onion"] = v
                    self.msg, self.msg_ok = t("Saved."), True
                else:
                    self.msg, self.msg_ok = "Onion address not found", False
            elif key == "add_friend":
                v = self.edit("Friend", "")
                if v and re.match(r"^[a-z2-7]{16,56}\.onion$", v.strip()):
                    if v.strip() not in self.st["friends"]: self.st["friends"].append(v.strip())
                    self.msg, self.msg_ok = t("Saved."), True
                elif v:
                    self.msg, self.msg_ok = "friend: invalid onion", False
            elif key == "add_logo":
                v = self.edit("Logo path", "")
                if v:
                    ok, msg = install_logo(self.st, v)
                    self.msg, self.msg_ok = msg, ok
            elif key == "reset_logo":
                ok, msg = reset_logo(self.st)
                self.msg, self.msg_ok = msg, ok
            elif key == "vouchers":
                self.voucher_screen()
            elif key == "tor_log":
                self.log_screen()
            elif key == "tor_restart":
                if self.st["tor"].get("mode") == "internal":
                    ok = start_tor(self.st)
                    self.msg, self.msg_ok = ("Tor restarted" if ok else "Tor failed; see log"), ok
                else:
                    self.msg, self.msg_ok = "Restart is available only for internal Tor", False
            return
        if kind == "friend":
            if self.confirm(t("Delete this friend?")):
                try: self.st["friends"].remove(key)
                except ValueError: pass
                self.msg, self.msg_ok = t("Saved."), True
            return
        if kind == "bool":
            if key in self.st:
                self.st[key] = not self.st[key]
            elif key.startswith("reg_"):
                self.st["registration"][key[4:]] = not self.st["registration"].get(key[4:])
            elif key in self.st["registration"]:
                self.st["registration"][key] = not self.st["registration"][key]
            elif key in self.st["limits"]:
                self.st["limits"][key] = not self.st["limits"][key]
            elif key.startswith("tor_") and key[4:] in self.st["tor"]:
                self.st["tor"][key[4:]] = not self.st["tor"][key[4:]]
            return
        if kind == "choice" and key == "tor_mode":
            modes = ["external","internal","off"]
            cur = modes.index(self.st["tor"].get("mode","external"))
            r = self.choose("Tor mode", ["External Tor","Internal Tor","Disabled"], cur)
            if r is not None:
                self.st["tor"]["mode"] = modes[r]
                self.sel = 0
            return
        if kind == "choice" and key == "default_theme":
            modes = ["light", "dark"]
            cur = modes.index(self.st.get("design", {}).get("theme", "light"))
            r = self.choose("Default theme", ["Light", "Dark"], cur)
            if r is not None:
                self.st.setdefault("design", {})["theme"] = modes[r]
            return
        if kind in ("text","pass","int"):
            cur = self.value_for(it)
            if key == "admin_password": cur = ""
            new = self.edit(label, cur, secret=(kind == "pass"))
            if new is None: return
            self.set_value(it, new)
            self.msg = ""

    def run(self):
        self.language_screen()
        while True:
            self.draw()
            try: ch = self.scr.getch()
            except KeyboardInterrupt: ch = 3
            if ch == 3:
                r = self.choose(t("Exit with saving?"), ["Save and exit","Exit without saving","Cancel"], 0)
                if r == 0:
                    ok, errs = save(self.st, "tor" if self.cat == 5 else "")
                    if ok: return 0
                    self.msg, self.msg_ok = t("Validation error")+": "+"; ".join(errs[:3]), False
                elif r == 1:
                    return 0
                continue
            if ch == 19:
                ok, errs = save(self.st, "tor" if self.cat == 5 else "")
                self.msg, self.msg_ok = (t("Saved."), True) if ok else (t("Validation error")+": "+"; ".join(errs[:3]), False)
                continue
            if ch == curses.KEY_LEFT:
                self.cat = (self.cat-1) % len(self.cats); self.sel=0; self.scroll=0; continue
            if ch == curses.KEY_RIGHT:
                self.cat = (self.cat+1) % len(self.cats); self.sel=0; self.scroll=0; continue
            if ch in (curses.KEY_UP,):
                self.sel = max(0,self.sel-1); continue
            if ch in (curses.KEY_DOWN,):
                self.sel = min(len(self.items())-1,self.sel+1); continue
            if ch == 9:
                self.sel = (self.sel+1) % len(self.items()); continue
            if ch in (10,13,curses.KEY_ENTER):
                items=self.items()
                if items: self.handle(items[min(self.sel,len(items)-1)])

def onionmail_pids():
    pids = []
    for name in os.listdir("/proc"):
        if not name.isdigit():
            continue
        try:
            cmd = open("/proc/%s/cmdline" % name, "rb").read().replace(b"\0", b" ").decode("utf-8", "replace")
        except OSError:
            continue
        if "java" in cmd and "onionmail -M:server" in cmd:
            pids.append(int(name))
    return pids

def restart_cmd():
    pids = onionmail_pids()
    if not pids:
        print("OnionMail server is not running; restarting container", flush=True)
        try:
            os.kill(1, signal.SIGTERM)
        except OSError as exc:
            print("restart failed: %s" % exc, file=sys.stderr)
            return 1
        return 0
    print("OnionMail restart requested", flush=True)
    for pid in pids:
        try:
            os.kill(pid, signal.SIGTERM)
        except OSError:
            pass
    return 0

def status_cmd():
    st = load_state()
    live = derive_state()
    conf = find_server_conf(live.get("nick")) or find_server_conf()
    lim = live.get("limits", {})
    reg = st.get("registration", {})
    print("config:  " + (conf or "-"))
    print("nick:    " + str(live.get("nick", "")))
    print("server:  " + str(live.get("server_name", "")))
    print("onion:   " + str(live.get("onion", "")))
    print("lang:    " + str(st.get("locale") or live.get("locale", "")))
    print("theme:   " + str(st.get("design", {}).get("theme", "light")))
    print("limits:  mail=%s msg=%sMB box=%sMB ml=%s rcpt=%s spam=%s" % (
        lim.get("max_messages", ""), lim.get("max_message_mb", ""), lim.get("box_mb", ""),
        lim.get("max_ml", ""), lim.get("max_rcpt", ""), lim.get("max_spam", "")))
    print("flags:   debug=%s autodelete=%s exit=%s logvoucher=%s" % (
        "on" if lim.get("debug") else "off", "on" if lim.get("autodelete") else "off",
        "on" if lim.get("exit") else "off", "on" if lim.get("logvoucher") else "off"))
    print("friends: %d" % len(live.get("friends", [])))
    print("admin:   " + ("enabled" if st.get("admin_enabled") else "disabled") + " (front state)")
    print("reg:     captcha=%s vouchers=%s exit=%s" % (
        "on" if reg.get("captcha", True) else "off",
        "required" if reg.get("voucher_required") else "optional",
        "on" if reg.get("exit_enabled", True) else "off"))
    print("vouchers: %d" % len(st.get("vouchers", []) or []))
    tor = live.get("tor", {})
    mode = str(st.get("tor", {}).get("mode", tor.get("mode", "")))
    ip = parse_kv(read(CORE_CONF), "TorIP") or tor.get("ip", "127.0.0.1")
    port = parse_kv(read(CORE_CONF), "TorPort") or tor.get("port", 9050)
    endpoint = "%s:%s" % (ip, port)
    if mode == "internal":
        state = tor_progress()
    elif mode == "external":
        state = "reachable" if tor_socket_ok(ip, port) else "unreachable"
    else:
        state = "disabled"
    print("tor:     " + mode + " " + endpoint + " " + state)
    drift = []
    checks = [
        ("nick", st.get("nick"), live.get("nick")),
        ("server", st.get("server_name"), live.get("server_name")),
        ("onion", st.get("onion"), live.get("onion")),
        ("relay", bool(st.get("relay")), bool(live.get("relay"))),
    ]
    for key in ("max_messages","max_message_mb","box_mb","max_ml","max_rcpt","max_spam"):
        checks.append((key, st.get("limits", {}).get(key), lim.get(key)))
    expected_port = st.get("tor", {}).get("internal_port" if mode == "internal" else "port", port)
    checks.append(("tor_port", expected_port, port))
    for name, a, b in checks:
        if str(a) != str(b):
            drift.append(name)
    print("sync:    " + ("om.json matches current config" if not drift else "differs: " + ", ".join(drift)))
    print("state:   " + STATE_PATH)

def main(argv):
    global ui_lang
    if os.geteuid() != 0:
        print("om: root privileges required (uid 0)", file=sys.stderr)
        return 1
    cmd = argv[1] if len(argv) > 1 else "help"
    if cmd in ("-h","--help","help"):
        print("Usage: om [setup|status|restart|version|check]")
        print("  setup    interactive purple TUI")
        print("  status   show current state")
        print("  restart  restart OnionMail (container restarts)")
        print("  check    validate configuration")
        return 0
    if cmd == "__boot-env":
        boot_env(); return 0
    if cmd == "__apply":
        apply_state(load_state()); return 0
    if cmd == "__torrc":
        write_torrc(load_state()); return 0
    if cmd == "status":
        status_cmd(); return 0
    if cmd == "version":
        print("om 1.0 / OnionMail 1.12"); return 0
    if cmd == "check":
        st = load_state(); errs = validate(st) + tor_config_errors(st)
        print("\n".join(errs) if errs else "OK"); return 0 if not errs else 2
    if cmd == "restart":
        return restart_cmd()
    if cmd in ("setup",""):
        st = load_state()
        ui_lang = st.get("lang", "ru")
        try:
            def wrapped(scr):
                curses.raw()
                curses.noecho()
                curses.curs_set(0)
                scr.keypad(True)
                return UI(scr, st).run()
            return curses.wrapper(wrapped) or 0
        except Exception as exc:
            print("om setup: %s" % exc, file=sys.stderr)
            return 1
    print("om: unknown command %s" % cmd, file=sys.stderr)
    return 1

if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv))
    except KeyboardInterrupt:
        sys.exit(130)
