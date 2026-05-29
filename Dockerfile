FROM golang:1.25-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    HTTP_PROXY= \
    HTTPS_PROXY= \
    http_proxy= \
    https_proxy=

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/agentpost ./cmd/agentpost

FROM alpine:3.22

RUN apk add --no-cache ca-certificates curl
WORKDIR /app

COPY --from=builder /out/agentpost /app/agentpost
COPY config.example.yaml /app/config.example.yaml

EXPOSE 8080 2525

ENTRYPOINT ["/app/agentpost"]
CMD ["-config", "/app/config.yaml"]
