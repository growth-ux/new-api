FROM docker.m.daocloud.io/oven/bun:1.4.0-alpine AS web-builder
# npm 依赖走国内源（bun.lock 未硬编码 registry，此变量能完全生效）
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun --bun run build

FROM docker.m.daocloud.io/library/golang:1.26.1-alpine AS go-builder
ENV CGO_ENABLED=0 GOWORK=off GOTOOLCHAIN=local
# Go 模块走国内代理（默认 proxy.golang.org 在国内连不上），direct 兜底
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build
COPY go.mod go.sum ./
COPY relaykit/go.mod relaykit/go.sum ./relaykit/
RUN go mod download
COPY . .
COPY --from=web-builder /build/web/dist ./web/dist
RUN go build -mod=readonly -ldflags "-s -w" -o /new-api


FROM docker.m.daocloud.io/library/alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-builder /new-api /new-api
WORKDIR /data
EXPOSE 3000
ENTRYPOINT ["/new-api"]
