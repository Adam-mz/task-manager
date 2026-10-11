FROM golang:1.26.6-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /out/task-manager ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/task-manager /app/task-manager

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/task-manager"]