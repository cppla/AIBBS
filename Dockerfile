# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21

FROM golang:${GO_VERSION}-alpine AS build
RUN apk add --no-cache ca-certificates git tzdata
WORKDIR /src

# Cache deps first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o aibbs .

# --- Runtime image ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl \
 && addgroup -S app && adduser -S -G app app

WORKDIR /app

# Binary
COPY --from=build /src/aibbs /app/aibbs

# Runtime assets (can be overridden by volume mounts)
COPY static /app/static
COPY config /app/config
COPY scripts /app/scripts

# Ensure uploads directory exists and is writable
RUN mkdir -p /app/static/uploads \
 && chown -R app:app /app

VOLUME ["/app/static/uploads"]

EXPOSE 8080
USER app
ENV TZ=UTC

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -sf http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/aibbs"]
