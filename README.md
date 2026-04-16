# Venti

```shell

# Показать справку
./build/venti --help

# Показать версию
./build/venti --version

# Запуск с системным конфигом
sudo ./build/venti

# Запуск с кастомным конфигом
./build/venti --config /path/to/custom/config.yaml

# Перезагрузка конфигурации (после изменения)
sudo systemctl reload venti

# Просмотр статистики в debug режиме
./build/venti --config configs/venti-debug.yaml

```