# 代理下载服务 (Download Proxy)

基于 Go 语言的 HTTP/3 代理下载服务。通过 URL 参数指定目标资源地址，服务会流式转发资源到客户端，同时支持 HTTP/1.1、HTTP/2 和 HTTP/3 (QUIC) 协议。

## 功能特性

- ⚡ **HTTP/3 支持** — 基于 QUIC 协议，低延迟、抗丢包
- 🔄 **流式传输** — 不缓存整个文件到内存，支持大文件下载
- 📥 **断点续传** — 支持 Range 请求头转发
- 📝 **智能文件名** — 自动从响应头或 URL 提取文件名，支持中文
- 🔒 **自签名证书** — 开发模式下自动生成，无需手动配置
- 🌐 **双协议栈** — TCP (HTTP/1.1+HTTP/2) 和 QUIC (HTTP/3) 同时监听

## 编译

```bash
# 确保已安装 Go 1.21+
go build -o download_proxy .

# Windows
go build -o download_proxy.exe .

# 交叉编译 Linux
GOOS=linux GOARCH=amd64 go build -o download_proxy .
```

## 使用方式

### 基本启动（自签名证书）

```bash
./download_proxy
```

服务将在 `https://localhost:8443` 启动，自动生成自签名证书。

### 自定义端口

```bash
./download_proxy -addr :9443
```

### 使用正式证书（生产环境）

```bash
./download_proxy -addr :443 -cert /path/to/cert.pem -key /path/to/key.pem
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-addr` | `:8443` | 监听地址和端口 |
| `-cert` | 空 | TLS 证书文件路径 |
| `-key` | 空 | TLS 私钥文件路径 |

## 下载接口

### 请求格式

```
https://your-server/proxy?url=<目标资源URL>
```

### 示例

```bash
# 使用 curl 下载（忽略自签名证书警告）
curl -kL "https://localhost:8443/proxy?url=https://dl.google.com/go/go1.23.0.linux-amd64.tar.gz" -o go.tar.gz

# 使用 curl 测试 HTTP/3（需要 curl 支持 HTTP/3）
curl -k --http3 "https://localhost:8443/proxy?url=https://example.com/file.zip" -o file.zip

# 使用 wget
wget --no-check-certificate "https://localhost:8443/proxy?url=https://example.com/file.zip"
```

### 浏览器使用

直接在浏览器地址栏输入：

```
https://localhost:8443/proxy?url=https://example.com/file.zip
```

> **注意**：使用自签名证书时，浏览器会显示安全警告，需要手动确认继续访问。

## Docker

### 构建镜像

```bash
docker build -t download-proxy .
```

### 运行容器

```bash
# 自签名证书模式
docker run -d --name download-proxy -p 8443:8443/tcp -p 8443:8443/udp download-proxy

# 使用正式证书
docker run -d --name download-proxy \
  -p 443:443/tcp -p 443:443/udp \
  -v /path/to/cert.pem:/certs/cert.pem:ro \
  -v /path/to/key.pem:/certs/key.pem:ro \
  download-proxy -addr :443 -cert /certs/cert.pem -key /certs/key.pem
```

> **注意**：HTTP/3使用UDP协议，需同时映射TCP和UDP端口。

### Docker Compose

```bash
docker compose up -d
```

### 优化 QUIC 性能（可选）

quic-go 需要较大的 UDP 缓冲区以获得最佳性能。`net.core.rmem_max` 是全局内核参数，**无法在容器内设置**，需要在**宿主机**上配置：

```bash
# 临时生效
sysctl -w net.core.rmem_max=7500000
sysctl -w net.core.wmem_max=7500000

# 永久生效
echo "net.core.rmem_max=7500000" >> /etc/sysctl.conf
echo "net.core.wmem_max=7500000" >> /etc/sysctl.conf
sysctl -p
```

> 不配置也不影响功能，仅在高带宽场景下性能略低。

## 部署建议

### Linux 服务化 (systemd)

创建 `/etc/systemd/system/download-proxy.service`：

```ini
[Unit]
Description=Download Proxy Service
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/download_proxy -addr :443 -cert /etc/ssl/cert.pem -key /etc/ssl/key.pem
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now download-proxy
```

### 防火墙配置

HTTP/3 使用 UDP 协议，需要同时开放 TCP 和 UDP 端口：

```bash
# iptables
sudo iptables -A INPUT -p tcp --dport 8443 -j ACCEPT
sudo iptables -A INPUT -p udp --dport 8443 -j ACCEPT

# firewalld
sudo firewall-cmd --add-port=8443/tcp --permanent
sudo firewall-cmd --add-port=8443/udp --permanent
sudo firewall-cmd --reload
```

## 项目结构

```
Download_proxy/
├── main.go          # 程序入口，启动HTTP/1.1+HTTP/2和HTTP/3双服务器
├── handler.go       # 代理下载处理器，流式转发逻辑
├── cert.go          # TLS证书管理（自签名/加载外部证书）
├── Dockerfile       # Docker多阶段构建
├── .dockerignore    # Docker构建排除列表
├── go.mod           # Go模块定义
├── go.sum           # 依赖校验
└── docs/
    └── usage.md     # 本文档
```
