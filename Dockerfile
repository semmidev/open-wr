# --- Stage 1: Builder ---
FROM golang:1.27-alpine AS builder7
# Build-time arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Install system dependencies (CA certificates & timezones)
RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /build

# Cache Go modules layer
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build static binary
# - CGO_ENABLED=0 for pure static binary without libc dependency
# - ldflags "-w -s" to strip debug info & symbol tables (smaller size)
# - ldflags "-X" to inject build-time variables into main package
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s \
    -X main.version=${VERSION} \
    -X main.commit=${COMMIT} \
    -X main.buildTime=${BUILD_TIME}" \
    -o owr ./cmd/server

# --- Stage 2: Final (Production) ---
# Distroless static non-root image: minimal attack surface, non-root user (UID 65532)
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy timezone data & CA certs from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy binary and default config from builder
COPY --from=builder /build/owr /app/owr
COPY --from=builder /build/config.example.yaml /app/config.example.yaml

# Expose port
EXPOSE 8080

# Default entrypoint & flags
ENTRYPOINT ["/app/owr"]
CMD ["--config", "config.example.yaml"]
