package loader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	histo "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/LeoBenChoi/go-wrk-cn/util"
)

const (
	USER_AGENT = "go-wrk"
)

type LoadCfg struct {
	duration           int // seconds
	goroutines         int
	testUrl            string
	reqBody            string
	method             string
	host               string
	header             map[string]string
	statsAggregator    chan *RequesterStats
	timeoutms          int
	allowRedirects     bool
	disableCompression bool
	disableKeepAlive   bool
	skipVerify         bool
	interrupted        int32
	clientCert         string
	clientKey          string
	caCert             string
	http2              bool
}

// RequesterStats 用于收集聚合统计信息
type RequesterStats struct {
	TotRespSize int64
	TotDuration time.Duration
	NumRequests int
	NumErrs     int
	ErrMap      map[string]int
	Histogram   *histo.Histogram
}

// NewLoadCfg 创建一个新的负载配置
func NewLoadCfg(duration int, // 测试持续时间（秒）
	goroutines int,
	testUrl string,
	reqBody string,
	method string,
	host string,
	header map[string]string,
	statsAggregator chan *RequesterStats,
	timeoutms int,
	allowRedirects bool,
	disableCompression bool,
	disableKeepAlive bool,
	skipVerify bool,
	clientCert string,
	clientKey string,
	caCert string,
	http2 bool) (rt *LoadCfg) {
	rt = &LoadCfg{duration, goroutines, testUrl, reqBody, method, host, header, statsAggregator, timeoutms,
		allowRedirects, disableCompression, disableKeepAlive, skipVerify, 0, clientCert, clientKey, caCert, http2}
	return
}

// escapeUrlStr 转义 URL 字符串
func escapeUrlStr(in string) string {
	qm := strings.Index(in, "?")
	if qm != -1 {
		qry := in[qm+1:]
		qrys := strings.Split(qry, "&")
		var query string = ""
		var qEscaped string = ""
		var first bool = true
		for _, q := range qrys {
			qSplit := strings.Split(q, "=")
			if len(qSplit) == 2 {
				qEscaped = qSplit[0] + "=" + url.QueryEscape(qSplit[1])
			} else {
				qEscaped = qSplit[0]
			}
			if first {
				first = false
			} else {
				query += "&"
			}
			query += qEscaped

		}
		return in[:qm] + "?" + query
	} else {
		return in
	}
}

// DoRequest 单个请求实现。返回响应大小和持续时间
// 错误时返回 -1
func DoRequest(httpClient *http.Client, header map[string]string, method, host, loadUrl, reqBody string) (respSize int, duration time.Duration, err error) {
	respSize = -1
	duration = -1

	loadUrl = escapeUrlStr(loadUrl)

	var buf io.Reader
	if len(reqBody) > 0 {
		buf = bytes.NewBufferString(reqBody)
	}

	req, err := http.NewRequest(method, loadUrl, buf)
	if err != nil {
		return 0, 0, err
	}

	for hk, hv := range header {
		req.Header.Add(hk, hv)
	}

	req.Header.Add("User-Agent", USER_AGENT)
	if host != "" {
		req.Host = host
	}
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		// 当重定向被阻止时，返回一个 url.Error。这会导致区分无效 URL 和重定向错误的问题。
		_, ok := err.(*url.Error)
		if !ok {
			return 0, 0, err
		}
		return 0, 0, err
	}
	if resp == nil {
		return 0, 0, errors.New("empty response")
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	if resp.StatusCode/100 == 2 { // 将所有 2XX 状态码视为成功
		duration = time.Since(start)
		respSize = len(body) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else {
		return 0, 0, errors.New(fmt.Sprint("received status code ", resp.StatusCode))
	}

	return
}

// unwrap 解包错误
func unwrap(err error) error {
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return err
}

// RunSingleLoadSession 一个 go 函数，用于重复请求并聚合统计信息，直到满足要求
// 完成后，使用 statsAggregator 通道发送结果
func (cfg *LoadCfg) RunSingleLoadSession() {
	stats := &RequesterStats{ErrMap: make(map[string]int), Histogram: histo.New(1, int64(cfg.duration*1000000), 4)}
	start := time.Now()

	httpClient, err := client(cfg.disableCompression, cfg.disableKeepAlive, cfg.skipVerify,
		cfg.timeoutms, cfg.allowRedirects, cfg.clientCert, cfg.clientKey, cfg.caCert, cfg.http2)
	if err != nil {
		log.Fatal(err)
	}

	for time.Since(start).Seconds() <= float64(cfg.duration) && atomic.LoadInt32(&cfg.interrupted) == 0 {
		respSize, reqDur, err := DoRequest(httpClient, cfg.header, cfg.method, cfg.host, cfg.testUrl, cfg.reqBody)
		if err != nil {
			stats.ErrMap[unwrap(err).Error()] += 1
			stats.NumErrs++
		} else if respSize > 0 {
			stats.TotRespSize += int64(respSize)
			stats.TotDuration += reqDur
			stats.Histogram.RecordValue(reqDur.Microseconds())
			stats.NumRequests++
		} else {
			stats.NumErrs++
		}
	}
	cfg.statsAggregator <- stats
}

func (cfg *LoadCfg) Stop() {
	atomic.StoreInt32(&cfg.interrupted, 1)
}
