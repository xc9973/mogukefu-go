# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build bot binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bot ./cmd/bot

# Build vectorize binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o vectorize ./cmd/vectorize

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/bot /app/bot
COPY --from=builder /build/vectorize /app/vectorize

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Default environment
ENV TZ=Asia/Shanghai

# Expose no ports (bot uses polling)

# Default command
ENTRYPOINT ["/app/bot"]
CMD ["--config", "/app/config.yaml", "--db", "/app/knowledge.db"]
