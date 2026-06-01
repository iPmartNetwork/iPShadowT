# iPShadowT - Multi-stage Docker build
# Build: docker build -t ipshadowt .
# Run:   docker run -v /path/to/config.toml:/etc/ipshadowt/config.toml ipshadowt

FROM golang:1.24 AS builder

ARG VERSION=2.2.1

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
ENV GOOS=linux

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=v${VERSION}" \
    -o /ipshadowt ./cmd/ipshadowt/

# Final image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates iptables curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /ipshadowt /usr/local/bin/ipshadowt

RUN mkdir -p /etc/ipshadowt

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://127.0.0.1:9090/health || exit 1

EXPOSE 443/tcp 443/udp 9090/tcp

ENTRYPOINT ["/usr/local/bin/ipshadowt"]
CMD ["-c", "/etc/ipshadowt/config.toml"]
