# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.27-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git

# Cache dependencies separately from application source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the API as a static Linux binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/taskforge-api \
    ./cmd/api

# ---- Runtime stage ----
FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/taskforge-api /app/taskforge-api

USER app

EXPOSE 8080

ENTRYPOINT ["/app/taskforge-api"]
