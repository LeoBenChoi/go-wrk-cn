go-wrk - HTTP 性能测试工具
==================================

**本项目是对原项目（https://github.com/tsliwowicz/go-wrk）的汉化，虽然项目叫go-wrk-cn，但是使用命令还算是go-wrk，代码方面只修改了弃用的库**

go-wrk 是一个现代化的 HTTP 性能测试工具，在单台多核 CPU 上运行时能够产生显著的负载。它基于 Go 语言的 goroutine 和调度器，在后台实现异步 IO 和并发。

创建 go-wrk 主要是为了检验 Go 语言（http://golang.org）与 C 语言（wrk 是用 C 语言编写的，参见 - <https://github.com/wg/wrk>）在性能和代码简洁性方面的对比。  
事实证明，go-wrk 在吞吐量方面同样出色！而且代码量更少。  

go-wrk 的大部分代码是在一个下午完成的，其质量可与 wrk 相媲美。

构建
--------

    go install github.com/LeoBenChoi/go-wrk-cn@latest

这将下载并编译 go-wrk。 

命令行参数 (./go-wrk -help)  
	
       Usage: go-wrk <options> <url>
       Options:
        -H       添加到每个请求的请求头（可以定义多个 -H 标志）(默认值 )
        -M       HTTP 方法 (默认值 GET)
        -T       Socket/请求超时时间（毫秒）(默认值 1000)
        -body    请求体字符串或 @文件名 (默认值 )
        -c       使用的 goroutine 数量（并发连接数）(默认值 10)
        -ca      用于验证对等方的 CA 文件（SSL/TLS）(默认值 )
        -cert    用于验证对等方的 CA 证书文件（SSL/TLS）(默认值 )
        -d       测试持续时间（秒）(默认值 10)
        -f       回放文件名 (默认值 <empty>)
        -help    打印帮助信息 (默认值 false)
        -host    Host 请求头 (默认值 )
        -http    使用 HTTP/2 (默认值 true)
        -key     私钥文件名（SSL/TLS）(默认值 )
        -no-c    禁用压缩 - 阻止发送 "Accept-Encoding: gzip" 请求头 (默认值 false)
        -no-ka   禁用 KeepAlive - 阻止在不同 HTTP 请求之间重用 TCP 连接 (默认值 false)
        -no-vr   跳过验证服务器的 SSL 证书 (默认值 false)
        -redir   允许重定向 (默认值 false)
        -v       打印版本详情 (默认值 false)

基本用法
-----------

    ./go-wrk -c 2048 -d 10 http://localhost:8080/plaintext

这将运行一个持续 10 秒的性能测试，使用 2048 个 goroutine（连接）

输出示例:

    正在运行 10 秒的测试 @ http://localhost:8080/plaintext
        同时有 2048 个 goroutine 并发执行
    439977 个请求在 10.012950719s 内完成, 读取 52.45MB
    每秒请求数:		43940.79
    每秒传输量:		5.24MB
    最快请求:		98µs
    平均请求耗时:		46.608ms
    最慢请求:		398.431ms
    错误数量:		0
    错误类型统计:		map[]
    10%:			    164µs
    50%:			    2.382ms
    75%:			    3.83ms
    99%:			    5.403ms
    99.9%:			    5.488ms
    99.9999%:		    5.5ms
    99.99999%:		    5.5ms
    标准偏差:			29.744ms


性能测试技巧
-----------------

  运行 go-wrk 的机器必须有足够数量的临时端口可用，并且关闭的套接字应该快速回收。为了处理初始连接突发，服务器的 listen(2) 积压队列应该大于正在测试的并发连接数。

致谢
----------------

  golang 非常棒。除了它，我不需要任何其他东西就能创建 go-wrk。  
  我完全感谢 wrk 项目（https://github.com/wg/wrk）提供的灵感和本文的部分内容。  
  我还使用了类似的命令行参数格式和输出格式。
