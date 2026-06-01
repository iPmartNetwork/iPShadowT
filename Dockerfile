# iPShadowT - Multi-stage Docker build
# Build: docker build -t ipshadowt .
# Run:   docker run -v /path/to/config.toml:/etc/ipshadowt/config.toml ipshadowt

FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.Version=v2.2.0 -X main.BuildTime=$(date -u +%Y%m%d%H%M%S)" \
    -o /ipshadowt ./cmd/ipshadowt/

# Final image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates iptables ip6tables curl

COPY --from=builder /ipshadowt /usr/local/bin/ipshadowt

RUN mkdir -p /etc/ipshadowt

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://127.0.0.1:9090/health || exit 1

EXPOSE 443/tcp 443/udp 9090/tcp

ENTRYPOINT ["/usr/local/bin/ipshadowt"]
CMD ["-c", "/etc/ipshadowt/config.toml"]
