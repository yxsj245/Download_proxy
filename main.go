package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	// 命令行参数
	addr := flag.String("addr", ":8443", "监听地址和端口")
	certFile := flag.String("cert", "", "TLS证书文件路径（留空则自动生成自签名证书）")
	keyFile := flag.String("key", "", "TLS私钥文件路径（留空则自动生成自签名证书）")
	flag.Parse()

	fmt.Println("🚀 代理下载服务 (Download Proxy)")
	fmt.Println("================================")

	// 加载或生成TLS证书
	tlsCert, err := loadOrGenerateCert(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("❌ TLS证书初始化失败: %v", err)
	}

	// TLS配置（用于TCP服务器：HTTP/1.1 + HTTP/2）
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2", "http/1.1"},
	}

	// HTTP/3 的 TLS 配置（需要单独配置，ALPN 不同）
	http3TLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13, // HTTP/3 要求 TLS 1.3
	}

	// 创建代理处理器
	handler := NewProxyHandler()

	// 创建HTTP多路复用器
	mux := http.NewServeMux()
	mux.Handle("/", handler)

	// 创建HTTP/3服务器（QUIC/UDP）
	http3Server := &http3.Server{
		Addr:      *addr,
		Handler:   mux,
		TLSConfig: http3TLSConfig,
	}

	// 创建HTTP/1.1 + HTTP/2 服务器（TCP），添加Alt-Svc头
	tcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 添加 Alt-Svc 头，告知客户端可以使用HTTP/3
		w.Header().Set("Alt-Svc", fmt.Sprintf(`h3="%s"; ma=86400`, *addr))
		mux.ServeHTTP(w, r)
	})

	tcpServer := &http.Server{
		Addr:              *addr,
		Handler:           tcpHandler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动HTTP/3服务器（QUIC/UDP）
	go func() {
		fmt.Printf("🌐 HTTP/3 (QUIC/UDP) 服务已启动: https://localhost%s\n", *addr)
		if err := http3Server.ListenAndServe(); err != nil {
			log.Printf("⚠️ HTTP/3服务错误: %v", err)
		}
	}()

	// 启动HTTP/1.1 + HTTP/2服务器（TCP + TLS）
	go func() {
		// 创建TCP监听器
		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			log.Fatalf("❌ TCP监听失败: %v", err)
		}

		fmt.Printf("🌐 HTTP/1.1+HTTP/2 (TCP) 服务已启动: https://localhost%s\n", *addr)
		// 使用 ServeTLS 而非 Serve，传入空字符串让它使用 TLSConfig 中的证书
		// ServeTLS 会自动配置 HTTP/2 支持
		if err := tcpServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ TCP服务器错误: %v", err)
		}
	}()

	fmt.Println("================================")
	fmt.Printf("📖 使用方式: https://localhost%s/proxy?url=https://example.com/file.zip\n", *addr)
	fmt.Println("按 Ctrl+C 停止服务...")

	// 等待退出信号
	select {
	case <-sigChan:
		fmt.Println("\n🛑 正在关闭服务...")
	case <-ctx.Done():
	}

	// 优雅关闭
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := tcpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️ TCP服务器关闭错误: %v", err)
	}
	if err := http3Server.Close(); err != nil {
		log.Printf("⚠️ HTTP/3服务器关闭错误: %v", err)
	}

	fmt.Println("👋 服务已停止")
}
