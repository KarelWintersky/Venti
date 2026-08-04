# HOW-TO — Venti: запуск, логи, конфиг

Venti — FastCGI-сервер для исполнения CGI-скриптов на Perl через пул персистентных
воркеров (nginx → unix-сокет → Venti → perl).

---

## 1. Сборка

```sh
make build            # → build/venti (версия инжектится из git)
make test             # go test -v ./...
```

Бинарник: `build/venti`.

---

## 2. Запуск вручную (с ключом --config)

```sh
./build/venti --config configs/venti.yaml          # свой конфиг
./build/venti --config /etc/venti/venti.yaml       # системный
./build/venti --config configs/venti.yaml --verbose  # логи ещё и в консоль
```

Без `--config` Venti сам ищет файл в порядке: `/etc/venti/venti.yaml`,
`./venti.yaml`, `./configs/venti.yaml`, `/usr/local/etc/venti/venti.yaml`.
Если ни одного нет — падает с подсказкой.

Справка: `./build/venti --help`, версия: `./build/venti --version`.

### Сигналы (ручное управление)

| Сигнал   | Что делает                                              |
|----------|---------------------------------------------------------|
| `SIGHUP` | перечитывает конфиг и живьём применяет настройки пула   |
| `SIGINT` / `SIGTERM` | graceful shutdown: закрывает сокет, ждёт идущие запросы ≤ `shutdown_grace`, убивает воркеров, exit 0 |

---

## 3. Установка и запуск через systemd

Файлы для системы лежат в `scripts/`:

- `scripts/venti.service` — юнит
- `scripts/venti-nginx.conf` — конфиг nginx
- `scripts/venti-run.sh` — скрипт запуска (поиск конфига + exec)

Установка:

```sh
sudo make install    # копирует бинарник в /usr/local/bin, конфиг в /etc/venti/, создаёт каталоги
```

Предусловия:

```sh
sudo useradd -r -s /bin/false venti          # если ещё нет
sudo mkdir -p /run/venti /var/log/venti
sudo chown venti:venti /run/venti /var/log/venti
sudo cp scripts/venti.service /etc/systemd/system/
sudo systemctl daemon-reload
```

Юнит запускается как `venti:venti` с `ExecStart=/usr/local/bin/venti --config /etc/venti/venti.yaml`.

Управление:

```sh
sudo systemctl start venti
sudo systemctl enable venti
sudo systemctl status venti
sudo systemctl reload venti     # = kill -HUP → перечитать конфиг на лету
sudo systemctl restart venti
sudo systemctl stop venti
```

nginx (`scripts/venti-nginx.conf`): `fastcgi_pass unix:/run/venti/venti.sock;`
и обязательный параметр `SCRIPT_FILENAME`. Корень скриптов, как правило,
`/var/www/scripts` — проверь `document_root` в конфиге.

---

## 4. Где смотреть логи

Поведение логирования задаётся секцией `logging` конфига.

| Куда пишутся логи | Условие |
|---|---|
| консоль (stdout) | `logging.file` пуст, ИЛИ задан флаг `--verbose` |
| файл `logging.file` | `logging.file` задан |
| journald | при запуске через systemd (даже если `logging.file` пуст) |

То есть:

- Вручную с дефолтным конфигом — `tail -f logs/access.log` (или путь из `logging.file`).
- Вручную «наблюдать вживую»: `./build/venti --config configs/venti.yaml --verbose` —
  логи дублируются в консоль.
- systemd: `journalctl -u venti -f` (или `-u venti -f --since "10 min ago"`).

Формат записей: JSON или текст (`logging.format: json|text`). Пример записи:

```json
{"time":"2026-08-04T13:06:49.14Z","level":"INFO","msg":"🌪️ Anemo power awakened","bards_summoned":2,"max_power":4}
```

Уровень задаётся `logging.level` (`debug`, `info`, `warn`, `error`). При `debug`
раз в 30 с дополнительно пишется статистика пула (сколько бардов на сцене / на покое).

---

## 5. Конфиг (YAML)

Формат — YAML. Пример со всеми ключами:

```yaml
# Слушатель
listener:
  type: unix            # unix или tcp
  address: /run/venti/venti.sock   # путь к сокету или адрес:порт для tcp
  mode: 0660            # права на unix-сокет (по умолчанию 0660)

# Путь к Perl
perl_path: /usr/bin/perl   # обязателен, должен существовать

# Пул воркеров (сила Анемо)
anemo_power:
  min_bards: 4          # минимум живых perl-процессов (по умолчанию 1)
  max_bards: 20         # потолок (клампится до min, если меньше)
  idle_timeout: 60      # сек бездействия, после чего лишний воркер уходит на покой
  max_lifetime: 3600    # сек жизни воркера, после которых он заменяется

# Таймауты (секунды)
timeouts:
  song_duration: 30     # лимит исполнения одного скрипта
  tune_up: 5            # лимит ожидания свободного воркера
  shutdown_grace: 10    # сколько ждать идущие запросы при остановке

# Логирование
logging:
  level: info           # debug | info | warn | error
  file: /var/log/venti/access.log   # пусто → только консоль
  format: json          # json | text

# Ограничения
limits:
  max_verse_size: 10485760    # макс. тело запроса в байтах (10 MB), иначе 413
  max_songs_per_bard: 1000    # песен на воркера до замены
  # document_root: /var/www   # разрешать SCRIPT_FILENAME только внутри этого каталога
```

### Секции и дефолты

| Ключ | Дефолт | Комментарий |
|---|---|---|
| `listener.type` | — | только `unix` или `tcp`, обязателен |
| `listener.address` | — | обязателен |
| `listener.mode` | `0660` | права сокета; владелец и группа — от юзера Venti |
| `perl_path` | `/usr/bin/perl` | бинарник должен существовать |
| `anemo_power.min_bards` | `1` | меньше 1 → 1 |
| `anemo_power.max_bards` | = min | клампится снизу до min |
| `anemo_power.idle_timeout` | 0 (не убирать) | сек |
| `anemo_power.max_lifetime` | 0 (не менять) | сек |
| `timeouts.song_duration` | `30` | сек |
| `timeouts.tune_up` | `5` | сек |
| `timeouts.shutdown_grace` | `10` | сек |
| `logging.level` | `info` | debug/info/warn/error |
| `logging.file` | пусто | пусто → stdout |
| `logging.format` | `text` | json/text |
| `limits.max_verse_size` | `10 MiB` | байт |
| `limits.max_songs_per_bard` | `1000` | шт |
| `limits.document_root` | пусто | пусто → без ограничений |

### Живое обновление (SIGHUP / systemctl reload)

При `SIGHUP` Venti перечитывает конфиг и на лету применяет `anemo_power`
(min/max/idle/lifetime/songs_per_bard). Изменения `listener.*`, `logging.*`,
`timeouts.*` и `limits.*` требуют рестарта (`systemctl restart venti`).

### Безопасность (кратко)

- Сокет по умолчанию `0660` — доступ только у владельца (venti) и его группы.
- `limits.document_root` ограничивает SCRIPT_FILENAME каталогом (защита от обхода через `..`).
- `limits.max_verse_size` ограничивает размер тела запроса.
- Окружение сервера не утекает в CGI: в скрипты уходит только то, что передал nginx.
- systemd-юнит: `ProtectSystem=strict`, `NoNewPrivileges`, `PrivateTmp` и т.д.

---

## 6. Проверка после запуска

```sh
# процесс на месте?
systemctl status venti          # или ps aux | grep venti

# воркеры живы? (имя переименовывается в путь скрипта, см. pgrep по perl-воркерам)
pgrep -f '^perl -e'             # базовый; активные воркеры называются по SCRIPT_FILENAME

# запрос через nginx
curl -i http://localhost/test.pl
```

Скрипт должен сам печатать заголовки, минимум `Content-Type`:

```perl
#!/usr/bin/perl
print "Content-Type: text/html\n\n";
print "<h1>Hello from Venti!</h1>";
```

`exit N` в скрипте → Venti отвечает 500 (вывод отбрасывается).
`die` → 500 с текстом ошибки в логе.
Несуществующий скрипт → 500, в логе `script not found: ...`.
