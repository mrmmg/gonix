BINARY      := gonix
CMD_PATH    := ./cmd/gonix
VERSION     := $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X github.com/mrmmg/gonix/internal/tui.Version=$(VERSION)

# Layout matches install.sh (the prebuilt-release installer), so a
# source-built `make install` and a `curl | bash` install end up identical.
INSTALL_DIR     := /opt/gonix
BIN_DIR         := $(INSTALL_DIR)/bin
GLOBAL_BIN_DIR  := /usr/local/bin
CONFIG_DIR      := /etc/gonix
CONFIG_FILE     := $(CONFIG_DIR)/gonix.yaml

.PHONY: build
build:
	go build -o $(BINARY) $(CMD_PATH)

.PHONY: release
release:
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD_PATH)

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -l .

.PHONY: lint
lint: vet fmt

.PHONY: install
install: release
	@echo "==> Installing $(BINARY) to $(BIN_DIR)"
	sudo mkdir -p "$(BIN_DIR)" "$(CONFIG_DIR)/backups" "$(CONFIG_DIR)/accesslists" /var/log/gonix
	sudo chmod 750 "$(CONFIG_DIR)" "$(CONFIG_DIR)/backups" "$(CONFIG_DIR)/accesslists"
	sudo install -m 0755 "$(BINARY)" "$(BIN_DIR)/$(BINARY)"
	sudo sh -c "echo '$(VERSION)' > '$(INSTALL_DIR)/VERSION'"
	@echo "==> Linking global command: $(GLOBAL_BIN_DIR)/$(BINARY)"
	sudo mkdir -p "$(GLOBAL_BIN_DIR)"
	sudo ln -sf "$(BIN_DIR)/$(BINARY)" "$(GLOBAL_BIN_DIR)/$(BINARY)"
	@if [ -f "$(CONFIG_FILE)" ]; then \
		echo "==> Existing configuration preserved: $(CONFIG_FILE)"; \
	else \
		echo "==> Installing default configuration to $(CONFIG_FILE)"; \
		sudo install -Dm644 configs/default.yaml "$(CONFIG_FILE)"; \
	fi
	@if command -v logrotate >/dev/null 2>&1; then \
		echo "==> Installing logrotate policy"; \
		sudo install -Dm644 configs/logrotate.conf /etc/logrotate.d/gonix; \
	else \
		echo "==> logrotate not found; skipping log rotation setup"; \
	fi
	@echo "==> Installation complete. Run: sudo gonix"

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -f $(BINARY)-linux-amd64 $(BINARY)-linux-arm64

.PHONY: cross
cross:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-amd64 $(CMD_PATH)
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 $(CMD_PATH)
