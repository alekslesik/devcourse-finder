# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then export SSL_CERT_FILE=/run/secrets/proxy_ca; fi; \
    go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/devcourse-finder .

FROM alpine:3.22

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S app && adduser -S -G app app

COPY --from=build /out/devcourse-finder /usr/local/bin/devcourse-finder

USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/devcourse-finder"]
