# Multi-stage build — builder stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/indexer ./cmd/indexer

# Runtime stage — minimal Alpine image with non-root user
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl && \
    adduser -D -u 1000 appuser
WORKDIR /app
COPY --from=builder /app/indexer .
COPY --chown=appuser:appuser config.example.yml ./config.example.yml
USER appuser
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8090/health || exit 1
CMD ["./indexer"]
