package main

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// ProxyHandler 代理下载处理器
type ProxyHandler struct {
	// httpClient 用于向目标URL发起请求的HTTP客户端
	httpClient *http.Client
}

// NewProxyHandler 创建新的代理下载处理器
func NewProxyHandler() *ProxyHandler {
	return &ProxyHandler{
		httpClient: &http.Client{
			// 不设置超时，因为大文件下载可能需要很长时间
			// 但设置一个合理的连接超时
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
			},
		},
	}
}

// ServeHTTP 处理代理下载请求
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 仅支持 /proxy 路径
	if r.URL.Path != "/proxy" {
		// 根路径返回简单的使用说明
		if r.URL.Path == "/" {
			h.serveIndex(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	// 获取目标URL
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "缺少 url 参数。使用方式: /proxy?url=https://example.com/file.zip", http.StatusBadRequest)
		return
	}

	// 验证URL格式
	parsedURL, err := url.Parse(targetURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		http.Error(w, "无效的URL，仅支持 http:// 和 https:// 协议", http.StatusBadRequest)
		return
	}

	log.Printf("📥 开始代理下载: %s (客户端: %s, 协议: %s)", targetURL, r.RemoteAddr, r.Proto)

	// 向目标URL发起GET请求
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("创建请求失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 转发客户端的Range头（支持断点续传）
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// 设置合理的User-Agent
	req.Header.Set("User-Agent", "DownloadProxy/1.0")

	// 发起请求
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("❌ 请求目标URL失败: %v", err)
		http.Error(w, fmt.Sprintf("请求目标URL失败: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("目标服务器返回错误: %s", resp.Status), http.StatusBadGateway)
		return
	}

	// 设置响应头
	h.setResponseHeaders(w, resp, parsedURL)

	// 设置状态码（可能是200或206）
	w.WriteHeader(resp.StatusCode)

	// 流式传输响应体到客户端
	startTime := time.Now()
	written, err := io.Copy(w, resp.Body)
	duration := time.Since(startTime)

	if err != nil {
		log.Printf("⚠️ 传输中断: %s (已传输: %s, 耗时: %s, 错误: %v)",
			targetURL, formatBytes(written), duration, err)
		return
	}

	log.Printf("✅ 下载完成: %s (大小: %s, 耗时: %s, 速度: %s/s)",
		targetURL, formatBytes(written), duration, formatBytes(int64(float64(written)/duration.Seconds())))
}

// setResponseHeaders 设置代理响应头
func (h *ProxyHandler) setResponseHeaders(w http.ResponseWriter, resp *http.Response, targetURL *url.URL) {
	// 转发 Content-Type
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	// 转发 Content-Length
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}

	// 转发 Content-Range（断点续传）
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}

	// 转发 Accept-Ranges
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}

	// 设置文件名
	filename := h.extractFilename(resp, targetURL)
	if filename != "" {
		// 使用 RFC 5987 编码支持中文文件名
		w.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
				filename, url.PathEscape(filename)))
	}

	// 允许跨域访问
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// extractFilename 从响应头或URL中提取文件名
func (h *ProxyHandler) extractFilename(resp *http.Response, targetURL *url.URL) string {
	// 优先从 Content-Disposition 头获取
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		_, params, err := mime.ParseMediaType(cd)
		if err == nil {
			if name, ok := params["filename"]; ok {
				return name
			}
		}
	}

	// 从URL路径中提取
	urlPath := targetURL.Path
	if urlPath != "" && urlPath != "/" {
		filename := path.Base(urlPath)
		if filename != "." && filename != "/" {
			return filename
		}
	}

	return ""
}

// serveIndex 返回首页使用说明
func (h *ProxyHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>代理下载服务</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #0c0c1d 0%, #1a1a3e 50%, #0c0c1d 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #e0e0e0;
        }
        .container {
            background: rgba(255,255,255,0.05);
            backdrop-filter: blur(20px);
            border: 1px solid rgba(255,255,255,0.1);
            border-radius: 20px;
            padding: 3rem;
            max-width: 700px;
            width: 90%;
            box-shadow: 0 25px 50px rgba(0,0,0,0.5);
        }
        h1 {
            font-size: 2rem;
            background: linear-gradient(135deg, #60a5fa, #a78bfa);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 0.5rem;
        }
        .badge {
            display: inline-block;
            background: linear-gradient(135deg, #10b981, #059669);
            color: white;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.75rem;
            font-weight: 600;
            margin-bottom: 1.5rem;
        }
        .usage {
            background: rgba(0,0,0,0.3);
            border-radius: 12px;
            padding: 1.5rem;
            margin: 1.5rem 0;
            border: 1px solid rgba(255,255,255,0.05);
        }
        .usage h3 { color: #a78bfa; margin-bottom: 0.75rem; font-size: 0.9rem; text-transform: uppercase; letter-spacing: 1px; }
        code {
            display: block;
            background: rgba(96,165,250,0.1);
            border: 1px solid rgba(96,165,250,0.2);
            border-radius: 8px;
            padding: 1rem;
            font-family: 'Fira Code', 'JetBrains Mono', monospace;
            font-size: 0.85rem;
            color: #60a5fa;
            word-break: break-all;
            margin-top: 0.5rem;
        }
        .protocol { color: #94a3b8; font-size: 0.85rem; margin-top: 1rem; }
        .protocol strong { color: #a78bfa; }
    </style>
</head>
<body>
    <div class="container">
        <h1>⚡ 代理下载服务</h1>
        <span class="badge">HTTP/3 已启用</span>
        <p class="protocol">支持 <strong>HTTP/1.1</strong>、<strong>HTTP/2</strong> 和 <strong>HTTP/3 (QUIC)</strong> 协议</p>
        <div class="usage">
            <h3>📖 使用方式</h3>
            <p>在URL中传入要下载的资源地址：</p>
            <code>` + fmt.Sprintf("https://%s/proxy?url=https://example.com/file.zip", r.Host) + `</code>
        </div>
        <div class="usage">
            <h3>✨ 功能特性</h3>
            <p>• 流式传输，不占用服务器内存</p>
            <p>• 支持断点续传 (Range请求)</p>
            <p>• 自动提取文件名</p>
            <p>• 支持中文文件名</p>
        </div>
    </div>
</body>
</html>`
	fmt.Fprint(w, html)
}

// formatBytes 将字节数格式化为人类可读的字符串
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return strconv.FormatFloat(float64(bytes)/float64(GB), 'f', 2, 64) + " GB"
	case bytes >= MB:
		return strconv.FormatFloat(float64(bytes)/float64(MB), 'f', 2, 64) + " MB"
	case bytes >= KB:
		return strconv.FormatFloat(float64(bytes)/float64(KB), 'f', 2, 64) + " KB"
	default:
		return strconv.FormatInt(bytes, 10) + " B"
	}
}

// 以下函数未使用但保留用于未来可能的日志增强
func truncateURL(rawURL string, maxLen int) string {
	if len(rawURL) <= maxLen {
		return rawURL
	}
	return rawURL[:maxLen] + "..."
}

// sanitizeFilename 清理文件名中的非法字符
func sanitizeFilename(name string) string {
	// 替换Windows和Linux文件系统中的非法字符
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
