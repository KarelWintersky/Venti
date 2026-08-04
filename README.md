# Venti

FastCGI-сервер для Perl-CGI: nginx → unix-сокет → Venti → пул персистентных
perl-процессов. Perl не стартует на каждый запрос — воркеры живут, скрипт
выполняется внутри воркера через `do`.

Полная документация: **[HOW-TO.md](HOW-TO.md)** — запуск, логи, конфиг.

```shell
# Показать справку
./build/venti --help

# Показать версию
./build/venti --version

# Сборка
make build

# Запуск с конфигом
./build/venti --config configs/venti.yaml

# Запуск + дублирование логов в консоль
./build/venti --config configs/venti.yaml --verbose

# Живая перезагрузка конфигурации (пул обновится на лету)
sudo systemctl reload venti
```

Конфиг — YAML, пример в [`configs/venti.yaml`](configs/venti.yaml).
Скрипты для systemd и nginx: `scripts/venti.service`, `scripts/venti-nginx.conf`.
