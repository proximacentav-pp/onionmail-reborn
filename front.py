#!/usr/bin/env python3
"""OnionMail front-end: original site proxy + small webmail.

- "/" and every original OnionMail page (registration with captcha and
  voucher codes, info, admin) are proxied to OnionMail's built-in HTTP
  server on 127.0.0.1:18001.
- "/web" is a Gmail-like webmail in OnionMail colours. It authenticates
  with the mailbox username + SMTP password + POP3 password and talks to
  the local POP3/SMTP servers.
- "/api/config.api" is adjusted so the info page shows the operator's
  advertised limits.
"""

import base64
import email
import email.policy
import email.utils
import html
import http.client
import json
import os
import re
import secrets
import smtplib
import socket
import ssl
import sys
import threading
import time
from email.header import decode_header, make_header
from email.message import EmailMessage
from email.parser import BytesParser
from html.parser import HTMLParser
from http.cookies import SimpleCookie
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote, urlsplit, urlencode

LANG_COOKIE = "omlang"
THEME_COOKIE = "omtheme"
THEMES = ("light", "dark")
DEFAULT_THEME = os.environ.get("ONIONMAIL_THEME", "light").strip().lower()
if DEFAULT_THEME not in THEMES:
    DEFAULT_THEME = "light"
LANGS = ("en", "de", "fr", "ru", "es")
LANG_NAMES = {
    "en": "English",
    "de": "Deutsch",
    "fr": "Français",
    "ru": "Русский",
    "es": "Español",
}
_lang_local = threading.local()

I18N = {
    "de": {
        "Inbox": "Posteingang",
        "Sent": "Gesendet",
        "New message": "Neue Nachricht",
        "Log out": "Abmelden",
        "OnionMail webmail": "OnionMail Webmail",
        "Sign in to your mailbox on this server.": "Melden Sie sich bei Ihrem Postfach auf diesem Server an.",
        "Username (main login, no @)": "Benutzername (Hauptlogin, ohne @)",
        "e.g. ivan": "z. B. ivan",
        "SMTP password": "SMTP-Passwort",
        "POP3 password": "POP3-Passwort",
        "Sign in": "Anmelden",
        "Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration).": "Der Login ist der Postfachname vor dem @, nicht der Exit-Login. SMTP- und POP3-Passwörter sind unterschiedlich (beide werden nach der Registrierung angezeigt).",
        "No account? Register with a voucher code": "Kein Konto? Mit einem Voucher-Code registrieren",
        "Mail sign-in": "Postfach-Anmeldung",
        "Mailbox sign-in": "Postfach-Anmeldung",
        "Messages: {n}": "Nachrichten: {n}",
        "No messages": "Keine Nachrichten",
        "Could not open mailbox: {err}": "Postfach konnte nicht geöffnet werden: {err}",
        "Could not read message: {err}": "Nachricht konnte nicht gelesen werden: {err}",
        "Attachments: {names}": "Anhänge: {names}",
        "From": "Von",
        "To": "An",
        "Date": "Datum",
        "Subject": "Betreff",
        "Message": "Nachricht",
        "Send": "Senden",
        "Back": "Zurück",
        "No sent messages": "Keine gesendeten Nachrichten",
        "To: {to}": "An: {to}",
        "(no subject)": "(kein Betreff)",
        "(empty message)": "(leere Nachricht)",
        "[... truncated ...]": "[... gekürzt ...]",
        "Message not found.": "Nachricht nicht gefunden.",
        "Message sent.": "Nachricht gesendet.",
        "Not found.": "Nicht gefunden.",
        "Fill in all fields and check the username.": "Füllen Sie alle Felder aus und prüfen Sie den Benutzernamen.",
        "Wrong username or password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login. SMTP and POP3 passwords are different.": "Falscher Benutzername oder falsches Passwort. Im Feld „Benutzername“ den Hauptnamen des Postfachs (Localpart) ohne @ eingeben – nicht den Exit-Login. SMTP- und POP3-Passwörter sind unterschiedlich.",
        "It looks like “{user}” is an exit login. The main login for signing in is “{main}”.": "„{user}“ scheint ein Exit-Login zu sein. Der Hauptlogin für die Anmeldung ist „{main}“.",
        "Invalid exit login": "Ungültiger Exit-Login",
        "Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list.": "Der Exit-Login muss 4–32 Zeichen haben: lateinische Buchstaben, Ziffern, Punkt, Bindestrich oder Unterstrich, und darf nicht auf .onion, .sys, .op, .list enden.",
        "Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.": "Konto erstellt, aber der Exit-Login {exit} wurde nicht hinzugefügt (möglicherweise bereits vergeben). Die onionmail.info-Adresse bleibt {login}@{domain}.",
        "Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.": "Webmail-Anmeldung (/web): Login {login} – ohne @, verwenden Sie die oben angezeigten SMTP- und POP3-Passwörter. Der Exit-Login {exit}@{domain} gilt nur für die Adresse, nicht für die Anmeldung.",
        "Statistics": "Statistik",
        "Net": "Netz",
        "Vouchers": "Voucher",
        "Users": "Benutzer",
        "with POP3 password: {n}": "mit POP3-Passwort: {n}",
        "Messages in mailboxes": "Nachrichten in Postfächern",
        "server received: {n} · {size}": "Server empfangen: {n} · {size}",
        "Sent": "Gesendet",
        "server sent: {n} · {size}": "Server gesendet: {n} · {size}",
        "User data": "Benutzerdaten",
        "usr, keys, registry": "usr, keys, Registry",
        "Service files": "Dienstdateien",
        "indexes, Tor, logs, www": "Indizes, Tor, Logs, www",
        "Free disk": "Freier Speicher",
        "of {size}": "von {size}",
        "Server uptime": "Server-Laufzeit",
        "threads: {n}": "Threads: {n}",
        "Exit / Friends": "Exit / Freunde",
        "spam: {n}": "Spam: {n}",
        "Storage": "Speicher",
        "Total: {total} · free {free}": "Gesamt: {total} · frei {free}",
        "occupied by data": "durch Daten belegt",
        "Top mailboxes by size": "Größte Postfächer",
        "Received + sent": "Empfangen + gesendet",
        "of users": "der Benutzer",
        "No mailbox size data.": "Keine Postfachgrößendaten.",
        "Mail": "Mail",
        "OnionMail counters": "OnionMail-Zähler",
        "messages total": "Nachrichten gesamt",
        "Mailboxes": "Postfächer",
        "OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below.": "OnionMail stellt keine Postfachliste über die API bereit. Das Panel führt ein lokales Register: Neue Registrierungen und /web-Anmeldungen werden automatisch hinzugefügt, weitere können unten manuell ergänzt werden.",
        "Mailbox": "Postfach",
        "Exit login": "Exit-Login",
        "Only for the @{domain} address. To sign in at /web use the main login (without @).": "Nur für die Adresse @{domain}. Für die Anmeldung unter /web den Hauptlogin (ohne @) verwenden.",
        "Inbox": "Posteingang",
        "Size": "Größe",
        "Sent": "Gesendet",
        "Last login": "Letzte Anmeldung",
        "Status": "Status",
        "No mailboxes in the registry yet.": "Noch keine Postfächer im Register.",
        "Add / update mailbox": "Postfach hinzufügen / aktualisieren",
        "Login (no @)": "Login (ohne @)",
        "SMTP password (optional)": "SMTP-Passwort (optional)",
        "Exit login (optional)": "Exit-Login (optional)",
        "Check and save": "Prüfen und speichern",
        "Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}": "Version: {version} · Web: {hits} Anfragen, {errors} Fehler · Neu heute: {new} · Fehler: {errs}",
        "POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.": "POP3-/SMTP-Passwörter werden nur lokal in {path} (0600) gespeichert und für die Messung der Postfachgrößen benötigt.",
        "Mailbox verified via POP3 and saved.": "Postfach per POP3 geprüft und gespeichert.",
        "CSRF error: reload the page and try again.": "CSRF-Fehler: Seite neu laden und erneut versuchen.",
        "Login: 3–16 characters, Latin letters/digits/._-, no @.": "Login: 3–16 Zeichen, lateinische Buchstaben/Ziffern/._-, kein @.",
        "Exit login: 3–30 characters, Latin letters/digits/._-.": "Exit-Login: 3–30 Zeichen, lateinische Buchstaben/Ziffern/._-.",
        "POP3 did not accept login “{login}” with this password.": "POP3 hat den Login „{login}“ mit diesem Passwort nicht akzeptiert.",
        "Received mail": "Empfangene Mail",
        "Sent mail": "Gesendete Mail",
        "Other": "Sonstige",
        "Incoming": "Eingehend",
        "Outgoing": "Ausgehend",
        "no password": "kein Passwort",
        "error": "Fehler",
        "ok": "ok",
        "No password": "Kein Passwort",
        "Error": "Fehler",
        "Language": "Sprache",
        "Light theme": "Helles Design",
        "Dark theme": "Dunkles Design",
        "Register": "Registrieren",
        "Specify a valid recipient.": "Geben Sie einen gültigen Empfänger an.",
        "Invalid subject.": "Ungültiger Betreff.",
        "Message is too large.": "Nachricht ist zu groß.",
        "Could not send: {err}": "Senden fehlgeschlagen: {err}",
        "CSRF error.": "CSRF-Fehler.",
        "Voucher required": "Voucher erforderlich",
        "Registration is available only with a valid voucher code.": "Die Registrierung ist nur mit einem gültigen Voucher-Code möglich.",
    },
    "fr": {
        "Inbox": "Boîte de réception",
        "Sent": "Envoyés",
        "New message": "Nouveau message",
        "Log out": "Se déconnecter",
        "OnionMail webmail": "Webmail OnionMail",
        "Sign in to your mailbox on this server.": "Connectez-vous à votre boîte mail sur ce serveur.",
        "Username (main login, no @)": "Nom d’utilisateur (identifiant principal, sans @)",
        "e.g. ivan": "ex. : ivan",
        "SMTP password": "Mot de passe SMTP",
        "POP3 password": "Mot de passe POP3",
        "Sign in": "Se connecter",
        "Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration).": "L’identifiant est le nom de la boîte avant @, pas l’identifiant exit. Les mots de passe SMTP et POP3 sont différents (les deux sont affichés après l’inscription).",
        "No account? Register with a voucher code": "Pas de compte ? S’inscrire avec un code voucher",
        "Mail sign-in": "Connexion à la messagerie",
        "Mailbox sign-in": "Connexion à la boîte",
        "Messages: {n}": "Messages : {n}",
        "No messages": "Aucun message",
        "Could not open mailbox: {err}": "Impossible d’ouvrir la boîte : {err}",
        "Could not read message: {err}": "Impossible de lire le message : {err}",
        "Attachments: {names}": "Pièces jointes : {names}",
        "From": "De",
        "To": "À",
        "Date": "Date",
        "Subject": "Objet",
        "Message": "Message",
        "Send": "Envoyer",
        "Back": "Retour",
        "No sent messages": "Aucun message envoyé",
        "To: {to}": "À : {to}",
        "(no subject)": "(sans objet)",
        "(empty message)": "(message vide)",
        "[... truncated ...]": "[... tronqué ...]",
        "Message not found.": "Message introuvable.",
        "Message sent.": "Message envoyé.",
        "Not found.": "Introuvable.",
        "Fill in all fields and check the username.": "Remplissez tous les champs et vérifiez le nom d’utilisateur.",
        "Wrong username or password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login. SMTP and POP3 passwords are different.": "Identifiant ou mot de passe incorrect. Dans le champ « Nom d’utilisateur », utilisez le nom principal de la boîte (localpart) sans @ — pas l’identifiant exit. Les mots de passe SMTP et POP3 sont différents.",
        "It looks like “{user}” is an exit login. The main login for signing in is “{main}”.": "« {user} » semble être un identifiant exit. L’identifiant principal pour la connexion est « {main} ».",
        "Invalid exit login": "Identifiant exit invalide",
        "Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list.": "L’identifiant exit doit contenir 4 à 32 caractères : lettres latines, chiffres, point, tiret ou souligné, et ne pas se terminer par .onion, .sys, .op, .list.",
        "Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.": "Compte créé, mais l’identifiant exit {exit} n’a pas été ajouté (peut-être déjà pris). L’adresse onionmail.info reste {login}@{domain}.",
        "Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.": "Connexion webmail (/web) : identifiant {login} — sans @, utilisez les mots de passe SMTP et POP3 affichés ci-dessus. L’identifiant exit {exit}@{domain} sert uniquement pour l’adresse, pas pour la connexion.",
        "Statistics": "Statistiques",
        "Net": "Réseau",
        "Vouchers": "Vouchers",
        "Users": "Utilisateurs",
        "with POP3 password: {n}": "avec mot de passe POP3 : {n}",
        "Messages in mailboxes": "Messages dans les boîtes",
        "server received: {n} · {size}": "reçus par le serveur : {n} · {size}",
        "Sent": "Envoyés",
        "server sent: {n} · {size}": "envoyés par le serveur : {n} · {size}",
        "User data": "Données utilisateurs",
        "usr, keys, registry": "usr, keys, registre",
        "Service files": "Fichiers de service",
        "indexes, Tor, logs, www": "index, Tor, logs, www",
        "Free disk": "Espace libre",
        "of {size}": "sur {size}",
        "Server uptime": "Durée de service",
        "threads: {n}": "threads : {n}",
        "Exit / Friends": "Exit / Amis",
        "spam: {n}": "spam : {n}",
        "Storage": "Stockage",
        "Total: {total} · free {free}": "Total : {total} · libre {free}",
        "occupied by data": "occupé par les données",
        "Top mailboxes by size": "Plus grosses boîtes",
        "Received + sent": "Reçus + envoyés",
        "of users": "des utilisateurs",
        "No mailbox size data.": "Aucune donnée de taille de boîte.",
        "Mail": "Courrier",
        "OnionMail counters": "Compteurs OnionMail",
        "messages total": "messages au total",
        "Mailboxes": "Boîtes",
        "OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below.": "OnionMail n’expose pas de liste de boîtes via l’API. Le panneau tient un registre local : les nouvelles inscriptions et connexions /web sont ajoutées automatiquement, les autres peuvent être ajoutées manuellement ci-dessous.",
        "Mailbox": "Boîte",
        "Exit login": "Identifiant exit",
        "Only for the @{domain} address. To sign in at /web use the main login (without @).": "Uniquement pour l’adresse @{domain}. Pour vous connecter à /web, utilisez l’identifiant principal (sans @).",
        "Inbox": "Réception",
        "Size": "Taille",
        "Sent": "Envoyés",
        "Last login": "Dernière connexion",
        "Status": "État",
        "No mailboxes in the registry yet.": "Aucune boîte dans le registre.",
        "Add / update mailbox": "Ajouter / mettre à jour une boîte",
        "Login (no @)": "Identifiant (sans @)",
        "SMTP password (optional)": "Mot de passe SMTP (facultatif)",
        "Exit login (optional)": "Identifiant exit (facultatif)",
        "Check and save": "Vérifier et enregistrer",
        "Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}": "Version : {version} · Web : {hits} requêtes, {errors} erreurs · Nouveaux aujourd’hui : {new} · Erreurs : {errs}",
        "POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.": "Les mots de passe POP3/SMTP sont stockés uniquement en local dans {path} (0600) et servent à mesurer la taille des boîtes.",
        "Mailbox verified via POP3 and saved.": "Boîte vérifiée via POP3 et enregistrée.",
        "CSRF error: reload the page and try again.": "Erreur CSRF : rechargez la page et réessayez.",
        "Login: 3–16 characters, Latin letters/digits/._-, no @.": "Identifiant : 3–16 caractères, lettres latines/chiffres/._-, sans @.",
        "Exit login: 3–30 characters, Latin letters/digits/._-.": "Identifiant exit : 3–30 caractères, lettres latines/chiffres/._-.",
        "POP3 did not accept login “{login}” with this password.": "POP3 n’a pas accepté l’identifiant « {login} » avec ce mot de passe.",
        "Received mail": "Courrier reçu",
        "Sent mail": "Courrier envoyé",
        "Other": "Autres",
        "Incoming": "Entrants",
        "Outgoing": "Sortants",
        "no password": "sans mot de passe",
        "error": "erreur",
        "ok": "ok",
        "No password": "Sans mot de passe",
        "Error": "Erreur",
        "Language": "Langue",
        "Light theme": "Thème clair",
        "Dark theme": "Thème sombre",
        "Register": "S’inscrire",
        "Specify a valid recipient.": "Indiquez un destinataire valide.",
        "Invalid subject.": "Objet invalide.",
        "Message is too large.": "Message trop volumineux.",
        "Could not send: {err}": "Envoi impossible : {err}",
        "CSRF error.": "Erreur CSRF.",
        "Voucher required": "Voucher requis",
        "Registration is available only with a valid voucher code.": "L’inscription est possible uniquement avec un code voucher valide.",
    },
    "ru": {
        "Inbox": "Полученные",
        "Sent": "Отправленные",
        "New message": "Новое письмо",
        "Log out": "Выйти",
        "OnionMail webmail": "OnionMail веб-почта",
        "Sign in to your mailbox on this server.": "Войдите в свой почтовый ящик на этом сервере.",
        "Username (main login, no @)": "Имя пользователя (основной логин, без @)",
        "e.g. ivan": "например: ivan",
        "SMTP password": "Пароль SMTP",
        "POP3 password": "Пароль POP3",
        "Sign in": "Войти",
        "Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration).": "Логин — это имя ящика до @, не exit-логин. Пароли SMTP и POP3 разные (оба есть на странице после регистрации).",
        "No account? Register with a voucher code": "Нет аккаунта? Регистрация по voucher-коду",
        "Mail sign-in": "Вход в почту",
        "Mailbox sign-in": "Вход в почту",
        "Messages: {n}": "Писем: {n}",
        "No messages": "Писем нет",
        "Could not open mailbox: {err}": "Не удалось открыть ящик: {err}",
        "Could not read message: {err}": "Не удалось прочитать письмо: {err}",
        "Attachments: {names}": "Вложения: {names}",
        "From": "От",
        "To": "Кому",
        "Date": "Дата",
        "Subject": "Тема",
        "Message": "Сообщение",
        "Send": "Отправить",
        "Back": "Назад",
        "No sent messages": "Отправленных писем нет",
        "To: {to}": "Кому: {to}",
        "(no subject)": "(без темы)",
        "(empty message)": "(пустое сообщение)",
        "[... truncated ...]": "[... обрезано ...]",
        "Message not found.": "Письмо не найдено.",
        "Message sent.": "Письмо отправлено.",
        "Not found.": "Не найдено.",
        "Fill in all fields and check the username.": "Заполните все поля и проверьте имя пользователя.",
        "Wrong username or password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login. SMTP and POP3 passwords are different.": "Неверный логин или пароль. В поле «Имя пользователя» нужно основное имя ящика (localpart) без @ — не exit-логин. Пароли SMTP и POP3 различаются.",
        "It looks like “{user}” is an exit login. The main login for signing in is “{main}”.": "Похоже, «{user}» — это exit-логин. Основной логин для входа: «{main}».",
        "Invalid exit login": "Некорректный exit login",
        "Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list.": "Exit login должен состоять из 4–32 символов: латинские буквы, цифры, точка, дефис или подчёркивание, и не может заканчиваться на .onion, .sys, .op, .list.",
        "Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.": "Аккаунт создан, но exit login {exit} не добавлен (возможно, он уже занят). Адрес в onionmail.info остаётся {login}@{domain}.",
        "Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.": "Вход в веб-почту (/web): логин {login} — без @, пароли SMTP и POP3 те, что показаны выше. Exit-логин {exit}@{domain} — только для адреса, для входа не используется.",
        "Statistics": "Статистика",
        "Net": "Сеть",
        "Vouchers": "Vouchers",
        "Users": "Пользователей",
        "with POP3 password: {n}": "с паролем POP3: {n}",
        "Messages in mailboxes": "Писем в ящиках",
        "server received: {n} · {size}": "сервер принял: {n} · {size}",
        "Sent": "Отправлено",
        "server sent: {n} · {size}": "сервер отправил: {n} · {size}",
        "User data": "Данные пользователей",
        "usr, keys, registry": "usr, keys, реестр",
        "Service files": "Служебные файлы",
        "indexes, Tor, logs, www": "индексы, Tor, логи, www",
        "Free disk": "Свободно на диске",
        "of {size}": "из {size}",
        "Server uptime": "Uptime сервера",
        "threads: {n}": "потоки: {n}",
        "Exit / Friends": "Exit / Friends",
        "spam: {n}": "спам: {n}",
        "Storage": "Хранилище",
        "Total: {total} · free {free}": "Всего: {total} · свободно {free}",
        "occupied by data": "занято данными",
        "Top mailboxes by size": "Топ ящиков по объёму",
        "Received + sent": "Полученные + отправленные",
        "of users": "у пользователей",
        "No mailbox size data.": "Нет данных по размерам ящиков.",
        "Mail": "Почта",
        "OnionMail counters": "Счётчики OnionMail",
        "messages total": "писем всего",
        "Mailboxes": "Ящики",
        "OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below.": "OnionMail не отдаёт список ящиков через API. Панель ведёт локальный реестр: новые регистрации и входы в /web добавляются автоматически, остальные можно добавить вручную ниже.",
        "Mailbox": "Ящик",
        "Exit login": "Exit-логин",
        "Only for the @{domain} address. To sign in at /web use the main login (without @).": "Только для адреса @{domain}. Для входа в /web используйте основной логин (без @).",
        "Inbox": "Входящие",
        "Size": "Размер",
        "Sent": "Отправлено",
        "Last login": "Последний вход",
        "Status": "Статус",
        "No mailboxes in the registry yet.": "Ящики ещё не добавлены в реестр.",
        "Add / update mailbox": "Добавить / обновить ящик",
        "Login (no @)": "Логин (без @)",
        "SMTP password (optional)": "SMTP-пароль (необязательно)",
        "Exit login (optional)": "Exit-логин (необязательно)",
        "Check and save": "Проверить и сохранить",
        "Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}": "Версия: {version} · Веб: {hits} запросов, {errors} ошибок · Новых за сутки: {new} · Ошибки: {errs}",
        "POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.": "Пароли POP3/SMTP хранятся только локально в {path} (0600) и нужны для подсчёта размеров ящиков.",
        "Mailbox verified via POP3 and saved.": "Ящик проверен по POP3 и сохранён.",
        "CSRF error: reload the page and try again.": "CSRF error: обновите страницу и попробуйте снова.",
        "Login: 3–16 characters, Latin letters/digits/._-, no @.": "Логин: 3–16 символов, латиница/цифры/._-, без @.",
        "Exit login: 3–30 characters, Latin letters/digits/._-.": "Exit-логин: 3–30 символов, латиница/цифры/._-.",
        "POP3 did not accept login “{login}” with this password.": "POP3 не принял логин «{login}» с этим паролем.",
        "Received mail": "Полученные письма",
        "Sent mail": "Отправленные письма",
        "Other": "Остальные",
        "Incoming": "Входящие",
        "Outgoing": "Исходящие",
        "no password": "нет пароля",
        "error": "ошибка",
        "ok": "ok",
        "No password": "Нет пароля",
        "Error": "Ошибка",
        "Language": "Язык",
        "Light theme": "Светлая тема",
        "Dark theme": "Тёмная тема",
        "Register": "Регистрация",
        "Specify a valid recipient.": "Укажите корректного получателя.",
        "Invalid subject.": "Некорректная тема.",
        "Message is too large.": "Слишком большое письмо.",
        "Could not send: {err}": "Не удалось отправить: {err}",
        "CSRF error.": "CSRF error.",
        "Voucher required": "Требуется ваучер",
        "Registration is available only with a valid voucher code.": "Регистрация возможна только с действующим voucher-кодом.",
    },
    "es": {
        "Inbox": "Recibidos",
        "Sent": "Enviados",
        "New message": "Nuevo mensaje",
        "Log out": "Salir",
        "OnionMail webmail": "Webmail de OnionMail",
        "Sign in to your mailbox on this server.": "Inicie sesión en su buzón de este servidor.",
        "Username (main login, no @)": "Usuario (login principal, sin @)",
        "e.g. ivan": "p. ej.: ivan",
        "SMTP password": "Contraseña SMTP",
        "POP3 password": "Contraseña POP3",
        "Sign in": "Entrar",
        "Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration).": "El login es el nombre del buzón antes de @, no el login exit. Las contraseñas SMTP y POP3 son distintas (ambas se muestran tras el registro).",
        "No account? Register with a voucher code": "¿No tiene cuenta? Regístrese con un código voucher",
        "Mail sign-in": "Acceso al correo",
        "Mailbox sign-in": "Acceso al buzón",
        "Messages: {n}": "Mensajes: {n}",
        "No messages": "No hay mensajes",
        "Could not open mailbox: {err}": "No se pudo abrir el buzón: {err}",
        "Could not read message: {err}": "No se pudo leer el mensaje: {err}",
        "Attachments: {names}": "Adjuntos: {names}",
        "From": "De",
        "To": "Para",
        "Date": "Fecha",
        "Subject": "Asunto",
        "Message": "Mensaje",
        "Send": "Enviar",
        "Back": "Volver",
        "No sent messages": "No hay mensajes enviados",
        "To: {to}": "Para: {to}",
        "(no subject)": "(sin asunto)",
        "(empty message)": "(mensaje vacío)",
        "[... truncated ...]": "[... truncado ...]",
        "Message not found.": "Mensaje no encontrado.",
        "Message sent.": "Mensaje enviado.",
        "Not found.": "No encontrado.",
        "Fill in all fields and check the username.": "Rellene todos los campos y compruebe el usuario.",
        "Wrong username or password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login. SMTP and POP3 passwords are different.": "Usuario o contraseña incorrectos. En «Usuario» use el nombre principal del buzón (localpart) sin @, no el login exit. Las contraseñas SMTP y POP3 son distintas.",
        "It looks like “{user}” is an exit login. The main login for signing in is “{main}”.": "Parece que «{user}» es un login exit. El login principal para entrar es «{main}».",
        "Invalid exit login": "Login exit no válido",
        "Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list.": "El login exit debe tener 4–32 caracteres: letras latinas, dígitos, punto, guion o guion bajo, y no puede terminar en .onion, .sys, .op, .list.",
        "Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.": "Cuenta creada, pero el login exit {exit} no se añadió (puede que ya esté ocupado). La dirección de onionmail.info sigue siendo {login}@{domain}.",
        "Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.": "Acceso al webmail (/web): usuario {login}, sin @, use las contraseñas SMTP y POP3 mostradas arriba. El login exit {exit}@{domain} es solo para la dirección, no para entrar.",
        "Statistics": "Estadísticas",
        "Net": "Red",
        "Vouchers": "Vouchers",
        "Users": "Usuarios",
        "with POP3 password: {n}": "con contraseña POP3: {n}",
        "Messages in mailboxes": "Mensajes en buzones",
        "server received: {n} · {size}": "recibidos por el servidor: {n} · {size}",
        "Sent": "Enviados",
        "server sent: {n} · {size}": "enviados por el servidor: {n} · {size}",
        "User data": "Datos de usuarios",
        "usr, keys, registry": "usr, keys, registro",
        "Service files": "Archivos de servicio",
        "indexes, Tor, logs, www": "índices, Tor, logs, www",
        "Free disk": "Espacio libre",
        "of {size}": "de {size}",
        "Server uptime": "Tiempo activo",
        "threads: {n}": "hilos: {n}",
        "Exit / Friends": "Exit / Amigos",
        "spam: {n}": "spam: {n}",
        "Storage": "Almacenamiento",
        "Total: {total} · free {free}": "Total: {total} · libre {free}",
        "occupied by data": "ocupado por datos",
        "Top mailboxes by size": "Buzones más grandes",
        "Received + sent": "Recibidos + enviados",
        "of users": "de usuarios",
        "No mailbox size data.": "No hay datos de tamaño de buzones.",
        "Mail": "Correo",
        "OnionMail counters": "Contadores de OnionMail",
        "messages total": "mensajes en total",
        "Mailboxes": "Buzones",
        "OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below.": "OnionMail no expone una lista de buzones por API. El panel mantiene un registro local: los nuevos registros e inicios de sesión en /web se añaden automáticamente; los demás pueden añadirse abajo manualmente.",
        "Mailbox": "Buzón",
        "Exit login": "Login exit",
        "Only for the @{domain} address. To sign in at /web use the main login (without @).": "Solo para la dirección @{domain}. Para entrar en /web use el login principal (sin @).",
        "Inbox": "Entrada",
        "Size": "Tamaño",
        "Sent": "Enviados",
        "Last login": "Último acceso",
        "Status": "Estado",
        "No mailboxes in the registry yet.": "Aún no hay buzones en el registro.",
        "Add / update mailbox": "Añadir / actualizar buzón",
        "Login (no @)": "Usuario (sin @)",
        "SMTP password (optional)": "Contraseña SMTP (opcional)",
        "Exit login (optional)": "Login exit (opcional)",
        "Check and save": "Comprobar y guardar",
        "Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}": "Versión: {version} · Web: {hits} solicitudes, {errors} errores · Nuevos hoy: {new} · Errores: {errs}",
        "POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.": "Las contraseñas POP3/SMTP se guardan solo localmente en {path} (0600) y se necesitan para medir el tamaño de los buzones.",
        "Mailbox verified via POP3 and saved.": "Buzón verificado por POP3 y guardado.",
        "CSRF error: reload the page and try again.": "Error CSRF: recargue la página e inténtelo de nuevo.",
        "Login: 3–16 characters, Latin letters/digits/._-, no @.": "Usuario: 3–16 caracteres, letras latinas/dígitos/._-, sin @.",
        "Exit login: 3–30 characters, Latin letters/digits/._-.": "Login exit: 3–30 caracteres, letras latinas/dígitos/._-.",
        "POP3 did not accept login “{login}” with this password.": "POP3 no aceptó el usuario «{login}» con esta contraseña.",
        "Received mail": "Correo recibido",
        "Sent mail": "Correo enviado",
        "Other": "Otros",
        "Incoming": "Entrantes",
        "Outgoing": "Salientes",
        "no password": "sin contraseña",
        "error": "error",
        "ok": "ok",
        "No password": "Sin contraseña",
        "Error": "Error",
        "Language": "Idioma",
        "Light theme": "Tema claro",
        "Dark theme": "Tema oscuro",
        "Register": "Registrarse",
        "Specify a valid recipient.": "Indique un destinatario válido.",
        "Invalid subject.": "Asunto no válido.",
        "Message is too large.": "El mensaje es demasiado grande.",
        "Could not send: {err}": "No se pudo enviar: {err}",
        "CSRF error.": "Error CSRF.",
        "Voucher required": "Voucher obligatorio",
        "Registration is available only with a valid voucher code.": "El registro solo está disponible con un código voucher válido.",
    },
}


def current_lang():
    lang = getattr(_lang_local, "lang", "en")
    return lang if lang in LANGS else "en"


def current_theme():
    theme = getattr(_lang_local, "theme", DEFAULT_THEME)
    return theme if theme in THEMES else DEFAULT_THEME


def default_theme():
    design = om_state().get("design")
    if isinstance(design, dict):
        value = str(design.get("theme", "")).lower()
        if value in THEMES:
            return value
    return DEFAULT_THEME


def set_request_lang(handler):
    lang = "en"
    theme = DEFAULT_THEME
    try:
        cookie = SimpleCookie()
        cookie.load(handler.headers.get("Cookie", ""))
        value = cookie.get(LANG_COOKIE)
        if value and value.value in LANGS:
            lang = value.value
        value = cookie.get(THEME_COOKIE)
        if value and value.value in THEMES:
            theme = value.value
        else:
            theme = default_theme()
    except Exception:
        pass
    _lang_local.lang = lang
    _lang_local.theme = theme if theme in THEMES else DEFAULT_THEME
    _lang_local.path = handler.path or "/"
    return lang


def tr(text, **kwargs):
    value = I18N.get(current_lang(), {}).get(text, text)
    if kwargs:
        try:
            value = value.format(**kwargs)
        except Exception:
            pass
    return value


def lang_selector():
    lang = current_lang()
    options = "".join(
        f'<option value="{code}"{" selected" if code == lang else ""}>{html.escape(name)}</option>'
        for code, name in LANG_NAMES.items()
    )
    next_path = getattr(_lang_local, "path", "/")
    return (
        '<form class="langbox" method="get" action="/lang">'
        f'<label for="om-lang-select">{html.escape(tr("Language"))}</label>'
        f'<select id="om-lang-select" name="l" onchange="this.form.submit()">{options}</select>'
        f'<input type="hidden" name="next" value="{html.escape(next_path)}">'
        "</form>"
    )


def theme_toggle():
    theme = current_theme()
    other = "light" if theme == "dark" else "dark"
    icon = "sun.svg" if theme == "dark" else "moon.svg"
    label = tr("Light theme") if theme == "dark" else tr("Dark theme")
    next_path = getattr(_lang_local, "path", "/")
    return (
        '<form class="themebox" method="get" action="/theme">'
        f'<input type="hidden" name="t" value="{other}">'
        f'<input type="hidden" name="next" value="{html.escape(next_path)}">'
        f'<button type="submit" title="{html.escape(label)}" aria-label="{html.escape(label)}">'
        f'<img src="/theme-assets/{icon}" alt="" width="18" height="18"></button>'
        "</form>"
    )


def prefs_selector():
    return f'<div class="om-prefs">{lang_selector()}{theme_toggle()}</div>'


NICK = os.environ.get("ONIONMAIL_NICK", "onionmail")
DISPLAY_NAME = os.environ.get("ONIONMAIL_DISPLAY_NAME", "example").strip() or "example"
FRONT_PORT = int(os.environ.get("WEB_PORT", "18000"))
FRONT_BIND = os.environ.get("WEB_BIND", "127.0.0.1").strip() or "127.0.0.1"
UPSTREAM = ("127.0.0.1", 18001)
SMTP_PORT = 20025
POP3_PORT = 20110
SERVER_DIR = f"/etc/onionmail/servers.d/1-{NICK}.conf"
CONTROL_CONF = "/etc/onionmail/control.conf"
SENT_DIR = "/var/lib/onionmail/webmail/sent"
MAILDIR = f"/var/onionmail/{NICK}"
REGISTRY_FILE = "/var/lib/onionmail/webmail/registry.json"
SESSION_TTL = 3600
MAX_BODY = 200_000
COOKIE = "omweb"
EXIT_DOMAIN = "onionmail.info"
ALIAS_RE = re.compile(r"^[a-z0-9][a-z0-9._-]{2,30}[a-z0-9]$")

REG_LOGIN_ROW = b'<tr><td>Login:</td><td><input type="text" name="om-localpart" size="40"></td></tr>'
CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE
OM_STATE_PATH = "/etc/onionmail/om.json"
DEFAULT_LOGO = "/img/OM-BL.gif"
LOGO_RE = re.compile(r"^/img/[A-Za-z0-9._-]+$")
THEME_DIR = os.environ.get("ONIONMAIL_THEME_DIR", "/usr/local/share/onionmail/theme")
THEME_ASSETS = {
    "CIEU_white.png": "image/png",
    "CIED_white.png": "image/png",
    "CIEU_black.png": "image/png",
    "CIED_black.png": "image/png",
    "sun.svg": "image/svg+xml",
    "moon.svg": "image/svg+xml",
}
_om_state_lock = threading.Lock()
_om_state_cache = {"mtime": None, "value": {}}


def om_state():
    try:
        mtime = os.stat(OM_STATE_PATH).st_mtime_ns
    except OSError:
        return {}
    with _om_state_lock:
        if _om_state_cache["mtime"] == mtime:
            return _om_state_cache["value"]
        try:
            with open(OM_STATE_PATH, "r", encoding="utf-8") as fh:
                value = json.load(fh)
            if not isinstance(value, dict):
                value = {}
        except Exception:
            value = {}
        _om_state_cache.update({"mtime": mtime, "value": value})
        return value


def om_state_save(state):
    try:
        tmp = OM_STATE_PATH + ".fronttmp"
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(state, fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        os.chmod(tmp, 0o600)
        os.replace(tmp, OM_STATE_PATH)
    except Exception:
        pass


def om_admin_enabled():
    return bool(om_state().get("admin_enabled", True))


def om_registration():
    value = om_state().get("registration")
    return value if isinstance(value, dict) else {}


def om_limits():
    lim = om_state().get("limits")
    if not isinstance(lim, dict):
        lim = {}
    return {
        "maxmsgsize": int(lim.get("max_message_mb", 256)) * 1024,
        "boxsize": int(lim.get("box_mb", 4096)) * 1024,
        "maxmlsize": int(lim.get("max_ml", 4096)),
        "maxmultimsg": int(lim.get("max_rcpt", 24)),
        "maxspam": int(lim.get("max_spam", 196)),
        "maxmsgxusr": int(lim.get("max_messages", 256)),
        "debug": bool(lim.get("debug", False)),
        "autodelete": bool(lim.get("autodelete", False)),
    }

_sessions = {}
_sessions_lock = threading.Lock()
_sent_lock = threading.Lock()


def read_onion():
    with open(SERVER_DIR, "r", encoding="utf-8", errors="replace") as fh:
        for line in fh:
            parts = line.split()
            if len(parts) >= 2 and parts[0].lower() == "onion":
                return parts[1]
    raise RuntimeError("onion address not found")


ONION = read_onion()


def control_command(command, timeout=10):
    """Run a single command on OnionMail's local control port."""
    service_pass = None
    ctrl_port = 9010
    try:
        with open(SERVER_DIR, "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and parts[0].lower() == "passwd":
                    service_pass = parts[1]
                    break
        with open(CONTROL_CONF, "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and parts[0].lower() == "controlport":
                    ctrl_port = int(parts[1])
                    break
    except OSError as exc:
        raise RuntimeError(f"cannot read the control configuration: {exc}")
    if not service_pass:
        raise RuntimeError("service password not found")

    with socket.create_connection(("127.0.0.1", ctrl_port), timeout=timeout) as sock:
        sock.settimeout(timeout)
        fh = sock.makefile("rwb", buffering=0)

        def readline():
            line = fh.readline()
            if not line:
                raise RuntimeError("control port closed the connection")
            return line.decode("utf-8", "replace").rstrip("\r\n")

        readline()
        fh.write(f"server {NICK} {service_pass}\n".encode())
        auth = readline()
        if not auth.startswith("+OK"):
            raise RuntimeError(f"control authentication failed: {auth}")
        fh.write((command + "\n").encode())
        reply = readline()
        fh.write(b"quit\n")
        return reply


def valid_exit_login(name):
    if not ALIAS_RE.match(name):
        return False
    if ".." in name or name == "server":
        return False
    return not name.endswith((".onion", ".sys", ".op", ".list"))


SMALL_CSS = """
body{font:15px/1.5 system-ui,sans-serif;background:#eef2f6;color:#1b2733;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.b{background:#fff;border:1px solid #d5dde6;border-radius:12px;padding:22px;max-width:520px}
h1{font-size:18px;margin:0 0 10px;color:#004488}a{color:#004488}
.err{background:#fdecef;border:1px solid #f2b9c4;color:#8c2334;border-radius:8px;padding:10px 12px;margin:10px 0;font-size:13px}
.om-prefs{display:flex;gap:7px;align-items:center;justify-content:flex-end;margin-bottom:14px}
.langbox{display:flex;align-items:center;gap:7px;margin:0}
.langbox label{font-size:10.5px;font-weight:700;color:#748394;text-transform:uppercase}
.langbox select{border:1px solid #c9d5e2;border-radius:8px;padding:6px;background:#fff;color:#31414f}
"""


def small_page(title, message):
    return (
        f'<!doctype html><html lang="{current_lang()}"><head><meta charset="utf-8">'
        '<meta name="viewport" content="width=device-width,initial-scale=1">'
        f'<title>{html.escape(title)}</title><style>{theme_css(SMALL_CSS)}</style>'
        '</head><body><div class="b">'
        f'{prefs_selector()}'
        f'<h1>{html.escape(title)}</h1><div class="err">{html.escape(message)}</div>'
        f'<p><a href="/register.html">{html.escape(tr("Back"))}</a></p></div></body></html>'
    )


VENDOR_I18N = {
    "de": {
        "Sign UP": "Registrieren", "Web": "Web", "Help": "Hilfe", "Rulez": "Regeln",
        "Info": "Info", "About": "Über", "Logon": "Anmelden", "Servers": "Server",
        "This is an OnionMail server.": "Dies ist ein OnionMail-Server.",
        "Server informations": "Serverinformationen", "Onion address:": "Onion-Adresse:",
        "Nick name:": "Nickname:", "Certificate SHA-1:": "Zertifikat SHA-1:",
        "Certificate (PEM):": "Zertifikat (PEM):", "Download": "Herunterladen",
        "PGP Public Key:": "Öffentlicher PGP-Schlüssel:", "Thunderbird autoconfig:": "Thunderbird-Autokonfiguration:",
        "Supported": "Unterstützt", "Mail address:": "Mailadresse:",
        "Captcha code": "Captcha-Code",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code.": "Sie sehen ein ASCII-Bild; erkennen Sie die Zeichen und geben Sie den Code ein.",
        "Be careful there may be some strange symbols to ignore.": "Ignorieren Sie eventuelle Sonderzeichen.",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code. Be careful there may be some strange symbols to ignore.": "Sie sehen ein ASCII-Bild; erkennen Sie die Zeichen und geben Sie den Code ein. Ignorieren Sie eventuelle Sonderzeichen.",
        "Login:": "Login:", "Password:": "Passwort:", "Captcha:": "Captcha:",
        "Exit Server:": "Exit-Server:", "(Default)": "(Standard)",
        "(*) Voucher code:": "(*) Voucher-Code:", "* = Not required.": "* = Nicht erforderlich.",
        "Please wait...": "Bitte warten …",
        "Your user will be created within 200 seconds.": "Ihr Benutzer wird innerhalb von 200 Sekunden erstellt.",
        "This process is slow! Don't worry. Do not leave this page!": "Der Vorgang dauert. Keine Sorge. Verlassen Sie diese Seite nicht!",
        "Now we are generating your internal keys and other random stuff.": "Jetzt werden Ihre internen Schlüssel und Zufallsdaten erzeugt.",
        "Statistics": "Statistik", "Net": "Netz", "Vouchers": "Voucher", "Logout": "Abmelden",
        "Onion Mail": "Onion Mail", "Encrypted mail Tor server hidden service.": "Verschlüsselter Mailserver als Tor-Hidden-Service.",
    },
    "fr": {
        "Sign UP": "S’inscrire", "Web": "Web", "Help": "Aide", "Rulez": "Règles",
        "Info": "Info", "About": "À propos", "Logon": "Connexion", "Servers": "Serveurs",
        "This is an OnionMail server.": "Ceci est un serveur OnionMail.",
        "Server informations": "Informations du serveur", "Onion address:": "Adresse Onion :",
        "Nick name:": "Pseudonyme :", "Certificate SHA-1:": "Certificat SHA-1 :",
        "Certificate (PEM):": "Certificat (PEM) :", "Download": "Télécharger",
        "PGP Public Key:": "Clé publique PGP :", "Thunderbird autoconfig:": "Configuration automatique Thunderbird :",
        "Supported": "Pris en charge", "Mail address:": "Adresse mail :",
        "Captcha code": "Code captcha",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code.": "Une image ASCII s’affiche ; reconnaissez les caractères et saisissez le code.",
        "Be careful there may be some strange symbols to ignore.": "Ignorez les symboles étranges.",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code. Be careful there may be some strange symbols to ignore.": "Une image ASCII s’affiche ; reconnaissez les caractères et saisissez le code. Ignorez les symboles étranges.",
        "Login:": "Identifiant :", "Password:": "Mot de passe :", "Captcha:": "Captcha :",
        "Exit Server:": "Serveur exit :", "(Default)": "(Par défaut)",
        "(*) Voucher code:": "(*) Code voucher :", "* = Not required.": "* = Facultatif.",
        "Please wait...": "Veuillez patienter…",
        "Your user will be created within 200 seconds.": "Votre compte sera créé sous 200 secondes.",
        "This process is slow! Don't worry. Do not leave this page!": "Cette opération est lente. Ne quittez pas cette page !",
        "Now we are generating your internal keys and other random stuff.": "Nous générons vos clés internes et des données aléatoires.",
        "Statistics": "Statistiques", "Net": "Réseau", "Vouchers": "Vouchers", "Logout": "Déconnexion",
        "Onion Mail": "Onion Mail", "Encrypted mail Tor server hidden service.": "Serveur mail chiffré en service caché Tor.",
    },
    "ru": {
        "Sign UP": "Регистрация", "Web": "Почта", "Help": "Помощь", "Rulez": "Правила",
        "Info": "Инфо", "About": "О проекте", "Logon": "Вход", "Servers": "Серверы",
        "This is an OnionMail server.": "Это сервер OnionMail.",
        "Server informations": "Информация о сервере", "Onion address:": "Onion-адрес:",
        "Nick name:": "Никнейм:", "Certificate SHA-1:": "Сертификат SHA-1:",
        "Certificate (PEM):": "Сертификат (PEM):", "Download": "Скачать",
        "PGP Public Key:": "Публичный ключ PGP:", "Thunderbird autoconfig:": "Автонастройка Thunderbird:",
        "Supported": "Поддерживается", "Mail address:": "Почтовый адрес:",
        "Captcha code": "Код captcha",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code.": "Будет показана ASCII-картинка; распознайте символы и введите код.",
        "Be careful there may be some strange symbols to ignore.": "Странные символы можно игнорировать.",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code. Be careful there may be some strange symbols to ignore.": "Будет показана ASCII-картинка; распознайте символы и введите код. Странные символы можно игнорировать.",
        "Login:": "Логин:", "Password:": "Пароль:", "Captcha:": "Captcha:",
        "Exit Server:": "Exit-сервер:", "(Default)": "(По умолчанию)",
        "(*) Voucher code:": "(*) Voucher-код:", "* = Not required.": "* = Необязательно.",
        "Please wait...": "Подождите…",
        "Your user will be created within 200 seconds.": "Ваш аккаунт будет создан в течение 200 секунд.",
        "This process is slow! Don't worry. Do not leave this page!": "Процесс небыстрый. Не закрывайте страницу!",
        "Now we are generating your internal keys and other random stuff.": "Сейчас генерируются внутренние ключи и случайные данные.",
        "Statistics": "Статистика", "Net": "Сеть", "Vouchers": "Vouchers", "Logout": "Выход",
        "Onion Mail": "Onion Mail", "Encrypted mail Tor server hidden service.": "Зашифрованный почтовый сервер как Tor hidden service.",
    },
    "es": {
        "Sign UP": "Registrarse", "Web": "Web", "Help": "Ayuda", "Rulez": "Reglas",
        "Info": "Info", "About": "Acerca de", "Logon": "Entrar", "Servers": "Servidores",
        "This is an OnionMail server.": "Este es un servidor OnionMail.",
        "Server informations": "Información del servidor", "Onion address:": "Dirección onion:",
        "Nick name:": "Alias:", "Certificate SHA-1:": "Certificado SHA-1:",
        "Certificate (PEM):": "Certificado (PEM):", "Download": "Descargar",
        "PGP Public Key:": "Clave pública PGP:", "Thunderbird autoconfig:": "Configuración automática de Thunderbird:",
        "Supported": "Compatible", "Mail address:": "Dirección de correo:",
        "Captcha code": "Código captcha",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code.": "Se mostrará una imagen ASCII; reconozca los caracteres e introduzca el código.",
        "Be careful there may be some strange symbols to ignore.": "Ignore los símbolos extraños.",
        "You will be shown a picture in ascii art, you will need to recognize the characters and enter the code. Be careful there may be some strange symbols to ignore.": "Se mostrará una imagen ASCII; reconozca los caracteres e introduzca el código. Ignore los símbolos extraños.",
        "Login:": "Usuario:", "Password:": "Contraseña:", "Captcha:": "Captcha:",
        "Exit Server:": "Servidor exit:", "(Default)": "(Predeterminado)",
        "(*) Voucher code:": "(*) Código voucher:", "* = Not required.": "* = No obligatorio.",
        "Please wait...": "Espere…",
        "Your user will be created within 200 seconds.": "Su cuenta se creará en 200 segundos.",
        "This process is slow! Don't worry. Do not leave this page!": "El proceso es lento. No cierre esta página.",
        "Now we are generating your internal keys and other random stuff.": "Ahora se generan sus claves internas y datos aleatorios.",
        "Statistics": "Estadísticas", "Net": "Red", "Vouchers": "Vouchers", "Logout": "Salir",
        "Onion Mail": "Onion Mail", "Encrypted mail Tor server hidden service.": "Servidor de correo cifrado como servicio oculto Tor.",
    },
}


def reg_exit_row():
    hint = tr("Only for the @{domain} address. To sign in at /web use the main login (without @).", domain=EXIT_DOMAIN)
    return (
        f'<tr><td>{html.escape(tr("Exit login"))}:</td><td><input type="text" name="om-exitlogin" '
        f'size="40" placeholder="for @{EXIT_DOMAIN}"></td></tr>'
        f'<tr><td></td><td><span style="font-size:12px;color:#748394">{html.escape(hint)}</span></td></tr>'
    ).encode("utf-8")


def translate_vendor_html(data):
    lang = current_lang()
    table = VENDOR_I18N.get(lang)
    if not table:
        return data
    try:
        text = data.decode("utf-8", "replace")
    except Exception:
        return data
    keys = sorted(table, key=len, reverse=True)
    pattern = re.compile(r"(?<=>)(\s*)(" + "|".join(re.escape(k) for k in keys) + r")(\s*)(?=<)")
    text = pattern.sub(lambda m: m.group(1) + html.escape(table[m.group(2)]) + m.group(3), text)
    dynamic = {
        "de": ("Erstellen Sie Ihr {name}-Mailkonto", "Anmeldung bei {name} Mailserver"),
        "fr": ("Créez votre compte mail {name}", "Connexion au serveur mail {name}"),
        "ru": ("Создайте почтовый аккаунт {name}", "Вход на почтовый сервер {name}"),
        "es": ("Cree su cuenta de correo {name}", "Acceso al servidor de correo {name}"),
    }
    if lang in dynamic:
        create_tpl, logon_tpl = dynamic[lang]
        text = re.sub(
            r"(?<=>)(\s*)Create Your (.+?) Mail Account(\s*)(?=<)",
            lambda m: m.group(1) + html.escape(create_tpl.format(name=m.group(2))) + m.group(3),
            text,
        )
        text = re.sub(
            r"(?<=>)(\s*)Logon to (.+?) Mail server(\s*)(?=<)",
            lambda m: m.group(1) + html.escape(logon_tpl.format(name=m.group(2))) + m.group(3),
            text,
        )
    return text.encode("utf-8")


CSS = """
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
@media(max-width:800px){.shell{flex-direction:column}.sb{width:auto;height:auto;position:static;flex-direction:row;flex-wrap:wrap;align-items:center}.nav{flex-direction:row;flex-wrap:wrap}.acc{margin:0;border:0;padding:0}.row{grid-template-columns:1fr 90px}.row .size{display:none}.main{padding:18px}}
"""


THEME = """
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
"""

DARK_MAP = {
    "#eef2f6": "#1a1025",
    "#ffffff": "#241631", "#fff": "#241631",
    "#f7fafd": "#2b1a3d", "#fbfdff": "#2b1a3d", "#f4f8fc": "#322046",
    "#eef4fb": "#2f1d42", "#e3edf9": "#3b2459", "#cfe2f7": "#5b3a80",
    "#e7edf3": "#3a2850", "#e4eaf0": "#3a2850", "#edf1f5": "#332145",
    "#dfe5eb": "#3a2850", "#d5dde6": "#4a3563", "#cfd8e2": "#4a3563",
    "#c9d5e2": "#4a3563", "#bcd3ea": "#6d4a99", "#7ba7cf": "#9d7ad1",
    "#16232f": "#f4ecff", "#1b2733": "#f4ecff", "#1f2d3a": "#f4ecff",
    "#0d2233": "#f4ecff", "#233241": "#e6daf5", "#2a3948": "#e6daf5",
    "#31414f": "#e6daf5", "#425466": "#d9cbea", "#5b6b7c": "#c3b3d9",
    "#5f7182": "#c3b3d9", "#748394": "#a99bc0", "#7b8a9b": "#a99bc0",
    "#8695a6": "#a99bc0", "#004488": "#b794ff", "#0058ad": "#d0b5ff",
    "#00366b": "#7c3aed", "#eaf6ee": "#1e3b2d", "#b7dfc3": "#2f6b4a",
    "#b9dfc5": "#2f6b4a", "#1f6b3a": "#8ee0ad", "#23633a": "#8ee0ad",
    "#fdecef": "#3d1c2b", "#f2b9c4": "#7a3b50", "#8c2334": "#ff9fb4",
    "#fff4e5": "#3d2c16", "#8a5a12": "#ffc46b", "#e08a3c": "#d99a5c",
}

DARK_EXTRA = """
input[type=submit],input[type=button],button,.btn{background:#7c3aed!important;color:#fff!important;border-color:#7c3aed!important}
input[type=submit]:hover,input[type=button]:hover,button:hover,.btn:hover{background:#9b5cff!important;border-color:#9b5cff!important}
nav a.on,.nav a.on{background:#7c3aed!important;border-color:#7c3aed!important;color:#fff!important}
.themebox img{filter:brightness(1.45) saturate(1.2)}
"""

TOGGLE_CSS = """
.om-prefs{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.om-prefs .langbox{margin:0}
.themebox{margin:0;display:flex}
.themebox button{display:flex;align-items:center;justify-content:center;width:34px;height:34px;padding:0!important;border:1px solid #c9d5e2!important;border-radius:8px!important;background:#fff!important;color:#004488!important;cursor:pointer}
.themebox button:hover{background:#f4f8fc!important}
.themebox img{width:18px;height:18px;display:block}
"""

UPSTREAM_PREFS_CSS = """
#om-prefs{position:fixed;top:10px;right:10px;z-index:2147483647;background:#fff;border:1px solid #d5dde6;border-radius:10px;padding:6px 9px;box-shadow:0 2px 10px rgba(16,42,67,.14);font:13px system-ui,sans-serif;display:flex;align-items:center;gap:8px;max-width:calc(100vw - 20px)}
#om-prefs form{display:flex;align-items:center;gap:7px;margin:0}
#om-prefs label{font:700 10px system-ui,sans-serif;color:#748394;text-transform:uppercase;letter-spacing:.08em;margin:0}
#om-prefs select{border:1px solid #c9d5e2;border-radius:7px;background:#fff;color:#31414f;font:600 12px system-ui,sans-serif;padding:5px 7px;cursor:pointer}
#om-prefs .themebox button{width:30px;height:30px;padding:0!important;border:1px solid #c9d5e2!important;border-radius:7px!important;background:#fff!important;color:#004488!important;cursor:pointer}
#om-prefs .themebox button:hover{background:#f4f8fc!important}
#om-prefs .themebox img{width:17px;height:17px;display:block}
@media(max-width:560px){#om-prefs label{display:none}}
"""


def _dark_colors(text):
    pattern = re.compile(r"#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b")
    return pattern.sub(lambda m: DARK_MAP.get(m.group(0).lower(), m.group(0)), text)


def theme_css(base):
    toggle = _dark_colors(TOGGLE_CSS) if current_theme() == "dark" else TOGGLE_CSS
    if current_theme() == "dark":
        return _dark_colors(base) + DARK_EXTRA + toggle
    return base + toggle


def cloud_swap_script():
    suffix = "black" if current_theme() == "dark" else "white"
    return (
        "<script>(function(){function s(){var r='"
        + suffix
        + "';document.querySelectorAll('img').forEach(function(i){var u=i.getAttribute('src')||'';"
        "if(/CIEU\\.png(?:\\?|$)/.test(u)){i.src='/theme-assets/CIEU_'+r+'.png';}"
        "else if(/CIED\\.png(?:\\?|$)/.test(u)){i.src='/theme-assets/CIED_'+r+'.png';}});}"
        "if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',s);}else{s();}})();</script>"
    )


def current_logo():
    design = om_state().get("design", {})
    logo = design.get("logo", DEFAULT_LOGO) if isinstance(design, dict) else DEFAULT_LOGO
    if not isinstance(logo, str) or not LOGO_RE.match(logo):
        return DEFAULT_LOGO
    return logo


def inject_logo(data):
    pattern = re.compile(rb"/img/(?:OM-BL\.gif|om-custom-logo\.(?:png|jpg|jpeg))")
    return pattern.sub(current_logo().encode("ascii"), data)


def inject_theme(data):
    data = inject_logo(data)
    lower = data.lower()
    if b"</head>" not in lower:
        return data
    box = (
        b'<style id="om-prefs-style">' + theme_css(UPSTREAM_PREFS_CSS).encode("utf-8") + b"</style>"
        b'<div id="om-prefs">' + prefs_selector().encode("utf-8") + b"</div>"
        + cloud_swap_script().encode("utf-8")
    )
    pos = lower.find(b"<body")
    if pos != -1:
        j = data.find(b">", pos)
        if j != -1:
            data = data[:j + 1] + box + data[j + 1:]
            lower = data.lower()
    else:
        data = box + data
        lower = data.lower()
    i = lower.rfind(b"</head>")
    return data[:i] + b'<style id="om-theme">' + theme_css(THEME).encode("utf-8") + b"</style>" + data[i:]


def tls_wrap(sock):
    return CTX.wrap_socket(sock, server_hostname="localhost")


def pop3_conn(user, password):
    import poplib
    p = poplib.POP3("127.0.0.1", POP3_PORT, timeout=30)
    p.stls(context=CTX)
    p.user(user)
    p.pass_(password)
    return p


def smtp_conn(user, password):
    s = smtplib.SMTP("127.0.0.1", SMTP_PORT, timeout=30)
    s.ehlo()
    s.starttls(context=CTX)
    s.ehlo()
    s.login(user, password)
    return s


def decode_hdr(value):
    if not value:
        return ""
    try:
        return str(make_header(decode_header(str(value))))
    except Exception:
        return str(value)


def fmt_addr(value):
    if not value:
        return ""
    try:
        pairs = email.utils.getaddresses([str(value)])
        out = []
        for name, addr in pairs:
            name = decode_hdr(name)
            out.append(f"{name} <{addr}>" if name else addr)
        return ", ".join(out)
    except Exception:
        return str(value)


def human_size(n):
    try:
        n = int(n)
    except Exception:
        return ""
    if n < 1024:
        return f"{n} B"
    if n < 1024 * 1024:
        return f"{n / 1024:.1f} KB"
    return f"{n / (1024 * 1024):.1f} MB"


class _TextExtractor(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.parts = []
        self.skip = 0

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style"):
            self.skip += 1
        if tag in ("br", "p", "div", "tr", "li", "h1", "h2", "h3", "blockquote"):
            self.parts.append("\n")

    def handle_endtag(self, tag):
        if tag in ("script", "style") and self.skip:
            self.skip -= 1
        if tag in ("p", "div", "tr", "li", "h1", "h2", "h3", "blockquote"):
            self.parts.append("\n")

    def handle_data(self, data):
        if not self.skip:
            self.parts.append(data)


def html_to_text(data):
    p = _TextExtractor()
    try:
        p.feed(data)
        p.close()
        txt = "".join(p.parts)
        txt = re.sub(r"\n{3,}", "\n\n", txt)
        return txt.strip()
    except Exception:
        return data


def extract_body(msg):
    text_parts = []
    html_parts = []
    attachments = []
    if msg.is_multipart():
        for part in msg.walk():
            if part.is_multipart():
                continue
            ctype = part.get_content_type()
            disp = (part.get("Content-Disposition") or "").lower()
            filename = part.get_filename()
            if filename or "attachment" in disp:
                if filename:
                    attachments.append(decode_hdr(filename))
                continue
            try:
                payload = part.get_content()
            except Exception:
                continue
            if not isinstance(payload, str):
                continue
            if ctype == "text/plain":
                text_parts.append(payload)
            elif ctype == "text/html":
                html_parts.append(payload)
    else:
        try:
            payload = msg.get_content()
        except Exception:
            payload = ""
        if isinstance(payload, str):
            if msg.get_content_type() == "text/html":
                html_parts.append(payload)
            else:
                text_parts.append(payload)
    body = "\n\n".join(t for t in text_parts if t.strip())
    if not body.strip() and html_parts:
        body = "\n\n".join(html_to_text(h) for h in html_parts)
    if not body.strip():
        body = tr("(empty message)")
    if len(body) > MAX_BODY:
        body = body[:MAX_BODY] + "\n\n" + tr("[... truncated ...]")
    return body, attachments


def parse_msg(raw):
    return BytesParser(policy=email.policy.default).parsebytes(raw)


def inbox_items(user, password, limit=300):
    p = pop3_conn(user, password)
    try:
        _, listing, _ = p.list()
        nums = []
        for line in listing:
            try:
                parts = line.split()
                nums.append((int(parts[0]), int(parts[1])))
            except Exception:
                continue
        nums = nums[-limit:]
        items = []
        for n, size in reversed(nums):
            try:
                _, lines, _ = p.top(n, 0)
                msg = parse_msg(b"\r\n".join(lines))
                items.append({
                    "n": n,
                    "size": size,
                    "from": fmt_addr(msg.get("From")),
                    "subject": decode_hdr(msg.get("Subject")) or tr("(no subject)"),
                    "date": decode_hdr(msg.get("Date")),
                })
            except Exception:
                continue
        return items
    finally:
        try:
            p.quit()
        except Exception:
            pass


def fetch_message(user, password, n):
    p = pop3_conn(user, password)
    try:
        _, lines, _ = p.retr(int(n))
        msg = parse_msg(b"\r\n".join(lines))
        body, atts = extract_body(msg)
        return {
            "from": fmt_addr(msg.get("From")),
            "to": fmt_addr(msg.get("To")),
            "subject": decode_hdr(msg.get("Subject")) or tr("(no subject)"),
            "date": decode_hdr(msg.get("Date")),
            "body": body,
            "attachments": atts,
        }
    finally:
        try:
            p.quit()
        except Exception:
            pass


def new_session(user, smtp_pass, pop3_pass):
    token = secrets.token_urlsafe(32)
    with _sessions_lock:
        _sessions[token] = {
            "user": user,
            "smtp": smtp_pass,
            "pop3": pop3_pass,
            "csrf": secrets.token_urlsafe(24),
            "exp": time.time() + SESSION_TTL,
        }
        cutoff = time.time()
        for k in [k for k, v in _sessions.items() if v["exp"] < cutoff]:
            _sessions.pop(k, None)
    return token


def get_session(handler):
    raw = handler.headers.get("Cookie")
    if not raw:
        return None, None
    jar = SimpleCookie()
    try:
        jar.load(raw)
    except Exception:
        return None, None
    morsel = jar.get(COOKIE)
    if not morsel:
        return None, None
    token = morsel.value
    with _sessions_lock:
        sess = _sessions.get(token)
        if sess and sess["exp"] > time.time():
            sess["exp"] = time.time() + SESSION_TTL
            return token, sess
        _sessions.pop(token, None)
    return None, None


def session_cookie(token):
    return f"{COOKIE}={token}; Path=/web; HttpOnly; SameSite=Strict; Max-Age={SESSION_TTL}"


def clear_cookie():
    return f"{COOKIE}=deleted; Path=/web; HttpOnly; SameSite=Strict; Max-Age=0"


def sent_path(user):
    if not re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", user or ""):
        raise ValueError("bad user")
    return os.path.join(SENT_DIR, user + ".json")


def load_sent(user):
    path = sent_path(user)
    try:
        with open(path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        return data if isinstance(data, list) else []
    except (OSError, ValueError):
        return []


def add_sent(user, entry):
    path = sent_path(user)
    os.makedirs(SENT_DIR, mode=0o700, exist_ok=True)
    with _sent_lock:
        data = load_sent(user)
        data.insert(0, entry)
        data = data[:500]
        tmp = path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(data, fh, ensure_ascii=False)
        os.chmod(tmp, 0o600)
        os.replace(tmp, path)


ADMIN_FORM_TOKEN = secrets.token_urlsafe(24)
_registry_lock = threading.Lock()


def load_registry():
    try:
        with open(REGISTRY_FILE, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        if isinstance(data, dict) and isinstance(data.get("users"), dict):
            return data
    except (OSError, ValueError):
        pass
    return {"users": {}}


def save_registry(data):
    os.makedirs(os.path.dirname(REGISTRY_FILE), mode=0o700, exist_ok=True)
    tmp = REGISTRY_FILE + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(data, fh, ensure_ascii=False, indent=2)
    os.chmod(tmp, 0o600)
    os.replace(tmp, REGISTRY_FILE)


def register_user(login, smtp="", pop3="", exitlogin="", last_login=None):
    login = (login or "").strip().lower()
    if not re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", login):
        return False
    with _registry_lock:
        data = load_registry()
        users = data["users"]
        entry = users.get(login) or {}
        if smtp:
            entry["smtp"] = smtp
        if pop3:
            entry["pop3"] = pop3
        if exitlogin:
            entry["exit"] = exitlogin.strip().lower()
        if last_login:
            entry["last_login"] = int(last_login)
        entry.setdefault("created", int(time.time()))
        users[login] = entry
        save_registry(data)
    return True


def pop3_stats(login, password):
    import poplib
    conn = poplib.POP3("127.0.0.1", POP3_PORT, timeout=8)
    try:
        conn.stls(context=CTX)
        conn.user(login)
        conn.pass_(password)
        count, size = conn.stat()
        return int(count), int(size)
    finally:
        try:
            conn.quit()
        except Exception:
            try:
                conn.close()
            except Exception:
                pass


def pop3_exists(login):
    import poplib
    conn = poplib.POP3("127.0.0.1", POP3_PORT, timeout=5)
    try:
        conn.stls(context=CTX)
        conn.user(login)
        return True
    except Exception:
        return False
    finally:
        try:
            conn.quit()
        except Exception:
            try:
                conn.close()
            except Exception:
                pass


def control_stat(timeout=8):
    service_pass = None
    ctrl_port = 9010
    try:
        with open(SERVER_DIR, "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and parts[0].lower() == "passwd":
                    service_pass = parts[1]
                    break
        with open(CONTROL_CONF, "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                parts = line.split()
                if len(parts) >= 2 and parts[0].lower() == "controlport":
                    ctrl_port = int(parts[1])
                    break
    except OSError:
        return {}
    if not service_pass:
        return {}
    out = {}
    with socket.create_connection(("127.0.0.1", ctrl_port), timeout=timeout) as sock:
        sock.settimeout(timeout)
        fh = sock.makefile("rwb", buffering=0)

        def readline():
            line = fh.readline()
            if not line:
                raise RuntimeError("control port closed")
            return line.decode("utf-8", "replace").rstrip("\r\n")

        readline()
        fh.write(f"server {NICK} {service_pass}\n".encode())
        if not readline().startswith("+OK"):
            return {}
        fh.write(b"stat\n")
        if readline().startswith("+OK"):
            while True:
                line = readline()
                if line in (".", "") or line.startswith("-ERR"):
                    break
                if ":" in line:
                    key, value = line.split(":", 1)
                    out[key.strip()] = value.strip()
        fh.write(b"quit\n")
    return out


def control_ver():
    try:
        reply = control_command("ver")
        if reply.startswith("+OK "):
            parts = reply[4:].split()
            if parts:
                return parts[0]
    except Exception:
        pass
    return ""


def process_uptime():
    try:
        return int(time.time() - os.stat("/proc/1").st_ctime)
    except OSError:
        return 0


def dir_size(path):
    total = 0
    for root, _dirs, files in os.walk(path, followlinks=False):
        for name in files:
            full = os.path.join(root, name)
            try:
                if not os.path.islink(full):
                    total += os.lstat(full).st_size
            except OSError:
                pass
    return total


def fmt_size(num):
    num = max(0, int(num or 0))
    kb = num / 1024.0
    if kb <= 4000:
        return f"{kb:.1f} KB".replace(".", ",")
    mb = kb / 1024.0
    if mb <= 8000:
        return f"{mb:.1f} MB".replace(".", ",")
    gb = mb / 1024.0
    if gb <= 999:
        return f"{gb:.1f} GB".replace(".", ",")
    tb = gb / 1024.0
    if tb <= 999:
        return f"{tb:.2f} TB".replace(".", ",")
    return f"{tb / 1024.0:.2f} PB".replace(".", ",")


def fmt_uptime(seconds):
    try:
        seconds = int(seconds)
    except (TypeError, ValueError):
        return "—"
    if seconds <= 0:
        return "—"
    days, rem = divmod(seconds, 86400)
    hours, rem = divmod(rem, 3600)
    mins = rem // 60
    if days:
        return f"{days}d {hours}h"
    if hours:
        return f"{hours}h {mins}m"
    return f"{mins}m"


def parse_success_creds(text):
    plain = html.unescape(re.sub(r"<[^>]+>", " ", text))
    sm = re.search(r"SMTP\s+Password\s*:?\s*([A-Za-z0-9+/=_-]{8,})", plain, re.I)
    po = re.search(r"POP3\s+Password\s*:?\s*([A-Za-z0-9+/=_-]{8,})", plain, re.I)
    if sm and po:
        return sm.group(1), po.group(1)
    return None


def registry_seed():
    with _registry_lock:
        data = load_registry()
        users = data["users"]
        changed = False
        sysop_file = os.path.join(MAILDIR, "sysop.txt")
        try:
            with open(sysop_file, "r", encoding="utf-8", errors="replace") as fh:
                text = fh.read()
            sm = re.search(r"SMTP\s+Password\s*:?\s*(\S+)", text, re.I)
            po = re.search(r"POP3\s+Password\s*:?\s*(\S+)", text, re.I)
            if sm and po:
                entry = users.get("sysop") or {}
                if not entry.get("smtp") or not entry.get("pop3"):
                    entry.update({"smtp": sm.group(1), "pop3": po.group(1)})
                    entry.setdefault("created", int(time.time()))
                    users["sysop"] = entry
                    changed = True
        except OSError:
            pass
        try:
            for name in os.listdir(SENT_DIR):
                if not name.endswith(".json"):
                    continue
                login = name[:-5].lower()
                if re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", login) and login not in users:
                    users[login] = {"created": int(time.time())}
                    changed = True
        except OSError:
            pass
        if "root" not in users and pop3_exists("root"):
            users["root"] = {"created": int(time.time())}
            changed = True
        if changed:
            save_registry(data)


def page_shell(sess, active, body, title=None):
    title = title or DISPLAY_NAME
    user = html.escape(sess["user"]) if sess else ""
    addr = f"{html.escape(sess['user'])}@{html.escape(ONION)}" if sess else ""
    csrf = html.escape(sess["csrf"]) if sess else ""
    return f"""<!doctype html>
<html lang="{current_lang()}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{html.escape(title)}</title>
<style>{theme_css(CSS)}</style></head><body>
<div class="shell">
<aside class="sb">
  {prefs_selector()}
  <div class="brand"><b>{html.escape(DISPLAY_NAME)}</b><span>{html.escape(tr("OnionMail webmail"))}</span></div>
  <nav class="nav">
    <a class="{'on' if active=='inbox' else ''}" href="/web/inbox">{html.escape(tr("Inbox"))}</a>
    <a class="{'on' if active=='sent' else ''}" href="/web/sent">{html.escape(tr("Sent"))}</a>
    <a class="{'on' if active=='compose' else ''}" href="/web/compose">{html.escape(tr("New message"))}</a>
  </nav>
  <div class="acc">
    <div class="who">{user}<br><span>{addr}</span></div>
    <form method="post" action="/web/logout">
      <input type="hidden" name="csrf" value="{csrf}">
      <button type="submit">{html.escape(tr("Log out"))}</button>
    </form>
  </div>
</aside>
<main class="main">{body}</main>
</div></body></html>"""


def login_page(error=""):
    err = f'<div class="err">{html.escape(error)}</div>' if error else ""
    return f"""<!doctype html>
<html lang="{current_lang()}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{html.escape(DISPLAY_NAME)} — {html.escape(tr("Mail sign-in"))}</title>
<style>{theme_css(CSS)}</style></head><body>
<div class="login"><div class="card">
  {prefs_selector()}
  <div class="brand"><b>{html.escape(DISPLAY_NAME)}</b><span>{html.escape(tr("OnionMail webmail"))}</span></div>
  <p>{html.escape(tr("Sign in to your mailbox on this server."))}</p>
  {err}
  <form method="post" action="/web/login">
    <label for="u">{html.escape(tr("Username (main login, no @)"))}</label>
    <input id="u" name="username" type="text" autocomplete="username" spellcheck="false" placeholder="{html.escape(tr("e.g. ivan"))}" required>
    <label for="s">{html.escape(tr("SMTP password"))}</label>
    <input id="s" name="smtp" type="password" autocomplete="current-password" required>
    <label for="p">{html.escape(tr("POP3 password"))}</label>
    <input id="p" name="pop3" type="password" autocomplete="current-password" required>
    <div style="margin-top:18px"><button class="btn" type="submit">{html.escape(tr("Sign in"))}</button></div>
  </form>
  <p style="margin-top:10px">{html.escape(tr("Your login is the mailbox name before @, not the exit login. SMTP and POP3 passwords are different (both are shown after registration)."))}</p>
  <p style="margin-top:14px"><a href="/register.html">{html.escape(tr("No account? Register with a voucher code"))}</a></p>
</div></div></body></html>"""


def login_form(error="", status=200):
    return status, login_page(error)


def inbox_page(sess):
    try:
        items = inbox_items(sess["user"], sess["pop3"])
    except Exception as exc:
        body = f'<div class="err">{html.escape(tr("Could not open mailbox: {err}", err=str(exc)))}</div>'
        return page_shell(sess, "inbox", f'<div class="top"><h1>{html.escape(tr("Inbox"))}</h1></div>{body}')
    if not items:
        rows = f'<div class="row"><div class="from">{html.escape(tr("No messages"))}</div><div class="subj"></div><div></div><div></div></div>'
    else:
        rows = ""
        for it in items:
            rows += f"""<a class="row" href="/web/message?n={it['n']}">
<div class="from">{html.escape(it['from'])}</div>
<div class="subj">{html.escape(it['subject'])}</div>
<div class="date">{html.escape(it['date'])}</div>
<div class="size">{human_size(it['size'])}</div></a>"""
    return page_shell(sess, "inbox", f"""<div class="top"><h1>{html.escape(tr("Inbox"))}</h1>
<span class="muted">{html.escape(tr("Messages: {n}", n=len(items)))}</span></div><div class="list">{rows}</div>""")


def message_page(sess, n):
    try:
        m = fetch_message(sess["user"], sess["pop3"], n)
    except Exception as exc:
        return page_shell(sess, "inbox", f'<div class="err">{html.escape(tr("Could not read message: {err}", err=str(exc)))}</div>')
    atts = ""
    if m["attachments"]:
        atts = f'<div class="att">{html.escape(tr("Attachments: {names}", names=", ".join(m["attachments"])))}</div>'
    body = f"""<a class="back" href="/web/inbox">&larr; {html.escape(tr("Back"))}</a>
<div class="card"><h2>{html.escape(m['subject'])}</h2>
<div class="meta">{html.escape(tr("From"))}: {html.escape(m['from'])}<br>{html.escape(tr("To"))}: {html.escape(m['to'])}<br>{html.escape(tr("Date"))}: {html.escape(m['date'])}</div>
<pre class="body">{html.escape(m['body'])}</pre>{atts}</div>"""
    return page_shell(sess, "inbox", body)


def sent_page(sess):
    items = load_sent(sess["user"])
    if not items:
        rows = f'<div class="row"><div class="from">{html.escape(tr("No sent messages"))}</div><div class="subj"></div><div></div><div></div></div>'
    else:
        rows = ""
        for it in items:
            rows += f"""<a class="row" href="/web/sent/view?id={quote(it.get('id',''))}">
<div class="from">{html.escape(tr("To: {to}", to=it.get('to','')))}</div>
<div class="subj">{html.escape(it.get('subject','')) or html.escape(tr("(no subject)"))}</div>
<div class="date">{html.escape(it.get('date',''))}</div>
<div class="size"></div></a>"""
    return page_shell(sess, "sent", f"""<div class="top"><h1>{html.escape(tr("Sent"))}</h1>
<span class="muted">{html.escape(tr("Messages: {n}", n=len(items)))}</span></div><div class="list">{rows}</div>""")


def sent_view(sess, sid):
    for it in load_sent(sess["user"]):
        if it.get("id") == sid:
            body = f"""<a class="back" href="/web/sent">&larr; {html.escape(tr("Back"))}</a>
<div class="card"><h2>{html.escape(it.get('subject','') or tr('(no subject)'))}</h2>
<div class="meta">{html.escape(tr("To"))}: {html.escape(it.get('to',''))}<br>{html.escape(tr("Date"))}: {html.escape(it.get('date',''))}</div>
<pre class="body">{html.escape(it.get('body',''))}</pre></div>"""
            return page_shell(sess, "sent", body)
    return page_shell(sess, "sent", f'<div class="err">{html.escape(tr("Message not found."))}</div>')


def compose_page(sess, ok="", error=""):
    note = ""
    if ok:
        note = f'<div class="ok">{html.escape(tr("Message sent."))}</div>'
    if error:
        note += f'<div class="err">{html.escape(error)}</div>'
    body = f"""<div class="top"><h1>{html.escape(tr("New message"))}</h1></div>{note}
<div class="card"><form method="post" action="/web/send">
<input type="hidden" name="csrf" value="{html.escape(sess['csrf'])}">
<label for="to">{html.escape(tr("To"))}</label><input id="to" type="text" name="to" placeholder="user@some.onion" required>
<label for="su">{html.escape(tr("Subject"))}</label><input id="su" type="text" name="subject" maxlength="200">
<label for="bo">{html.escape(tr("Message"))}</label><textarea id="bo" name="body" maxlength="100000"></textarea>
<div style="margin-top:16px"><button class="btn" type="submit">{html.escape(tr("Send"))}</button></div>
</form></div>"""
    return page_shell(sess, "compose", body)


ADMIN_CSS = """
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
"""

C_RECV = "#004488"
C_SENT = "#2f7fd1"
C_USER = "#7fb2e5"
C_SERVICE = "#c9d5e2"
C_IN = "#004488"
C_OUT = "#e08a3c"


def chart_palette():
    if current_theme() == "dark":
        return {
            "recv": "#b794ff", "sent": "#8b5cf6", "user": "#6d4ac9",
            "service": "#4a3563", "in": "#b794ff", "out": "#d99a5c",
            "top": ["#b794ff", "#9d6cff", "#8353e6", "#6d4ac9", "#4a3563"],
            "other": "#4a3563",
        }
    return {
        "recv": C_RECV, "sent": C_SENT, "user": C_USER, "service": C_SERVICE,
        "in": C_IN, "out": C_OUT,
        "top": ["#004488", "#2f7fd1", "#5b9bd5", "#86b7e4", "#b3d1ec"],
        "other": "#c9d5e2",
    }


def fmt_int(num):
    try:
        return f"{int(num):,}".replace(",", " ")
    except (TypeError, ValueError):
        return "0"


def donut_svg(segments, center_top, center_bottom, value_fmt=fmt_size):
    total = sum(max(0, v) for _n, v, _c in segments)
    dark = current_theme() == "dark"
    ring = "#332145" if dark else "#e7edf3"
    text_main = "#f4ecff" if dark else "#16232f"
    text_sub = "#a99bc0" if dark else "#748394"
    parts = []
    if total <= 0:
        parts.append(f'<circle cx="95" cy="95" r="64" fill="none" stroke="{ring}" stroke-width="26"/>')
    else:
        offset = 0.0
        for name, value, color in segments:
            value = max(0, value)
            if value <= 0:
                continue
            pct = value * 100.0 / total
            parts.append(
                f'<circle cx="95" cy="95" r="64" fill="none" stroke="{color}" stroke-width="26" '
                f'pathLength="100" stroke-dasharray="{pct:.3f} {100 - pct:.3f}" '
                f'stroke-dashoffset="{-offset:.3f}" transform="rotate(-90 95 95)">'
                f'<title>{html.escape(name)}: {value_fmt(value)} ({pct:.1f}%)</title></circle>'
            )
            offset += pct
    svg = (
        '<svg class="donut" viewBox="0 0 190 190" width="190" height="190" role="img" '
        'aria-label="' + html.escape(center_top) + '">'
        + "".join(parts)
        + f'<text x="95" y="91" text-anchor="middle" font-size="15" font-weight="700" fill="{text_main}">{html.escape(center_top)}</text>'
        + f'<text x="95" y="110" text-anchor="middle" font-size="10.5" fill="{text_sub}">{html.escape(center_bottom)}</text>'
        + "</svg>"
    )
    legend = '<div class="legend">'
    for name, value, color in segments:
        pct = (max(0, value) * 100.0 / total) if total else 0
        legend += (
            f'<div class="lg"><span class="dot" style="background:{color}"></span>'
            f'<span class="nm">{html.escape(name)}</span><b>{value_fmt(value)}</b>'
            f'<span class="pc">{pct:.0f}%</span></div>'
        )
    legend += "</div>"
    return f'<div class="chart">{svg}{legend}</div>'


def admin_card(label, value, sub=""):
    subhtml = f'<div class="s">{html.escape(sub)}</div>' if sub else ""
    return f'<div class="card"><div class="k">{html.escape(label)}</div><div class="v">{html.escape(value)}</div>{subhtml}</div>'


def render_dashboard(data, message="", error=""):
    alerts = ""
    if message:
        alerts += f'<div class="alert ok">{html.escape(message)}</div>'
    if error:
        alerts += f'<div class="alert err">{html.escape(error)}</div>'

    stat = data.get("stat") or {}
    users = data.get("users") or []
    cats = data.get("cat") or {}
    cat_total = cats.get("total", 0)
    disk_total, disk_free = data.get("disk", (0, 0))

    palette = chart_palette()
    received = int(cats.get("received", 0))
    sent = int(cats.get("sent", 0))
    userdata = int(cats.get("user", 0))
    service = int(cats.get("service", 0))

    storage_segments = [
        (tr("Received mail"), received, palette["recv"]),
        (tr("Sent mail"), sent, palette["sent"]),
        (tr("User data"), userdata, palette["user"]),
        (tr("Service files"), service, palette["service"]),
    ]

    top = sorted(users, key=lambda u: (u.get("received_bytes", 0) + u.get("sent_bytes", 0)), reverse=True)
    top = [u for u in top if (u.get("received_bytes", 0) + u.get("sent_bytes", 0)) > 0][:5]
    top_segments = [
        (u["login"], u.get("received_bytes", 0) + u.get("sent_bytes", 0), palette["top"][i % 5])
        for i, u in enumerate(top)
    ]
    top_rest = sum(u.get("received_bytes", 0) + u.get("sent_bytes", 0) for u in users) - sum(v for _n, v, _c in top_segments)
    if top_rest > 0:
        top_segments.append((tr("Other"), top_rest, palette["other"]))

    in_count = int(stat.get("Messages-In") or 0)
    out_count = int(stat.get("Messages-Out") or 0)
    mailbox_count = sum(int(u.get("received_count", 0)) for u in users)
    sent_copies = sum(int(u.get("sent_count", 0)) for u in users)
    mail_segments = [
        (tr("Incoming"), mailbox_count, palette["in"]),
        (tr("Outgoing"), sent_copies, palette["out"]),
    ]

    cards = "".join([
        admin_card(tr("Users"), fmt_int(data.get("user_total", 0)),
                   tr("with POP3 password: {n}", n=fmt_int(data.get('user_known', 0)))),
        admin_card(tr("Messages in mailboxes"), fmt_int(mailbox_count),
                   tr("server received: {n} · {size}", n=fmt_int(in_count), size=fmt_size(received))),
        admin_card(tr("Sent"), fmt_int(sent_copies),
                   tr("server sent: {n} · {size}", n=fmt_int(out_count), size=fmt_size(sent))),
        admin_card(tr("User data"), fmt_size(userdata), tr("usr, keys, registry")),
        admin_card(tr("Service files"), fmt_size(service), tr("indexes, Tor, logs, www")),
        admin_card(tr("Free disk"), fmt_size(disk_free), tr("of {size}", size=fmt_size(disk_total))),
        admin_card(tr("Server uptime"), fmt_uptime(data.get("uptime")),
                   tr("threads: {n}", n=fmt_int(data.get('threads', {}).get('thr', 0)))),
        admin_card(tr("Exit / Friends"), f"{fmt_int(data.get('exits', 0))} / {fmt_int(data.get('friends', 0))}",
                   tr("spam: {n}", n=fmt_int(stat.get('Spam-Blocked', 0)))),
    ])

    rows = ""
    for u in users:
        if u.get("status") == "ok":
            badge = f'<span class="badge ok">{html.escape(tr("ok"))}</span>'
        elif u.get("status") == "nopass":
            badge = f'<span class="badge warn">{html.escape(tr("no password"))}</span>'
        else:
            badge = f'<span class="badge err">{html.escape(tr("error"))}</span>'
        last = u.get("last_login") or 0
        last_s = time.strftime("%d.%m.%Y %H:%M", time.localtime(last)) if last else "—"
        rows += (
            f'<tr><td><span class="login">{html.escape(u["login"])}</span></td>'
            f'<td class="mut">{html.escape(u.get("exit") or "—")}</td>'
            f'<td class="num">{fmt_int(u.get("received_count", 0))}</td>'
            f'<td class="num">{fmt_size(u.get("received_bytes", 0))}</td>'
            f'<td class="num">{fmt_int(u.get("sent_count", 0))}</td>'
            f'<td class="num">{fmt_size(u.get("sent_bytes", 0))}</td>'
            f'<td class="mut">{last_s}</td><td>{badge}</td></tr>'
        )
    if not rows:
        rows = f'<tr><td colspan="8" class="empty">{html.escape(tr("No mailboxes in the registry yet."))}</td></tr>'

    web_hits = data.get("web", {}).get("httph", 0)
    web_err = data.get("web", {}).get("httpe", 0)
    version = data.get("version") or "—"
    footer = tr(
        "Version: {version} · Web: {hits} requests, {errors} errors · New today: {new} · Errors: {errs}",
        version=str(version), hits=fmt_int(web_hits), errors=fmt_int(web_err),
        new=fmt_int(stat.get('NewUsrThisDay', 0)), errs=fmt_int(stat.get('Errors', 0)),
    )
    privacy = tr(
        "POP3/SMTP passwords are stored only locally in {path} (0600) and are needed to measure mailbox sizes.",
        path=REGISTRY_FILE,
    )

    return f"""<!doctype html>
<html lang="{current_lang()}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>{html.escape(DISPLAY_NAME)} · sysop</title>
<style>{theme_css(ADMIN_CSS)}</style></head><body><div class="wrap">
<div class="topbar">{prefs_selector()}<div class="brand">{html.escape(DISPLAY_NAME)}<span>OnionMail · sysop panel</span></div>
<nav><a class="on" href="/admin/index.html">{html.escape(tr("Statistics"))}</a>
<a href="/admin/elist.html">{html.escape(tr("Net"))}</a>
<a href="/admin/voucher.html">{html.escape(tr("Vouchers"))}</a>
<a href="/admin/logout.html">{html.escape(tr("Log out"))}</a></nav></div>
{alerts}
<div class="cards">{cards}</div>
<div class="charts">
<div class="panel"><h2>{html.escape(tr("Storage"))}</h2><div class="hint">{html.escape(tr("Total: {total} · free {free}", total=fmt_size(cat_total), free=fmt_size(disk_free)))}</div>
{donut_svg(storage_segments, fmt_size(cat_total), tr("occupied by data"))}</div>
<div class="panel"><h2>{html.escape(tr("Top mailboxes by size"))}</h2><div class="hint">{html.escape(tr("Received + sent"))}</div>
{donut_svg(top_segments, fmt_size(sum(v for _n, v, _c in top_segments)), tr("of users")) if top_segments else f'<div class="empty">{html.escape(tr("No mailbox size data."))}</div>'}</div>
<div class="panel"><h2>{html.escape(tr("Mail"))}</h2><div class="hint">{html.escape(tr("OnionMail counters"))}</div>
{donut_svg(mail_segments, fmt_int(mailbox_count + sent_copies), tr("messages total"), value_fmt=fmt_int)}</div>
</div>
<div class="panel"><h2>{html.escape(tr("Mailboxes"))}</h2>
<div class="hint">{html.escape(tr("OnionMail does not expose a mailbox list via API. The panel keeps a local registry: new registrations and /web logins are added automatically, others can be added manually below."))}</div>
<div class="tblscroll"><table><thead><tr><th>{html.escape(tr("Mailbox"))}</th><th>{html.escape(tr("Exit login"))}</th><th>{html.escape(tr("Inbox"))}</th><th>{html.escape(tr("Size"))}</th><th>{html.escape(tr("Sent"))}</th><th>{html.escape(tr("Size"))}</th><th>{html.escape(tr("Last login"))}</th><th>{html.escape(tr("Status"))}</th></tr></thead>
<tbody>{rows}</tbody></table></div></div>
<div class="panel"><h2>{html.escape(tr("Add / update mailbox"))}</h2>
<form class="add" method="post" action="/admin/storage.html">
<input type="hidden" name="csrf" value="{html.escape(ADMIN_FORM_TOKEN)}">
<div><label for="login">{html.escape(tr("Login (no @)"))}</label><input id="login" name="login" required maxlength="16" placeholder="user"></div>
<div><label for="pop3">{html.escape(tr("POP3 password"))}</label><input id="pop3" name="pop3" type="password" required autocomplete="new-password"></div>
<div><label for="smtp">{html.escape(tr("SMTP password (optional)"))}</label><input id="smtp" name="smtp" type="password" autocomplete="new-password"></div>
<div><label for="exit">{html.escape(tr("Exit login (optional)"))}</label><input id="exit" name="exit" maxlength="30" placeholder="alias"></div>
<div><button type="submit">{html.escape(tr("Check and save"))}</button></div>
</form></div>
<div class="foot">{html.escape(footer)}<br>
{html.escape(privacy)}</div>
</div></body></html>"""


class Handler(BaseHTTPRequestHandler):
    server_version = "OnionMailFront/1.0"
    sys_version = ""
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        sys.stdout.write("%s %s\n" % (time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), fmt % args))
        sys.stdout.flush()

    def send_bytes(self, code, data, ctype, extra=None):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("X-Content-Type-Options", "nosniff")
        for k, v in (extra or []):
            self.send_header(k, v)
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(data)

    def send_html(self, code, text, extra=None):
        self.send_bytes(code, text.encode("utf-8"), "text/html; charset=utf-8", extra)

    def send_theme_asset(self, path):
        name = path[len("/theme-assets/"):]
        ctype = THEME_ASSETS.get(name)
        if not ctype or "/" in name or "\\" in name or name.startswith("."):
            self.send_html(404, "<h1>404</h1>")
            return
        try:
            with open(os.path.join(THEME_DIR, name), "rb") as fh:
                data = fh.read()
        except OSError:
            self.send_html(404, "<h1>404</h1>")
            return
        self.send_bytes(200, data, ctype)

    def redirect(self, location, extra=None):
        headers = [("Location", location)] + (extra or [])
        self.send_bytes(302, b"", "text/plain", headers)

    def read_body(self, limit=300_000):
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > limit:
            return b""
        return self.rfile.read(length)

    def form(self):
        raw = self.read_body()
        try:
            return parse_qs(raw.decode("utf-8", "replace"), keep_blank_values=True)
        except Exception:
            return {}

    def admin_ok(self):
        try:
            status, _headers, data = self.fetch_upstream(path="/admin/ver.api", method="GET")
            return status == 200 and b'"ok":true' in data
        except Exception:
            return False

    def admin_api(self, path):
        try:
            status, _headers, data = self.fetch_upstream(path=path, method="GET")
            if status == 200:
                value = json.loads(data.decode("utf-8"))
                return value if isinstance(value, (dict, list)) else {}
        except Exception:
            pass
        return {}

    def collect_dashboard(self):
        data = load_registry()
        users = data["users"]
        if "root" not in users and pop3_exists("root"):
            users["root"] = {"created": int(time.time())}
            with _registry_lock:
                fresh = load_registry()
                fresh["users"].setdefault("root", users["root"])
                save_registry(fresh)

        rows = []
        for login in sorted(users):
            if not re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", login):
                continue
            entry = users[login] if isinstance(users[login], dict) else {}
            recv_count = recv_bytes = 0
            status = "nopass"
            if entry.get("pop3"):
                try:
                    recv_count, recv_bytes = pop3_stats(login, entry["pop3"])
                    status = "ok"
                except Exception:
                    status = "err"
            elif not pop3_exists(login):
                status = "err"
            try:
                sent_items = load_sent(login)
            except ValueError:
                sent_items = []
            sent_bytes = sum(len(json.dumps(it, ensure_ascii=False).encode("utf-8")) for it in sent_items)
            rows.append({
                "login": login,
                "exit": entry.get("exit") or "",
                "last_login": entry.get("last_login") or 0,
                "received_count": recv_count,
                "received_bytes": recv_bytes,
                "sent_count": len(sent_items),
                "sent_bytes": sent_bytes,
                "status": status,
                "has_pass": bool(entry.get("pop3")),
            })
        rows.sort(key=lambda r: r["received_bytes"] + r["sent_bytes"], reverse=True)

        inbox_size = dir_size(os.path.join(MAILDIR, "inbox"))
        sent_size = dir_size(SENT_DIR)
        user_size = dir_size(os.path.join(MAILDIR, "usr")) + dir_size(os.path.join(MAILDIR, "keys"))
        try:
            user_size += os.path.getsize(REGISTRY_FILE)
        except OSError:
            pass
        total_all = dir_size(MAILDIR) + dir_size("/var/lib/onionmail") + dir_size("/etc/onionmail")
        service = max(0, total_all - inbox_size - sent_size - user_size)

        try:
            disk = os.statvfs("/")
            disk_total = disk.f_blocks * disk.f_frsize
            disk_free = disk.f_bavail * disk.f_frsize
        except OSError:
            disk_total = disk_free = 0

        stat = control_stat()
        if not stat.get("UpTime") or str(stat.get("UpTime")) == "0":
            stat["UpTime"] = process_uptime()
        threads = self.admin_api("/admin/threads.api")
        web = self.admin_api("/admin/stats.api")
        ver = self.admin_api("/admin/ver.api")
        exits = self.admin_api("/admin/elist.api")
        friends = self.admin_api("/admin/friends.api")

        def count_list(value):
            if isinstance(value, list):
                return len(value)
            if isinstance(value, dict):
                for key in ("elist", "list", "friends", "items"):
                    if isinstance(value.get(key), list):
                        return len(value[key])
                return len(value)
            return 0

        return {
            "users": rows,
            "user_total": len(rows),
            "user_known": sum(1 for r in rows if r["has_pass"]),
            "stat": stat,
            "uptime": process_uptime(),
            "threads": threads if isinstance(threads, dict) else {},
            "web": web if isinstance(web, dict) else {},
            "version": control_ver() or ((ver.get("ver") or "") + (" / api " + str(ver.get("apiVer")) if ver.get("apiVer") else "") if isinstance(ver, dict) else "—"),
            "exits": count_list(exits),
            "friends": count_list(friends),
            "disk": (disk_total, disk_free),
            "cat": {
                "received": inbox_size,
                "sent": sent_size,
                "user": user_size,
                "service": service,
                "total": total_all,
            },
        }

    def admin_dashboard(self):
        if not self.admin_ok():
            self.redirect("/logon.html")
            return
        message = ""
        error = ""
        if self.command == "POST":
            f = self.form()
            if f.get("csrf", [""])[0] != ADMIN_FORM_TOKEN:
                error = tr("CSRF error: reload the page and try again.")
            else:
                login = (f.get("login", [""])[0] or "").strip().lower()
                pop3 = (f.get("pop3", [""])[0] or "").strip()
                smtp = (f.get("smtp", [""])[0] or "").strip()
                exitlogin = (f.get("exit", [""])[0] or "").strip().lower()
                if not re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", login):
                    error = tr("Login: 3–16 characters, Latin letters/digits/._-, no @.")
                elif exitlogin and not valid_exit_login(exitlogin):
                    error = tr("Exit login: 3–30 characters, Latin letters/digits/._-.")
                else:
                    try:
                        pop3_stats(login, pop3)
                    except Exception:
                        error = tr("POP3 did not accept login “{login}” with this password.", login=login)
                    else:
                        register_user(login, smtp=smtp, pop3=pop3, exitlogin=exitlogin)
                        self.redirect("/admin/index.html?ok=1")
                        return
        else:
            qs = parse_qs(urlsplit(self.path).query)
            if qs.get("ok"):
                message = tr("Mailbox verified via POP3 and saved.")
        self.send_html(200, render_dashboard(self.collect_dashboard(), message, error))

    def do_HEAD(self):
        self.do_GET()

    def do_GET(self):
        set_request_lang(self)
        path = urlsplit(self.path).path
        if path == "/lang":
            qs = parse_qs(urlsplit(self.path).query)
            lang = qs.get("l", ["en"])[0]
            if lang not in LANGS:
                lang = "en"
            nxt = qs.get("next", ["/"])[0]
            if not nxt.startswith("/") or nxt.startswith("//"):
                nxt = "/"
            cookie = f"{LANG_COOKIE}={lang}; Path=/; HttpOnly; SameSite=Lax; Max-Age=31536000"
            self.redirect(nxt, [("Set-Cookie", cookie)])
            return
        if path == "/theme":
            qs = parse_qs(urlsplit(self.path).query)
            theme = qs.get("t", ["light"])[0].lower()
            if theme not in THEMES:
                theme = "light"
            nxt = qs.get("next", ["/"])[0]
            if not nxt.startswith("/") or nxt.startswith("//"):
                nxt = "/"
            cookie = f"{THEME_COOKIE}={theme}; Path=/; HttpOnly; SameSite=Lax; Max-Age=31536000"
            self.redirect(nxt, [("Set-Cookie", cookie)])
            return
        if path.startswith("/theme-assets/"):
            self.send_theme_asset(path)
            return
        if path == "/healthz":
            self.send_bytes(200, ONION.encode(), "text/plain")
            return
        if not om_admin_enabled() and (path == "/logon.html" or path.startswith("/admin")):
            self.send_html(404, "<h1>404</h1><p>Admin panel is disabled.</p>")
            return
        if path == "/register":
            self.redirect("/register.html")
            return
        if path == "/register.html":
            self.register_page()
            return
        if path in ("/admin", "/admin/", "/admin/index.html", "/admin/storage.html"):
            self.admin_dashboard()
            return
        if path == "/web" or path == "/web/":
            token, sess = get_session(self)
            if sess:
                self.redirect("/web/inbox")
            else:
                self.send_html(200, login_page())
            return
        if path.startswith("/web/"):
            self.web_get(path)
            return
        self.proxy()

    def do_POST(self):
        set_request_lang(self)
        path = urlsplit(self.path).path
        if not om_admin_enabled() and (path == "/logon.html" or path.startswith("/admin")):
            self.send_html(404, "<h1>404</h1><p>Admin panel is disabled.</p>")
            return
        if path == "/newuser.html":
            self.register_post()
            return
        if path == "/admin/storage.html":
            self.admin_dashboard()
            return
        if path.startswith("/web/"):
            self.web_post(path)
            return
        self.proxy()

    def fetch_upstream(self, body=None, path=None, method=None):
        headers = {}
        for k, v in self.headers.items():
            lk = k.lower()
            if lk in ("connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
                      "te", "trailers", "transfer-encoding", "upgrade", "accept-encoding", "content-length"):
                continue
            headers[k] = v
        headers["Accept-Encoding"] = "identity"
        conn = http.client.HTTPConnection(UPSTREAM[0], UPSTREAM[1], timeout=240)
        try:
            conn.request(method or self.command, path or self.path, body=body, headers=headers)
            resp = conn.getresponse()
            return resp.status, resp.getheaders(), resp.read()
        finally:
            conn.close()

    def relay(self, status, resp_headers, data):
        ctype = ""
        encoded = False
        for k, v in resp_headers:
            lk = k.lower()
            if lk == "content-type":
                ctype = v.lower()
            elif lk == "content-encoding":
                encoded = True
        if status == 200 and "text/html" in ctype and not encoded:
            data = translate_vendor_html(data)
            themed = inject_theme(data)
            if themed != data:
                data = themed
                resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]
        self.send_response(status)
        for k, v in resp_headers:
            lk = k.lower()
            if lk in ("connection", "keep-alive", "transfer-encoding", "content-length"):
                continue
            self.send_header(k, v)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(data)

    def register_page(self):
        try:
            status, resp_headers, data = self.fetch_upstream()
        except Exception as exc:
            self.log_message("upstream error: %s", exc)
            self.send_html(502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>")
            return
        reg = om_registration()
        if status == 200:
            if reg.get("exit_name_enabled", True) and REG_LOGIN_ROW in data:
                data = data.replace(REG_LOGIN_ROW, REG_LOGIN_ROW + b"\n\t" + reg_exit_row(), 1)
            if not reg.get("exit_enabled", True):
                data = re.sub(br"<tr><td>Exit Server:</td>.*?</tr>", b"", data, count=1, flags=re.S)
            if reg.get("voucher_required"):
                data = data.replace(b"* = Not required.", b"* = Required: enter a valid voucher code.")
            resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]
        self.relay(status, resp_headers, data)

    def register_post(self):
        raw = self.read_body()
        fields = parse_qs(raw.decode("utf-8", "replace"), keep_blank_values=True)
        login = (fields.get("om-localpart", [""])[0] or "").strip().lower()
        reg = om_registration()
        if not reg.get("exit_name_enabled", True):
            fields.pop("om-exitlogin", None)
        if not reg.get("exit_enabled", True):
            fields.pop("om-exit", None)
        exitlogin = (fields.pop("om-exitlogin", [""])[0] or "").strip().lower()
        voucher_code = (fields.get("om-voucher", [""])[0] or "").strip()
        if reg.get("voucher_required"):
            state = om_state()
            codes = {str(v.get("code", "")) for v in state.get("vouchers", []) if isinstance(v, dict)}
            if not voucher_code or voucher_code not in codes:
                self.send_html(400, small_page(
                    tr("Voucher required"),
                    tr("Registration is available only with a valid voucher code."),
                ))
                return

        if exitlogin and exitlogin != login and not valid_exit_login(exitlogin):
            self.send_html(400, small_page(
                tr("Invalid exit login"),
                tr("Exit login must be 4–32 characters: Latin letters, digits, dot, hyphen or underscore, and must not end with .onion, .sys, .op, .list."),
            ))
            return

        pairs = [(k, v) for k, vs in fields.items() for v in vs]
        body = urlencode(pairs).encode("utf-8")
        try:
            status, resp_headers, data = self.fetch_upstream(body)
        except Exception as exc:
            self.log_message("upstream error: %s", exc)
            self.send_html(502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>")
            return

        if exitlogin and exitlogin != login and status == 200 and b"Subscription complete" in data:
            alias_ok = False
            try:
                reply = control_command(f"addalias {exitlogin} {login}")
                alias_ok = reply.startswith("+OK")
                self.log_message("exit login %s -> %s: %s", exitlogin, login, reply)
            except Exception as exc:
                self.log_message("exit login %s -> %s failed: %s", exitlogin, login, exc)
            if alias_ok:
                data = data.replace(
                    f"{login}@{EXIT_DOMAIN}".encode("utf-8"),
                    f"{exitlogin}@{EXIT_DOMAIN}".encode("utf-8"),
                )
                resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]
            else:
                warn_text = tr(
                    "Account created, but exit login {exit} was not added (it may already be taken). The onionmail.info address remains {login}@{domain}.",
                    exit=exitlogin, login=login, domain=EXIT_DOMAIN,
                )
                warn = (
                    '<div class="om-warn">'
                    f'{html.escape(warn_text)}</div>'
                ).encode("utf-8")
                data = data.replace(b"</body>", warn + b"</body>", 1) if b"</body>" in data else data + warn
                resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]

            if exitlogin:
                note_text = tr(
                    "Webmail sign-in (/web): login {login} — without @, use the SMTP and POP3 passwords shown above. Exit login {exit}@{domain} is only for the address, not for signing in.",
                    login=login, exit=exitlogin, domain=EXIT_DOMAIN,
                )
                note = (
                    '<div class="om-note">'
                    f'{html.escape(note_text)}</div>'
                ).encode("utf-8")
                data = data.replace(b"</body>", note + b"</body>", 1) if b"</body>" in data else data + note
                resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]

        if status == 200 and b"Subscription complete" in data:
            if reg.get("voucher_required") and voucher_code:
                state = om_state()
                state["vouchers"] = [v for v in state.get("vouchers", [])
                                     if not (isinstance(v, dict) and str(v.get("code", "")) == voucher_code)]
                om_state_save(state)
            try:
                creds = parse_success_creds(data.decode("utf-8", "replace"))
                if creds:
                    register_user(login, smtp=creds[0], pop3=creds[1], exitlogin=exitlogin)
            except Exception as exc:
                self.log_message("registry registration update failed: %s", exc)

        self.relay(status, resp_headers, data)

    def proxy(self):
        length = self.headers.get("Content-Length", "0")
        body = None
        try:
            n = int(length)
        except ValueError:
            n = 0
        if n > 0:
            body = self.rfile.read(n)
        try:
            status, resp_headers, data = self.fetch_upstream(body)
        except Exception as exc:
            self.log_message("upstream error: %s", exc)
            self.send_html(502, "<h1>502</h1><p>OnionMail HTTP is not reachable.</p>")
            return

        if urlsplit(self.path).path == "/api/config.api" and status == 200:
            try:
                js = json.loads(data.decode("utf-8"))
                js.update(om_limits())
                data = json.dumps(js).encode("utf-8")
                resp_headers = [(k, v) for (k, v) in resp_headers if k.lower() != "content-length"]
            except Exception:
                pass

        self.relay(status, resp_headers, data)

    def web_get(self, path):
        token, sess = get_session(self)
        if not sess:
            self.send_html(200, login_page())
            return
        if path == "/web/inbox":
            self.send_html(200, inbox_page(sess))
        elif path == "/web/message":
            qs = parse_qs(urlsplit(self.path).query)
            try:
                n = int(qs.get("n", ["0"])[0])
            except ValueError:
                n = 0
            if n <= 0:
                self.redirect("/web/inbox")
                return
            self.send_html(200, message_page(sess, n))
        elif path == "/web/sent":
            self.send_html(200, sent_page(sess))
        elif path == "/web/sent/view":
            qs = parse_qs(urlsplit(self.path).query)
            self.send_html(200, sent_view(sess, qs.get("id", [""])[0]))
        elif path == "/web/compose":
            qs = parse_qs(urlsplit(self.path).query)
            self.send_html(200, compose_page(sess, ok=qs.get("ok", [""])[0]))
        else:
            self.send_html(404, page_shell(sess, "", f'<div class="err">{html.escape(tr("Not found."))}</div>'))

    def web_post(self, path):
        token, sess = get_session(self)
        if path == "/web/login":
            f = self.form()
            user = (f.get("username", [""])[0] or "").strip().lower()
            user = user.split("@", 1)[0]
            smtp = f.get("smtp", [""])[0] or ""
            pop3 = f.get("pop3", [""])[0] or ""
            if not re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", user) or not smtp or not pop3:
                self.send_html(400, login_page(tr("Fill in all fields and check the username.")))
                return
            try:
                p = pop3_conn(user, pop3)
                p.quit()
                s = smtp_conn(user, smtp)
                s.quit()
            except Exception:
                time.sleep(1)
                hint = ""
                try:
                    reply = control_command("alias " + user)
                    if reply.startswith("+OK "):
                        main = reply[4:].strip().lower()
                        if re.match(r"^[a-z0-9][a-z0-9._-]{1,14}[a-z0-9]$", main):
                            hint = " " + tr("It looks like “{user}” is an exit login. The main login for signing in is “{main}”.", user=user, main=main)
                except Exception:
                    pass
                self.send_html(403, login_page(
                    tr("Wrong username or password. In the “Username” field use the main mailbox name (localpart) without @ — not the exit login. SMTP and POP3 passwords are different.") + hint))
                return
            try:
                register_user(user, smtp=smtp, pop3=pop3, last_login=time.time())
            except Exception as exc:
                self.log_message("registry login update failed: %s", exc)
            token = new_session(user, smtp, pop3)
            self.redirect("/web/inbox", [("Set-Cookie", session_cookie(token))])
            return

        if not sess:
            self.redirect("/web")
            return

        if path == "/web/logout":
            f = self.form()
            if f.get("csrf", [""])[0] != sess["csrf"]:
                self.send_html(403, page_shell(sess, "", f'<div class="err">{html.escape(tr("CSRF error."))}</div>'))
                return
            with _sessions_lock:
                _sessions.pop(token, None)
            self.redirect("/web", [("Set-Cookie", clear_cookie())])
            return

        if path == "/web/send":
            f = self.form()
            if f.get("csrf", [""])[0] != sess["csrf"]:
                self.send_html(403, compose_page(sess, error=tr("CSRF error.")))
                return
            to = (f.get("to", [""])[0] or "").strip()
            subject = (f.get("subject", [""])[0] or "").strip()
            body = f.get("body", [""])[0] or ""
            if not to or len(to) > 320 or "\r" in to or "\n" in to:
                self.send_html(400, compose_page(sess, error=tr("Specify a valid recipient.")))
                return
            if "\r" in subject or "\n" in subject:
                self.send_html(400, compose_page(sess, error=tr("Invalid subject.")))
                return
            if len(body) > 100_000:
                self.send_html(400, compose_page(sess, error=tr("Message is too large.")))
                return
            msg = EmailMessage()
            msg["From"] = f"{sess['user']}@{ONION}"
            msg["To"] = to
            msg["Subject"] = subject
            msg["Date"] = email.utils.formatdate(localtime=True)
            msg["Message-ID"] = email.utils.make_msgid(domain=ONION)
            msg.set_content(body)
            try:
                s = smtp_conn(sess["user"], sess["smtp"])
                s.send_message(msg)
                s.quit()
            except Exception as exc:
                self.log_message("send error: %s", exc)
                self.send_html(502, compose_page(sess, error=tr("Could not send: {err}", err=str(exc))))
                return
            add_sent(sess["user"], {
                "id": secrets.token_urlsafe(12),
                "to": to,
                "subject": subject,
                "date": time.strftime("%Y-%m-%d %H:%M"),
                "body": body,
            })
            self.redirect("/web/sent?ok=1")
            return

        self.send_html(404, page_shell(sess, "", f'<div class="err">{html.escape(tr("Not found."))}</div>'))


def main():
    os.makedirs(SENT_DIR, mode=0o700, exist_ok=True)
    try:
        registry_seed()
    except Exception as exc:
        sys.stdout.write(f"registry seed failed: {exc}\n")
    sys.stdout.write(f"front ready for {ONION} (webmail /web, upstream {UPSTREAM[0]}:{UPSTREAM[1]})\n")
    sys.stdout.flush()
    httpd = ThreadingHTTPServer((FRONT_BIND, FRONT_PORT), Handler)
    httpd.daemon_threads = True
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
