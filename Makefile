VERSION  ?= $(shell git -C . describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X main.version=$(VERSION)
TOOL      = go tool -modfile=tools/$(1)/go.mod $(1)
DIST     := dist

.PHONY: build snapshot install uninstall clean lint actionlint vuln check docker-build docker-run

## build: build for the current platform
build:
	go build -ldflags "$(LDFLAGS)" -o azpim .

## snapshot: build release artifacts locally without publishing (for testing)
snapshot:
	$(call TOOL,goreleaser) release --snapshot --clean

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

## lint: run golangci-lint
lint:
	$(call TOOL,golangci-lint) run ./...

## vuln: scan dependencies for known vulnerabilities
vuln:
	$(call TOOL,govulncheck) ./...

## actionlint: lint GitHub Actions workflows
actionlint:
	$(call TOOL,actionlint)

## check: run all quality and security checks
check: lint actionlint vuln

## docker-build: build the container image (uses Chainguard hardened base images)
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t azpim:$(VERSION) -t azpim:latest .

## docker-run: run azpim in a container, mounting the host az login session
##   Pass subcommand + flags via CMD, e.g.: make docker-run CMD="eligible --scope /"
docker-run:
	docker run --rm -it \
	  -v "$(HOME)/.azure:/home/nonroot/.azure:ro" \
	  azpim:latest $(CMD)

## clean: remove build artefacts
clean:
	rm -rf $(DIST) azpim
