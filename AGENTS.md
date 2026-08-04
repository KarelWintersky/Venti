# AGENTS.md — Venti (FastCGI-пул для Perl-CGI)

Технический справочник для агента. Всё, что нужно для работы с проектом — без перечитывания кода.

## Overview

- Репозиторий: `/var/www.projects/Venti/`
- Go-модуль `venti`, `go 1.21`, единственная зависимость `gopkg.in/yaml.v3`
- Что это: FastCGI-сервер (nginx → unix-сокет → Venti → пул персистентных perl-процессов), исполняющий CGI-скрипты. Perl не запускается на каждый запрос: воркеры живут, скрипт выполняется через `do` внутри воркера.
- Архитектура: `lyre` (FastCGI-сервер) → `bard` (Stage, воркер, протокол) → `anemo` (пул).
- Семантика «бардов/песен»: бард = perl-воркер, песня = CGI-запрос.

## go.mod (факты)

- `module venti`, `go 1.21`
- require: только `gopkg.in/yaml.v3 v3.0.1`
- Тестовых фреймворков нет — чистый `testing` stdlib.

## Structure

```
cmd/venti/main.go           # точка входа, флаги, сигналы (SIGHUP/SIGINT/SIGTERM), slog
internal/lyre/server.go     # FastCGI-сервер, WaitGroup для in-flight, Drain
internal/bard/
  handler.go                # Stage (обработчик запросов), Troupe/Performer (воркер), publishPerformance
  worker.pl                 # perl-воркер (//go:embed), протокол и перехват exit/die
  handler_test.go           # publishPerformance / withinRoot / prepareLyrics
  worker_test.go            # интеграционные тесты на реальном perl
internal/anemo/pool.go      # пул (таверна), balance/cleanupIdle, ApplyConfig, Close
internal/config/config.go   # LoadConfig/Validate + getters таймаутов
configs/venti.yaml          # рабочий конфиг
scripts/                    # venti.service, venti-nginx.conf, venti-run.sh
build/venti                 # артефакт сборки (make build)
```

## Протокол воркера (worker.pl)

Запрос (в stdin воркера):
- строка `<env_len> <stdin_len>\n` (байты), затем env-блок (строки `KEY=VALUE`, разделённые `\n`), затем тело запроса.

Ответ (в stdout воркера):
- строка `<out_len> <err_len> <status>\n`, затем stdout-данные, затем stderr-данные.

Ключевые механики (не ломать!):
- stdin читается ТОЛЬКО через `sysread` (`read_header` по 1 байту, `read_exact`) — PerlIO read-ahead ломает ребайнд STDIN.
- `%ENV = ()` очищается на каждый запрос; скрипт задаётся переменной `SCRIPT_FILENAME` (из неё же воркер берёт путь для `do`). PATH по умолчанию подкладывает Go.
- `exit N` перехватывается через `BEGIN { *CORE::GLOBAL::exit = ... }`: код пишется в `$main::VENTI_EXIT_CODE` ДО `die "__VENTI_EXIT__\n"` — потому что `do` проглатывает die. Статус берётся из переменной.
- `die` → stderr в `err_data`, `status 1`. `exit N` → `status N` (Go-сторона отдаёт 500 и отбрасывает вывод).
- вывод/ввод ребайндится через `File::Temp`; обязательны: `close(STDIN)` перед `open(STDIN,'<&',$in)` (голый dup не работает), сохранение/восстановление `select()`, `seek(0)` перед чтением, сохранение/восстановление STDIN/OUT/ERR.
- несуществующий/нечитаемый скрипт → ответ `status 1` с текстом `script not found: ...`.
- `$0 = $script` — воркер переименовывается в имя скрипта (ломает grep по `perl -e` при диагностике!).
- Воркер завершается только по EOF на stdin (родитель умер/закрыл пайп).

## Ключевые классы и API

### `bard.Troupe` / `bard.Performer` (handler.go)

- `Troupe{PerlPath string, SongTimeout time.Duration}`; `Recruit() (*Performer, error)` — запускает `perl -e <workerCode>` (worker.pl через `//go:embed`, импорт `_ "embed"`), stderr дренится в `io.Discard`.
- `Performer.Sing(ctx, songPath string, lyrics []string, melody []byte) ([]byte, error)` — один запрос. Таймаут = `songTimeout` (default 30s), `ctx.Deadline` может сократить. При любой ошибке воркер убивается (`p.kill()`). `status != 0` → ошибка с текстом stderr.
- `Rest() error` — убить воркер (идемпотентно). `IsHealthy() bool`, `GetSongsCount()`, `GetBirthTime()`, `GetLastSongTime()`, `GetName()`.
- `p.kill()`: `dead.Swap(true)`, Kill+Wait, закрыть пайпы.

### `bard.Stage` (handler.go)

- `NewStage(cfg *config.Config, anemoPower AnemoPower) *Stage`
- `Perform(w, r)`: берёт `SCRIPT_FILENAME` из `fcgi.ProcessEnv(r)` (НЕ из `r.Header` — Go stdlib `cgi.RequestFromMap` кладёт в header только `HTTP_*`!), проверяет `document_root` (`withinRoot`), читает тело через `io.LimitReader` (`max_verse_size`, иначе 413), ждёт барда с таймаутом `tune_up` (CallBard), `Sing`, `publishPerformance`.
- `prepareLyrics(r) []string` — строит CGI-env из `fcgi.ProcessEnv(r)` + HTTP-заголовки (`HTTP_*`), REMOTE_ADDR/PORT, `HTTPS=on` при TLS. НЕ использует `os.Environ()` (нет утечки окружения сервера) — только базовый PATH, перекрываемый fastcgi_param.
- `publishPerformance(w, performance []byte)` — парсит заголовки CGI-ответа по `\r\n\r\n` ИЛИ `\n\n`, `Status:` → `WriteHeader`, остальные в `w.Header()`, тело пишет последним.

### `anemo.AnemoPower` (pool.go)

- `NewAnemoPower(cfg *PowerConfig, factory BardFactory, logger Logger) (*AnemoPower, error)` — призывает MinBards, запускает `flow()` (тик каждые 10с).
- `PowerConfig{MinBards, MaxBards, IdleTimeout, MaxLifetime, MaxSongsPerBard}`
- `CallBard(ctx) (bard.Bard, error)` — из таверны (buffered chan, cap MaxBards) или ctx-таймаут.
- `ReleaseBard(b bard)` — бард на покой если unhealthy / songs >= MaxSongsPerBard / возраст > MaxLifetime; иначе назад в таверну.
- `ApplyConfig(newCfg *PowerConfig) error` — живое обновление параметров + добор бардов до нового минимума (для SIGHUP).
- `Close() error` — cancel+wait flow, всех бардов Rest.
- `GetStats()` — map для debug-логов.

### `lyre.Lyre` (server.go)

- `NewLyre(cfg, stage, logger)`, `Play() error` (fcgi.Serve), `Silence() error` (закрыть listener), `Drain(grace time.Duration)` (ждать in-flight ≤ grace).
- `Lyre` реализует `ServeHTTP` — через WaitGroup трекает идущие запросы.

### `config` (config.go)

- `LoadConfig(path)` + `Validate()` (клампит min/max, дефолты). Getters: `GetSongDuration()`, `GetTuneUpTimeout()`, `GetShutdownGrace()`, `GetIdleTimeout()`, `GetMaxLifetime()`.
- `Listener.Mode os.FileMode` — права сокета (default 0660). `Limits.DocumentRoot` — ограничение SCRIPT_FILENAME.

## main.go: флаги и сигналы

- Флаги: `--config`, `--version`, `--help`. Config ищется по умолчанию в `/etc/venti/venti.yaml`, `./venti.yaml`, `./configs/venti.yaml`.
- Логгер: `slog` (JSON/text по конфигу), файл из `logging.file`.
- **SIGHUP**: перечитать конфиг → `anemoPower.ApplyConfig(...)` → `*cfg = *newCfg` (Stage читает лимиты по указателю). При ошибке — лог, конфиг не меняется.
- **SIGINT/SIGTERM**: `lyreServer.Silence()` (закрыть listener) → `Play()` возвращает `net.ErrClosed` → `Drain(shutdown_grace)` → возврат из main → deferred `anemoPower.Close()` (убивает воркеров). Exit code 0, воркеры без сирот.
- Версия: `make build` инжектит через `-ldflags "-X main.Version=$(git describe --tags --always --dirty) -X main.BuildTime=..."`. Прямой `go build` → fallback на `debug.ReadBuildInfo()` (`vcs.revision` 7 символов + `vcs.time`), иначе `dev`.

## Tests

Команды: `go test ./...`, `go vet ./...` (оба чистые).

23 top-level теста + 2 подтеста (25 RUN), все PASS, 0 SKIP (реальный прогон):
- `internal/config`: 4 (+2 подтеста) — дефолты, явные значения, клампинг max_bards, ошибки парсинга.
- `internal/bard/handler_test.go`: 7 — publishPerformance (CRLF/LF/Status/без заголовков), withinRoot (обход через `..`), prepareLyrics (нет утечки os.Environ, HTTPS, HTTP_-заголовки).
- `internal/bard/worker_test.go`: 6 — интеграция на реальном `/usr/bin/perl` (skip, если perl нет): Sing, stdin-тело, exit 42, missing script, выживание состояния между запросами, Rest идемпотентен.
- `internal/anemo`: 6 — summon MinBards, roundtrip Call/Release, таймаут CallBard, ApplyConfig (+invalid), Close убивает всех.

## Wiring

- Сборка: `make build` (ldflags + версия), `make test`, `make install` (система), `make build-static`.
- systemd: `scripts/venti.service` — `ExecReload=/bin/kill -HUP $MAINPID`, юзер `venti:venti`, `ProtectSystem=strict`.
- nginx: `scripts/venti-nginx.conf` — `fastcgi_pass unix:/run/venti/venti.sock`, обязательный `fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;`.
- Конфиг: `configs/venti.yaml` (см. комментарии в файле): listener(type/address/mode), perl_path, anemo_power(min/max/idle_timeout/max_lifetime), timeouts(song_duration/tune_up/shutdown_grace), logging(level/file/format), limits(max_verse_size/max_songs_per_bard/document_root).

## Status / TODO

- Готово (2026-08-04): slog-логгер вместо сломанного SimpleLogger; реальный пул персистентных воркеров; парсинг CGI-заголовков `\n\n`/`\r\n\r\n`; graceful shutdown (Drain in-flight, exit 0, без сирот); живой SIGHUP-релоад; безопасность (сокет 0660, document_root, max_verse_size, убрана утечка os.Environ); версия при сборке; 23+2 теста.
- Заметка: воркер переименовывает себя в `$0 = $script` — при подсчёте процессов `pgrep 'perl -e'` не видит «активных» воркеров (не баг).
- Тегов git нет (коммиты подписаны 0.1.x в месседжах); `git describe` даёт хэш.
- Не в репо (харнесс для ручного e2e): сырой FastCGI-клиент `/tmp/opencode/fcgi_client`.
