BINARY      := nginx-manager
CMD_PATH    := ./cmd/nginx-manager
VERSION     := $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X github.com/shiva/nginx-manager/internal/tui.Version=$(VERSION)

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
	sudo ./scripts/install.sh

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -f $(BINARY)-linux-amd64 $(BINARY)-linux-arm64

.PHONY: cross
cross:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-amd64 $(CMD_PATH)
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 $(CMD_PATH)
