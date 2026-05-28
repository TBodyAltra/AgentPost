FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/getpost .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates curl
WORKDIR /app

COPY --from=builder /out/getpost /app/getpost
COPY config.example.yaml /app/config.example.yaml

EXPOSE 8080 2525

ENTRYPOINT ["/app/getpost"]
CMD ["-config", "/app/config.yaml"]
