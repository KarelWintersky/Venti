#!/bin/bash
# Venti startup script

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}"
echo "╔═══════════════════════════════════════════╗"
echo "║               V E N T I                   ║"
echo "║     Anemo Archon FastCGI Pool             ║"
echo "╚═══════════════════════════════════════════╝"
echo -e "${NC}"

# Поиск конфига
CONFIG_FILE=""
if [ -f "/etc/venti/venti.yaml" ]; then
    CONFIG_FILE="/etc/venti/venti.yaml"
    echo -e "${GREEN}✓ Using system config: ${CONFIG_FILE}${NC}"
elif [ -f "./venti.yaml" ]; then
    CONFIG_FILE="./venti.yaml"
    echo -e "${GREEN}✓ Using local config: ${CONFIG_FILE}${NC}"
elif [ -f "./configs/venti.yaml" ]; then
    CONFIG_FILE="./configs/venti.yaml"
    echo -e "${GREEN}✓ Using dev config: ${CONFIG_FILE}${NC}"
else
    echo -e "${RED}✗ Configuration file not found!${NC}"
    echo ""
    echo "Please create config file at one of these locations:"
    echo "  /etc/venti/venti.yaml"
    echo "  ./venti.yaml"
    echo "  ./configs/venti.yaml"
    exit 1
fi

# Проверяем бинарник
if [ ! -f "./build/venti" ]; then
    echo -e "${YELLOW}⚠ Venti binary not found. Building...${NC}"
    make build
fi

# Запускаем
echo -e "${GREEN}🎵 Starting Venti...${NC}"
echo ""

exec ./build/venti --config "$CONFIG_FILE"