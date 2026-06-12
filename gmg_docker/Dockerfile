# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG APP=gateway
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:3.24 AS runtime

ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata

RUN adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/app /usr/local/bin/app
COPY configs /app/configs

USER appuser

EXPOSE 9000 9001 8081 8082

ENTRYPOINT ["/usr/local/bin/app"]
