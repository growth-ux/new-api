FROM oven/bun:1.4.0-alpine AS web-builder
WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun --bun run build

FROM golang:1.26.1-alpine AS go-builder
ENV CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local
WORKDIR /build
COPY go.mod go.sum ./
COPY relaykit/go.mod relaykit/go.sum ./relaykit/
RUN go mod download
COPY . .
COPY --from=web-builder /build/web/dist ./web/dist
RUN go build -mod=readonly -ldflags "-s -w" -o /new-api


FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-builder /new-api /new-api
WORKDIR /data
EXPOSE 3000
ENTRYPOINT ["/new-api"]
