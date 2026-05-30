# iPShadowT - Multi-stage Docker build
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.Version=docker -X main.BuildTime=$(date -u +%Y%m%d%H%M%S)" \
    -o /ipshadowt ./cmd/ipshadowt/

# Final image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates iptables ip6tables

COPY --from=builder /ipshadowt /usr/local/bin/ipshadowt

RUN mkdir -p /etc/ipshadowt

EXPOSE 443

ENTRYPOINT ["/usr/local/bin/ipshadowt"]
CMD ["-c", "/etc/ipshadowt/config.toml"]
