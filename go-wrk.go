package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	histo "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/LeoBenChoi/go-wrk-cn/loader"
	"github.com/LeoBenChoi/go-wrk-cn/util"
)

const APP_VERSION = "0.10"

// 可以通过命令行覆盖的默认值
var versionFlag bool = false
var helpFlag bool = false
var duration int = 10 //seconds
var goroutines int = 2
var testUrl string
var method string = "GET"
var host string
var headerFlags util.HeaderList
var header map[string]string
var statsAggregator chan *loader.RequesterStats
var timeoutms int
var allowRedirectsFlag bool = false
var disableCompression bool
var disableKeepAlive bool
var skipVerify bool
var playbackFile string
var reqBody string
var clientCert string
var clientKey string
var caCert string
var http2 bool
var cpus int = 0

func init() {
	flag.BoolVar(&versionFlag, "v", false, "打印版本详情")
	flag.BoolVar(&allowRedirectsFlag, "redir", false, "允许重定向")
	flag.BoolVar(&helpFlag, "help", false, "打印帮助信息")
	flag.BoolVar(&disableCompression, "no-c", false, "禁用压缩 - 阻止发送 \"Accept-Encoding: gzip\" 请求头")
	flag.BoolVar(&disableKeepAlive, "no-ka", false, "禁用 KeepAlive - 阻止在不同 HTTP 请求之间重用 TCP 连接")
	flag.BoolVar(&skipVerify, "no-vr", false, "跳过验证服务器的 SSL 证书")
	flag.IntVar(&goroutines, "c", 10, "使用的 goroutine 数量（并发连接数）")
	flag.IntVar(&duration, "d", 10, "测试持续时间（秒）")
	flag.IntVar(&timeoutms, "T", 1000, "Socket/请求超时时间（毫秒）")
	flag.IntVar(&cpus, "cpus", 0, "CPU 数量，即 GOMAXPROCS。0 = 系统默认值。")
	flag.StringVar(&method, "M", "GET", "HTTP 方法")
	flag.StringVar(&host, "host", "", "Host 请求头")
	flag.Var(&headerFlags, "H", "添加到每个请求的请求头（可以定义多个 -H 标志）")
	flag.StringVar(&playbackFile, "f", "<empty>", "回放文件名")
	flag.StringVar(&reqBody, "body", "", "请求体字符串或 @文件名")
	flag.StringVar(&clientCert, "cert", "", "用于验证对等方的 CA 证书文件（SSL/TLS）")
	flag.StringVar(&clientKey, "key", "", "私钥文件名（SSL/TLS）")
	flag.StringVar(&caCert, "ca", "", "用于验证对等方的 CA 文件（SSL/TLS）")
	flag.BoolVar(&http2, "http", true, "使用 HTTP/2")
}

// printDefaults 以更友好的格式打印默认值
func printDefaults() {
	fmt.Println("用法: go-wrk-cn <options> <url>")
	fmt.Println("选项:")
	flag.VisitAll(func(flag *flag.Flag) {
		if flag.DefValue != "" {
			fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(默认值 "+flag.DefValue+")")
		} else {
			fmt.Println("\t-"+flag.Name, "\t", flag.Usage)
		}
	})
}

// mapToString 将 map[string]int 转换为字符串，格式为 "key=value,key=value,..."
func mapToString(m map[string]int) string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, fmt.Sprint(k, "=", v))
	}
	return strings.Join(s, ",")
}

func main() {

	statsAggregator = make(chan *loader.RequesterStats, goroutines)
	sigChan := make(chan os.Signal, 1)

	// 这里是为了监听来自操作系统的中断信号（如 Ctrl+C），并将该信号传递到 sigChan 通道，以便可以在后续优雅地处理中断（如提前结束测试、资源回收等）。
	signal.Notify(sigChan, os.Interrupt)

	flag.Parse() // 扫描参数列表
	header = make(map[string]string)
	for _, hdr := range headerFlags {
		hp := strings.SplitN(hdr, ":", 2)
		header[hp[0]] = hp[1]
	}

	if playbackFile != "<empty>" {
		file, err := os.Open(playbackFile) // 打开回放文件
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer file.Close()
		url, err := io.ReadAll(file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		testUrl = string(url)
	} else {
		testUrl = flag.Arg(0)
	}

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(testUrl) == 0 {
		printDefaults()
		return
	}

	if cpus > 0 {
		runtime.GOMAXPROCS(cpus)
	}

	fmt.Printf("正在运行 %v 秒的测试 @ %v\n  同时有 %v 个 goroutine 并发执行\n", duration, testUrl, goroutines)

	if len(reqBody) > 0 && reqBody[0] == '@' {
		bodyFilename := reqBody[1:]
		data, err := os.ReadFile(bodyFilename)
		if err != nil {
			fmt.Println(fmt.Errorf("无法读取文件 %q: %v", bodyFilename, err))
			os.Exit(1)
		}
		reqBody = string(data)
	}

	loadGen := loader.NewLoadCfg(
		duration,
		goroutines,
		testUrl,
		reqBody,
		method,
		host,
		header,
		statsAggregator,
		timeoutms,
		allowRedirectsFlag,
		disableCompression,
		disableKeepAlive,
		skipVerify,
		clientCert,
		clientKey,
		caCert,
		http2,
	)

	start := time.Now()

	for i := 0; i < goroutines; i++ {
		go loadGen.RunSingleLoadSession()
	}

	responders := 0
	aggStats := loader.RequesterStats{ErrMap: make(map[string]int), Histogram: histo.New(1, int64(duration*1000000), 4)}

	for responders < goroutines {
		select {
		case <-sigChan:
			loadGen.Stop()
			fmt.Printf("正在停止...\n")
		case stats := <-statsAggregator:
			aggStats.NumErrs += stats.NumErrs
			aggStats.NumRequests += stats.NumRequests
			aggStats.TotRespSize += stats.TotRespSize
			aggStats.TotDuration += stats.TotDuration
			responders++
			for k, v := range stats.ErrMap {
				aggStats.ErrMap[k] += v
			}
			aggStats.Histogram.Merge(stats.Histogram)
		}
	}

	duration := time.Since(start)

	if aggStats.NumRequests == 0 {
		fmt.Println("错误：未收集到统计数据 / 未发现请求")
		fmt.Printf("错误数量:\t%v\n", aggStats.NumErrs)
		if aggStats.NumErrs > 0 {
			fmt.Printf("错误类型统计:\t%v\n", mapToString(aggStats.ErrMap))
		}
		return
	}

	avgThreadDur := aggStats.TotDuration / time.Duration(responders) // 需要对聚合的持续时间求平均

	reqRate := float64(aggStats.NumRequests) / avgThreadDur.Seconds()
	bytesRate := float64(aggStats.TotRespSize) / avgThreadDur.Seconds()

	overallReqRate := float64(aggStats.NumRequests) / duration.Seconds()
	overallBytesRate := float64(aggStats.TotRespSize) / duration.Seconds()

	fmt.Printf("%v 个请求在 %v 内完成, 读取 %v\n", aggStats.NumRequests, avgThreadDur, util.ByteSize{Size: float64(aggStats.TotRespSize)})
	fmt.Printf("每秒请求数:\t\t%.2f\n每秒传输量:\t\t%v\n", reqRate, util.ByteSize{Size: bytesRate})
	fmt.Printf("总请求速率(每秒):\t%.2f\n总传输速率(每秒):\t%v\n", overallReqRate, util.ByteSize{Size: overallBytesRate})
	fmt.Printf("最快请求:\t\t%v\n", toDuration(aggStats.Histogram.Min()))
	fmt.Printf("平均请求耗时:\t\t%v\n", toDuration(int64(aggStats.Histogram.Mean())))
	fmt.Printf("最慢请求:\t\t%v\n", toDuration(aggStats.Histogram.Max()))
	fmt.Printf("错误数量:\t\t%v\n", aggStats.NumErrs)
	if aggStats.NumErrs > 0 {
		fmt.Printf("错误类型统计:\t\t%v\n", mapToString(aggStats.ErrMap))
	}
	fmt.Printf("10%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.10)))
	fmt.Printf("50%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.50)))
	fmt.Printf("75%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.75)))
	fmt.Printf("99%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.99)))
	fmt.Printf("99.9%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.999)))
	fmt.Printf("99.9999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.999999)))
	fmt.Printf("99.99999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.9999999)))
	fmt.Printf("标准偏差:\t\t\t%v\n", toDuration(int64(aggStats.Histogram.StdDev())))
	// aggStats.Histogram.PercentilesPrint(os.Stdout,1,1)
}

func toDuration(usecs int64) time.Duration {
	return time.Duration(usecs * 1000)
}
