# Build/push the web-deps target in Dockerfile.deps with this tag first.
FROM stgharbor.liangdawang.com:1180/library/new-api-web-deps:amd-musl AS builder
WORKDIR /build/web
COPY ./web ./
RUN bun --bun run build

FROM stgharbor.liangdawang.com:1180/library/new-api-go-deps:amd AS builder2
ENV GOPROXY=off
WORKDIR /build
COPY . .
COPY --from=builder /build/web/dist ./web/dist
RUN go build -mod=readonly -ldflags "-s -w" -o new-api


FROM stgharbor.liangdawang.com:1180/library/alpine:3.20
COPY --from=builder2 /build/new-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
