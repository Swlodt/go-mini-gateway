# go-mini-gateway 压测记录

## 测试信息

- 时间：2026-06-10 21:41-21:45 CST
- Git 提交：`66fd1c4`
- 工作区状态：存在未提交修改，本次按当前工作区代码构建和测试
- 操作系统：macOS 26.5.1，arm64
- 硬件：Apple M4 Pro，14 核（10 性能核 + 4 能效核），48 GB 内存
- Go：`go1.26.1 darwin/arm64`
- 压测工具：`hey`
- 网关地址：`http://127.0.0.1:9000`
- 后端地址：`http://127.0.0.1:8081`
- 测试接口：`GET /api/hello?name=benchmark`

## 测试配置

使用 `configs/gateway.json`：

- 请求超时：3 秒
- 全局限流：关闭
- 全局并发限制：关闭
- 路由限流：关闭
- 路由并发限制：关闭
- 后端健康检查：开启
- 后端连接池：`MaxConnsPerHost=100`，`MaxIdleConnsPerHost=20`

网关会为每个请求同步输出访问日志。压测时将标准输出和错误输出重定向到
`/dev/null`，避免终端缓冲阻塞进程；日志格式化及写调用仍属于被测开销。

压测前执行：

```bash
go test ./...
go build -o /tmp/go-mini-gateway-backend ./cmd/backend
go build -o /tmp/go-mini-gateway ./cmd/gateway

/tmp/go-mini-gateway-backend
GATEWAY_ADMIN_TOKEN=benchmark \
  /tmp/go-mini-gateway -config configs/gateway.json >/dev/null 2>&1
```

所有测试包均通过。

## 压测命令

预热：

```bash
NO_PROXY=127.0.0.1,localhost hey -n 2000 -c 20 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
```

后端直连基线：

```bash
NO_PROXY=127.0.0.1,localhost hey -n 500000 -c 50 \
  'http://127.0.0.1:8081/hello?name=benchmark'
```

网关阶梯压测：

```bash
NO_PROXY=127.0.0.1,localhost hey -z 15s -c 1 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
NO_PROXY=127.0.0.1,localhost hey -z 15s -c 10 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
NO_PROXY=127.0.0.1,localhost hey -z 15s -c 50 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
NO_PROXY=127.0.0.1,localhost hey -z 10s -c 75 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
NO_PROXY=127.0.0.1,localhost hey -z 15s -c 100 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
NO_PROXY=127.0.0.1,localhost hey -z 10s -c 100 \
  'http://127.0.0.1:9000/api/hello?name=benchmark'
```

## 测试结果

| 目标 | 并发 | 时长/请求数 | 请求数 | RPS | 平均延迟 | P95 | P99 | 最慢 | 状态码 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 后端直连 | 50 | 500,000 请求 | 500,000 | 129,799 | 0.4 ms | 0.8 ms | 1.2 ms | 11.4 ms | 200: 500,000 |
| 网关 | 1 | 15 秒 | 156,158 | 10,410 | 0.1 ms | 0.2 ms | 0.2 ms | 1.3 ms | 200: 156,158 |
| 网关 | 10 | 15 秒 | 348,183 | 23,211 | 0.4 ms | 0.7 ms | 0.8 ms | 5.8 ms | 200: 348,183 |
| 网关 | 50 | 15 秒 | 470,830 | 31,386 | 1.6 ms | 3.2 ms | 4.2 ms | 15.9 ms | 200: 470,830 |
| 网关 | 75 | 10 秒 | 254,502 | 25,446 | 2.9 ms | 9.1 ms | 18.2 ms | 35.7 ms | 200: 254,502 |
| 网关 | 100 | 15 秒 | 209,200 | 13,915 | 7.2 ms | 22.4 ms | 62.1 ms | 189.3 ms | 200: 209,125；504: 75 |
| 网关复测 | 100 | 10 秒 | 97,687 | 9,721 | 10.3 ms | 28.1 ms | 116.3 ms | 277.6 ms | 200: 97,461；504: 226 |

压测后管理端 metrics 核对：

```text
gateway_http_requests_total 1538560
gateway_http_status_requests_total{status="200"} 1538259
gateway_http_status_requests_total{status="504"} 301
gateway_http_request_duration_avg_milliseconds 1.598868
gateway_http_request_duration_max_milliseconds 200.535875
```

metrics 总数与预热及六轮网关压测请求数完全一致。

## 结论

1. 当前机器和配置下，网关峰值出现在并发 50 左右，约为 31,386 req/s，
   P99 为 4.2 ms，全部请求成功。
2. 并发提高到 75 后，吞吐下降到 25,446 req/s，P99 增至 18.2 ms，
   但本轮仍无错误。
3. 并发 100 的两轮测试都出现吞吐明显下降和 504，错误率分别约为
   0.036% 和 0.231%，说明该并发下已经超过当前配置的稳定工作区间。
4. 后端直连基线约为 129,799 req/s。网关并发 50 的吞吐约为直连基线的
   24.2%。该差距包含反向代理、请求改写、响应头修改、metrics、超时中间件
   和逐请求访问日志等全部成本。
5. 建议当前配置将稳定容量按不高于 25,000 req/s 规划，并在真实部署环境中
   使用独立压测机、生产日志策略和目标响应体重新验证。

## 注意事项

- 压测客户端、网关和后端位于同一台机器，CPU 与网络栈会相互竞争；结果适合
  本机回归和定位拐点，不等同于生产容量。
- 并发 100 恰好达到后端连接池的 `MaxConnsPerHost=100`，同时
  `MaxIdleConnsPerHost=20` 较低，连接复用和连接抖动可能影响高并发结果。
- 最初使用 `hey -z 15s -c 50` 测后端直连时，工具输出的总时长、请求数和
  响应大小互相不一致，因此该轮已作废，基线改用固定 500,000 请求的结果。
- 本记录只描述当前工作区和当前机器的一次测试结果，不应直接与其他提交、
  机器或配置的结果横向比较。
