# Makefile
.PHONY: build clean install test run

BINARY_NAME=venti
BUILD_DIR=build
VERSION=1.0.0
GOOS=linux
GOARCH=amd64

# Флаги линковки
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -s -w"

build:
	@echo "🎵 Building Venti for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/venti
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"
	@ls -lh $(BUILD_DIR)/$(BINARY_NAME)

# Статическая сборка без зависимостей от libc
build-static:
	@echo "🎵 Building static Venti..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w -extldflags '-static'" \
		-tags netgo \
		-installsuffix netgo \
		-o $(BUILD_DIR)/$(BINARY_NAME)-static \
		./cmd/venti
	@echo "✅ Static build complete"

install: build
	@echo "🎵 Installing Venti..."
	sudo mkdir -p /etc/venti
	sudo mkdir -p /var/log/venti
	sudo mkdir -p /run/venti
	sudo mkdir -p /var/www/scripts
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	sudo cp configs/venti.yaml /etc/venti/
	@echo "✅ Venti installed successfully!"

clean:
	@echo "🧹 Cleaning..."
	rm -rf $(BUILD_DIR)
	go clean -cache

test:
	go test -v ./...

run: build
	./$(BUILD_DIR)/$(BINARY_NAME)

# Сборка для разных архитектур
build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-arm64 ./cmd/venti

build-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -o $(BUILD_DIR)/$(BINARY_NAME)-arm ./cmd/venti

# Создание архива с бинарником
package: build
	@mkdir -p $(BUILD_DIR)/package
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/package/
	@cp configs/venti.yaml $(BUILD_DIR)/package/
	@cp scripts/install.sh $(BUILD_DIR)/package/
	@cd $(BUILD_DIR) && tar czf venti-$(VERSION)-linux-amd64.tar.gz package/
	@echo "✅ Package created: $(BUILD_DIR)/venti-$(VERSION)-linux-amd64.tar.gz"

help:
	@echo "Available commands:"
	@echo "  make build        - Build Venti for Linux"
	@echo "  make build-static - Build static binary (no libc dependencies)"
	@echo "  make install      - Install Venti to system"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make run          - Run Venti locally"
	@echo "  make package      - Create distribution package"

