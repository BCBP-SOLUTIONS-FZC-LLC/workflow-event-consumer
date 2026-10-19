# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine@sha256:c95332c2af86b6d89b91bd0500f4b9529ccbd090a0d1855c6d1ceaa142ae8615 AS builder

ARG BUILD_VERSION=dev
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=${TARGETARCH} \
    GOPRIVATE=github.com/BCBP-SOLUTIONS-FZC-LLC \
    GONOSUMDB=github.com/BCBP-SOLUTIONS-FZC-LLC/*

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN --mount=type=secret,id=go_private_token \
    trap 'rm -f ~/.git-credentials ~/.gitconfig' EXIT && \
    git config --global credential.helper store && \
    echo "https://x-access-token:$(cat /run/secrets/go_private_token)@github.com" > ~/.git-credentials && \
    chmod 600 ~/.git-credentials && \
    go mod download && go mod verify

COPY . .

RUN go build \
    -ldflags="-w -s -X main.version=${BUILD_VERSION}" \
    -trimpath \
    -buildvcs=false \
    -o /build/bin/event-consumer \
    ./cmd/event-consumer
RUN go build \
    -ldflags="-w -s -X main.version=${BUILD_VERSION}" \
    -trimpath \
    -buildvcs=false \
    -o /build/bin/mock-downstream \
    ./cmd/mock-downstream

# gcr.io/distroless/static-debian12: no shell, no libc, no package manager.
# Runs as nonroot (uid 65532) by default.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS event-consumer
ARG BUILD_VERSION=dev
LABEL org.opencontainers.image.title="workflow-event-consumer" \
      org.opencontainers.image.source="https://github.com/BCBP-SOLUTIONS-FZC-LLC/workflow-event-consumer" \
      org.opencontainers.image.vendor="BCBP Solutions FZC LLC" \
      org.opencontainers.image.version="${BUILD_VERSION}"

COPY --from=builder /build/bin/event-consumer /event-consumer

EXPOSE 8082

ENTRYPOINT ["/event-consumer"]

# mock-downstream is docker-compose-only — never pushed to a real registry.
# Alpine (not distroless) so docker-compose's wget-based healthcheck works.
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc AS mock-downstream

COPY --from=builder /build/bin/mock-downstream /mock-downstream
USER nobody

EXPOSE 8090

ENTRYPOINT ["/mock-downstream"]
