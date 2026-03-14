VERSION  ?= $(shell git -C . describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)
DIST     := dist

.PHONY: build release install uninstall clean tools lint vuln check

## build: build for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o azpim .

## release: cross-compile binaries for macOS (arm64 + amd64), Linux (amd64), and Windows (amd64)
release: clean
	mkdir -p $(DIST)

	GOOS=darwin  GOARCH=arm64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/azpim-$(VERSION)-darwin-arm64   .
	GOOS=darwin  GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/azpim-$(VERSION)-darwin-amd64   .
	GOOS=windows GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/azpim-$(VERSION)-windows-amd64.exe .
	GOOS=linux   GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/azpim-$(VERSION)-linux-amd64    .

	cd $(DIST) && \
	  zip azpim-$(VERSION)-darwin-arm64.zip   azpim-$(VERSION)-darwin-arm64   && \
	  zip azpim-$(VERSION)-darwin-amd64.zip   azpim-$(VERSION)-darwin-amd64   && \
	  zip azpim-$(VERSION)-windows-amd64.zip  azpim-$(VERSION)-windows-amd64.exe && \
	  zip azpim-$(VERSION)-linux-amd64.zip    azpim-$(VERSION)-linux-amd64

	@echo "Release $(VERSION) artifacts in $(DIST)/"
	@ls -lh $(DIST)/*.zip

## install: install the current-platform binary to ~/.local/bin (no sudo needed)
install: build
	mkdir -p $(HOME)/.local/bin
	install -m 755 azpim $(HOME)/.local/bin/azpim
	@echo "Installed to $(HOME)/.local/bin/azpim"
	@echo "Make sure ~/.local/bin is on your PATH:"
	@echo "  echo 'export PATH=\"\$$HOME/.local/bin:\$$PATH\"' >> ~/.zshrc"

## uninstall: remove the installed binary from ~/.local/bin
uninstall:
	rm -f $(HOME)/.local/bin/azpim
	@echo "Removed $(HOME)/.local/bin/azpim"

## tools: install go-based quality tools (golangci-lint managed via mise)
tools:
	go install golang.org/x/vuln/cmd/govulncheck@latest

## lint: run golangci-lint
lint:
	mise exec -- golangci-lint run ./...

## vuln: scan dependencies for known vulnerabilities
vuln:
	govulncheck ./...

## check: run all quality and security checks
check: lint vuln

## clean: remove build artefacts
clean:
	rm -rf $(DIST) azpim
