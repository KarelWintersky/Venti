.PHONY: build clean install test run

BINARY_NAME=venti
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GOOS=linux
GOARCH=amd64

# Флаги линковки с передачей версии
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -s -w"

build:
	@echo "🎵 Building Venti $(VERSION) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/venti
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"
	@ls -lh $(BUILD_DIR)/$(BINARY_NAME)

# Сборка для разных архитектур
build-amd64:
	$(MAKE) build GOOS=linux GOARCH=amd64

build-arm64:
	$(MAKE) build GOOS=linux GOARCH=arm64

build-arm:
	$(MAKE) build GOOS=linux GOARCH=arm

# Сборка всех архитектур
build-all: build-amd64 build-arm64 build-arm

# Статическая сборка
build-static:
	@echo "🎵 Building static Venti..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w -extldflags '-static' -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)" \
		-tags netgo \
		-installsuffix netgo \
		-o $(BUILD_DIR)/$(BINARY_NAME)-static \
		./cmd/venti
	@echo "✅ Static build complete: $(BUILD_DIR)/$(BINARY_NAME)-static"

install: build
	@echo "🎵 Installing Venti..."
	sudo mkdir -p /etc/venti
	sudo mkdir -p /var/log/venti
	sudo mkdir -p /run/venti
	sudo mkdir -p /var/www/scripts
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@if [ ! -f /etc/venti/venti.yaml ]; then \
		sudo cp configs/venti.yaml /etc/venti/; \
		echo "📝 Default config copied to /etc/venti/venti.yaml"; \
	fi
	@echo "✅ Venti installed successfully!"
	@echo ""
	@echo "To start Venti:"
	@echo "  sudo venti"
	@echo ""
	@echo "Or run as service:"
	@echo "  sudo systemctl start venti"

clean:
	@echo "🧹 Cleaning..."
	rm -rf $(BUILD_DIR)
	go clean -cache

test:
	go test -v ./...

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --config configs/venti.yaml

# Установка с отладочной сборкой
install-debug: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/venti-debug
	sudo chmod +x /usr/local/bin/venti-debug
	@echo "✅ Venti debug installed as venti-debug"

# Проверка конфига
check-config:
	@echo "Checking configuration..."
	@./$(BUILD_DIR)/$(BINARY_NAME) --config configs/venti.yaml --version

help:
	@echo "Available commands:"
	@echo "  make build           - Build Venti for Linux (default architecture)"
	@echo "  make build-amd64     - Build for AMD64"
	@echo "  make build-arm64     - Build for ARM64"
	@echo "  make build-arm       - Build for ARM"
	@echo "  make build-static    - Build static binary (no libc dependencies)"
	@echo "  make install         - Install Venti to system"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make test            - Run tests"
	@echo "  make run             - Run Venti locally with default config"
	@echo "  make check-config    - Check configuration"
	@echo ""
	@echo "Current version: $(VERSION)"