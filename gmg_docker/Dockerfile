# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.24
ARG APP=gateway
ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
ARG GOPROXY=https://goproxy.cn,direct

FROM golang:${GO_VERSION}-alpine AS builder

ARG APP
ARG GOPROXY

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG APP=gateway
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${APP}

FROM alpine:${ALPINE_VERSION} AS runtime

FROM alpine:3.20 AS runtime
ARG ALPINE_MIRROR

RUN apk add --no-cache ca-certificates tzdata \
RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

WORKDIR /app
