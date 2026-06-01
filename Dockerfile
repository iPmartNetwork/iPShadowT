# iPShadowT - Multi-stage Docker build
# Build: docker build -t ipshadowt .
# Run:   docker run -v /path/to/config.toml:/etc/ipshadowt/config.toml ipshadowt

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

ENV GOTOOLCHAIN=local
ENV GONOSUMCHECK=*
ENV GOFLAGS=-mod=mod

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download -x 2>&1 | tail -5 || true

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=v2.2.0" \
    -o /ipshadowt ./cmd/ipshadowt/

# Final image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates iptables ip6tables curl

COPY --from=builder /ipshadowt /usr/local/bin/ipshadowt

RUN mkdir -p /etc/ipshadowt

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -sf http://127.0.0.1:9090/health || exit 1

EXPOSE 443/tcp 443/udp 9090/tcp

ENTRYPOINT ["/usr/local/bin/ipshadowt"]
CMD ["-c", "/etc/ipshadowt/config.toml"]
