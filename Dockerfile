# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS builder

ARG BUILD_VERSION=dev
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPRIVATE=github.com/BCBP-SOLUTIONS-FZC-LLC \
    GONOSUMDB=github.com/BCBP-SOLUTIONS-FZC-LLC/*

WORKDIR /build

RUN apk add --no-cache git ca-certificates

RUN --mount=type=secret,id=go_private_token \
    git config --global credential.helper store && \
    echo "https://x-access-token:$(cat /run/secrets/go_private_token)@github.com" > ~/.git-credentials && \
    chmod 600 ~/.git-credentials

COPY go.mod go.sum* ./
RUN go mod download && go mod verify

COPY . .

RUN go build \
    -ldflags="-w -s -X main.version=${BUILD_VERSION}" \
    -trimpath \
    -o /build/bin/event-consumer \
    ./cmd/event-consumer
RUN go build \
    -ldflags="-w -s -X main.version=${BUILD_VERSION}" \
    -trimpath \
    -o /build/bin/mock-downstream \
    ./cmd/mock-downstream

# gcr.io/distroless/static-debian12: no shell, no libc, no package manager.
# Runs as nonroot (uid 65532) by default.
FROM gcr.io/distroless/static-debian12:nonroot AS event-consumer

COPY --from=builder /build/bin/event-consumer /event-consumer

EXPOSE 8082

ENTRYPOINT ["/event-consumer"]

# mock-downstream is docker-compose-only — never pushed to a real registry.
# Alpine (not distroless) so docker-compose's wget-based healthcheck works.
FROM alpine:3.20 AS mock-downstream

COPY --from=builder /build/bin/mock-downstream /mock-downstream
USER nobody

EXPOSE 8090

ENTRYPOINT ["/mock-downstream"]
