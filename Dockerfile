# Multi-stage Dockerfile for Entropic Server

# Build stage
FROM golang:1.24.3-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o entropic-server ./cmd/server

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 entropic && \
    adduser -D -u 1001 -G entropic entropic

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/entropic-server .

# Copy configuration template
COPY --from=builder /app/entropic.example.yaml ./entropic.example.yaml

# Create directories
RUN mkdir -p /app/logs /app/data && \
    chown -R entropic:entropic /app

# Switch to non-root user
USER entropic

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD ./entropic-server version || exit 1

# Expose port
EXPOSE 8080

# Set environment variables
ENV ENTROPIC_SERVER_HOST=0.0.0.0
ENV ENTROPIC_SERVER_PORT=8080
ENV ENTROPIC_LOGGING_LEVEL=info

# Default command
ENTRYPOINT ["./entropic-server"]
CMD ["--config", "/app/entropic.yaml"]