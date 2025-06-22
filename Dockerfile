FROM golang:1.24.3-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o entropic-server ./cmd/server
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
RUN addgroup -g 1001 entropic && \
    adduser -D -u 1001 -G entropic entropic
WORKDIR /app
COPY --from=builder /app/entropic-server .
COPY --from=builder /app/entropic.example.yaml ./entropic.example.yaml
RUN mkdir -p /app/logs /app/data && \
    chown -R entropic:entropic /app
USER entropic
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD ./entropic-server version || exit 1
EXPOSE 8080
ENV ENTROPIC_SERVER_HOST=0.0.0.0
ENV ENTROPIC_SERVER_PORT=8080
ENV ENTROPIC_LOGGING_LEVEL=info
ENTRYPOINT ["./entropic-server"]
CMD ["--config", "/app/entropic.yaml"]
