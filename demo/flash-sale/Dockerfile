# --- Stage 1: Build Demo Application ---
FROM golang:1.27-alpine AS builder

WORKDIR /app

# Copy source code and embedded templates
COPY . .

# Build lightweight static Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o demo-app main.go

# --- Stage 2: Minimal Runtime ---
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/demo-app /app/demo-app

EXPOSE 3000

ENV PORT=3000

ENTRYPOINT ["/app/demo-app"]
