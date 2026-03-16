VERSION  ?= $(shell git -C . describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)
DIST     := dist

.PHONY: build release snapshot install uninstall clean tools lint vuln check docker-build docker-run

## build: build for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o azpim .

## release: build and publish a release via GoReleaser (requires a tagged commit)
release:
	goreleaser release --clean

## snapshot: build release artifacts locally without publishing (for testing)
snapshot:
	goreleaser release --snapshot --clean

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
	golangci-lint run ./...

## vuln: scan dependencies for known vulnerabilities
vuln:
	govulncheck ./...

## check: run all quality and security checks
check: lint vuln

## docker-build: build the container image (uses Chainguard hardened base images)
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t azpim:$(VERSION) -t azpim:latest .

## docker-run: run azpim in a container, mounting host az login session and cache
##   Pass subcommand + flags via CMD, e.g.: make docker-run CMD="eligible --scope /"
docker-run:
	docker run --rm -it \
	  -v "$(HOME)/.azure:/home/nonroot/.azure:ro" \
	  -v "$(HOME)/.cache/azpim:/home/nonroot/.cache/azpim" \
	  azpim:latest $(CMD)

## clean: remove build artefacts
clean:
	rm -rf $(DIST) azpim
