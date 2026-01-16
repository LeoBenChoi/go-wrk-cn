package loader

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/LeoBenChoi/go-wrk-cn/util"
	"golang.org/x/net/http2"
)

func client(disableCompression, disableKeepAlive, skipVerify bool, timeoutms int, allowRedirects bool, clientCert, clientKey, caCert string, usehttp2 bool) (*http.Client, error) {

	client := &http.Client{}
	// 覆盖默认参数
	client.Transport = &http.Transport{
		DisableCompression:    disableCompression,
		DisableKeepAlives:     disableKeepAlive,
		ResponseHeaderTimeout: time.Millisecond * time.Duration(timeoutms),
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: skipVerify},
	}

	if !allowRedirects {
		// 返回错误以阻止重定向发生。
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return util.NewRedirectError("不允许重定向")
		}
	}

	if clientCert == "" && clientKey == "" && caCert == "" {
		// 如果未提供任何客户端证书或 CA 证书，则直接返回默认客户端。
		return client, nil
	}

	if clientCert == "" {
		return nil, fmt.Errorf("客户端证书不能为空")
	}

	if clientKey == "" {
		return nil, fmt.Errorf("客户端密钥不能为空")
	}
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("无法加载证书，尝试加载 %v 和 %v 时出错: %v", clientCert, clientKey, err)
	}

	// 加载 CA 证书
	clientCACert, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("无法打开 CA 证书: %v", err)
	}

	clientCertPool := x509.NewCertPool()
	clientCertPool.AppendCertsFromPEM(clientCACert)

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            clientCertPool,
		InsecureSkipVerify: skipVerify,
	}

	t := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	if usehttp2 {
		http2.ConfigureTransport(t)
	}
	client.Transport = t
	return client, nil
}
