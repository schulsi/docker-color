BIN=$(CURDIR)/bin
GO=go
APP=docker-color-output
WRAPPER=dcolor

.PHONY: build
build:
	@mkdir -p $(BIN)
	@CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP) ./cmd/docker-color-output
	@CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER) ./cmd/dcolor

.PHONY: publish
publish: clean
	@mkdir -p $(BIN)
	# Mac
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP)-darwin-amd64 ./cmd/docker-color-output
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP)-darwin-arm64 ./cmd/docker-color-output
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER)-darwin-amd64 ./cmd/dcolor
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER)-darwin-arm64 ./cmd/dcolor
	# Linux
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP)-linux-amd64 ./cmd/docker-color-output
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP)-linux-arm64 ./cmd/docker-color-output
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER)-linux-amd64 ./cmd/dcolor
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER)-linux-arm64 ./cmd/dcolor
	# Windows
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(APP)-windows-amd64.exe ./cmd/docker-color-output
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN)/$(WRAPPER)-windows-amd64.exe ./cmd/dcolor
	# Cleanup
	mv ./bin/* ~/Downloads

.PHONY: test
test:
	@go test ./...

.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

.PHONY: clean
clean:
	@rm -f $(BIN)/$(APP)* $(BIN)/$(WRAPPER)*

.PHONY: lint
lint:
	@golangci-lint run -v --fix
