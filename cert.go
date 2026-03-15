package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// generateSelfSignedCert 生成自签名TLS证书，用于开发和测试环境
func generateSelfSignedCert() (tls.Certificate, error) {
	// 生成ECDSA私钥（P-256曲线，兼具安全性和性能）
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成私钥失败: %w", err)
	}

	// 生成序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成序列号失败: %w", err)
	}

	// 证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Download Proxy Dev"},
			CommonName:   "localhost",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // 有效期1年

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		// 添加本地地址到SAN
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:    []string{"localhost"},
	}

	// 创建自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("创建证书失败: %w", err)
	}

	// 编码为PEM格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("编码私钥失败: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// 解析为tls.Certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("解析证书对失败: %w", err)
	}

	fmt.Println("✅ 已生成自签名TLS证书（仅用于开发环境）")
	return tlsCert, nil
}

// loadOrGenerateCert 加载指定的证书文件，如果未指定则自动生成自签名证书
func loadOrGenerateCert(certFile, keyFile string) (tls.Certificate, error) {
	// 如果指定了证书文件路径，尝试加载
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("加载证书失败: %w", err)
		}

		// 检查证书文件是否存在
		if _, err := os.Stat(certFile); os.IsNotExist(err) {
			return tls.Certificate{}, fmt.Errorf("证书文件不存在: %s", certFile)
		}
		if _, err := os.Stat(keyFile); os.IsNotExist(err) {
			return tls.Certificate{}, fmt.Errorf("私钥文件不存在: %s", keyFile)
		}

		fmt.Printf("✅ 已加载TLS证书: %s\n", certFile)
		return cert, nil
	}

	// 未指定证书，自动生成自签名证书
	return generateSelfSignedCert()
}
