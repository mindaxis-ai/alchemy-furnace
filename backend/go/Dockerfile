# ═══════════════════════════════════════════
# Go API 网关 - 炼丹炉门户
#
# 多阶段构建：
#   builder —— go mod cache mount + go build cache，依赖层最大复用
#   runtime —— alpine 最小运行时，非 root 用户
#
# 受限网络传代理：docker build --build-arg HTTPS_PROXY=http://host:port .
# ═══════════════════════════════════════════

# ---- builder ----
FROM golang:1.23-alpine AS builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    GOFLAGS=-mod=readonly

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# 依赖层优先：go.mod/go.sum 未变直接命中缓存
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 源码：改一个 .go 只重 build 这一层
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/main

# ---- runtime ----
FROM alpine:3.20

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY}

RUN apk --no-cache add ca-certificates curl tzdata

# 非 root
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app

COPY --from=builder /out/server /app/server

RUN mkdir -p /app/uploads && chown -R app:app /app
USER app

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD curl -f http://localhost:8080/api/v1/system/health || exit 1

STOPSIGNAL SIGTERM

CMD ["./server"]
