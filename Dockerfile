# 阶段1: 编译
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 先复制依赖文件，利用Docker缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /download_proxy .

# 阶段2: 运行（使用最小镜像）
FROM alpine:3.20

# 安装CA证书（用于向HTTPS目标发起请求）
RUN apk --no-cache add ca-certificates

COPY --from=builder /download_proxy /usr/local/bin/download_proxy

# 默认端口
EXPOSE 8443/tcp
EXPOSE 8443/udp

ENTRYPOINT ["download_proxy"]
CMD ["-addr", ":8443"]
