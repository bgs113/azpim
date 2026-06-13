# ── build stage ──────────────────────────────────────────────────────────────
FROM cgr.dev/chainguard/go:latest AS builder

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o azpim .

# ── runtime stage ─────────────────────────────────────────────────────────────
# cgr.dev/chainguard/static: distroless, includes ca-certificates, runs as nonroot (uid 65532)
FROM cgr.dev/chainguard/static:latest

COPY --from=builder /src/azpim /usr/local/bin/azpim

ENTRYPOINT ["/usr/local/bin/azpim"]
