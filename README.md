# OnionMail Reborn

Docker-сборка OnionMail 1.12 с улучшенными веб-интерфейсом, админ-панелью,
многоязычным интерфейсом. с лицензией GPL-3

## Возможности

- OnionMail SMTP/POP3, встроенный HTTP и Tor hidden service.
- Веб-почта `/web`: вход по основному логину + паролям SMTP и POP3.
- поддержка exit-логинов.
- Улучшенная админ  панель `/admin/index.html`: статистика, диаграммы, размеры ящиков , (она read-only редактировать невозможно ничего кроме vouchers)
  реестр пользователей и ручное добавление ящиков.
- Языки интерфейса: English (по умолчанию), Deutsch, Français, Русский, Español.
  Переключатель `Language` находится в верхнем углу; выбор хранится в cookie.
- Светлая и тёмная темы для встроенных страниц OnionMail, веб-почты и админ-панели.
  Тема по умолчанию задаётся в `om setup` → `7. Design`, а переключатель
  «солнце/луна» рядом с языком сохраняет выбор в браузере.
- Свой логотип: в `7. Design` выберите `Add logo` и укажите путь к PNG/JPG
  внутри контейнера. `Reset logo` возвращает стандартный логотип OnionMail.
  Файл с хоста можно заранее скопировать:
  `docker cp logo.png onionmail:/tmp/logo.png`.
- команды `om setup`, `om status`, `om restart`, `om check`
## Быстрый старт

```bash
cp .env.example .env
# укажите параметры в .env под ваш сервер (все равно потом можно все это указать по docker exec (название контейнера) om setup)
docker compose build
docker compose up -d
```

Проверка:

```bash
docker compose ps
curl -fsS http://127.0.0.1:18000/healthz
```

Для публикации hidden service на хосте (рекомендуется) запустите:

```bash
sudo ./host_tor_setup.sh
```

и укажите полученный `.onion` в `ONION_ADDRESS` в `.env`.

## Переменные из .env

| Переменная | Назначение |
|---|---|
| `ONIONMAIL_NICK` | Внутреннее имя сервера, по умолчанию |
| `ONIONMAIL_DISPLAY_NAME` | Отображаемое имя сервера |
| `ONION_ADDRESS` | Адрес.onion (обязателен при USE_HOST_TOR=1) |
| `USE_HOST_TOR` | 1 — использовать Tor хоста, 0 — Tor внутри контейнера |
| `TOR_IP`, `TOR_PORT` | SOCKS-прокси Tor |
| `SYSOP_SMTP_PASSWORD`, `SYSOP_POP3_PASSWORD` | Необязательные пароли sysop при первичной генерации |
| `CAPTCHA_DISABLE` | yes не рекомендуется; обычно CAPTCHA включена |

Секреты и пароли не коммитятся: `.env`, `sysop.txt`, `keyblock.txt` и
`registry.json` остаются в Docker-томах/локально.

## Языки

Все пользовательские страницы проекта (`/web`, `/register.html`-надстройки,
`/admin/index.html`) переведены на 5 языков. Встроенные страницы OnionMail
дополнительно переводятся, выбор языка справа сверху
сохраняется для всего сайта.

- English — стандартный
- Deutsch
- Français
- Русский
- Español

## Структура

``` structure
Dockerfile                    сборка образа
docker-compose.yml            запуск контейнера
entrypoint.sh                 инициализация OnionMail, Tor, политик и front
front.py                      веб-почта, прокси, темы, админ-панель
theme/                        локальные изображения облаков и иконки тем
voucher.py                    CLI выпуска/проверки voucher-кодов
host_tor_setup.sh             публикация hidden service на хосте
onionmail_*.deb*              пакет OnionMail и контрольная сумма
.env.example                  шаблон конфигурации
```

## Примечания

Это независимый форк, не связанный с onionmail.info.

## Скриншоты

![главный экран][main_screen.png]
![лимиты(их можно менять)][limits.png]
