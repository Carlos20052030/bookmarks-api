# syntax=docker/dockerfile:1

# ---------- Stage 1: build ----------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Dependencies are copied first so this layer is cached when source changes.
COPY go.mod go.sum ./
RUN go mod download

# Only the packages the build actually needs.
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/

# Static binary: CGO off, paths trimmed, symbols stripped.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---------- Stage 2: runtime ----------
FROM alpine:3.21

# CA certificates for outbound HTTPS, tzdata so timestamps are correct.
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S app && adduser -S -D -H -G app app

USER app
WORKDIR /app

COPY --from=builder /out/api /app/api

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/api"]