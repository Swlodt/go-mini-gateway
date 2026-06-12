# 压测方法

压测的目标不是只得到一个漂亮的 QPS，而是验证系统在不同场景下的行为是否符合预期。

建议观察：

- QPS
- 平均延迟
- P95 / P99
- 状态码分布
- 错误率
- Gateway metrics
- CPU / 内存 / goroutine
- 连接数 / TIME_WAIT

---

## 工具

推荐使用 `hey`：

```bash
brew install hey
```

基本用法：

```bash
hey -z 30s -c 100 'http://127.0.0.1:9000/api/hello?name=benchmark'
```

参数说明：

- `-z 30s`：持续压测 30 秒
- `-c 100`：并发 100

---

## 基线压测

先直接压后端，得到 backend baseline：

```bash
hey -z 30s -c 100 'http://127.0.0.1:8081/hello?name=benchmark'
```

再压 Gateway：

```bash
hey -z 30s -c 100 'http://127.0.0.1:9000/api/hello?name=benchmark'
```

对比：

```text
backend direct QPS
gateway proxy QPS
```

这样可以估算代理层开销。

---

## 正常代理压测

```bash
hey -z 30s -c 50 'http://127.0.0.1:9000/api/hello?name=benchmark'
hey -z 30s -c 100 'http://127.0.0.1:9000/api/hello?name=benchmark'
hey -z 30s -c 300 'http://127.0.0.1:9000/api/hello?name=benchmark'
```

观察：

- QPS 是否随并发上升
- P95/P99 是否明显变差
- 是否出现 504
- CPU 是否打满
- 日志 IO 是否成为瓶颈

---

## 路由限流压测

配置：

```json
"rateLimitRPS": 100,
"rateLimitBurst": 100
```

压测：

```bash
hey -z 10s -c 100 'http://127.0.0.1:9000/api/hello?name=benchmark'
```

预期：

```text
约 100 * 10 + burst 个请求返回 200
其他请求返回 429
```

查看：

```bash
curl -s http://127.0.0.1:9001/metrics | grep 'status="429"'
```

---

## 并发限制压测

后端 `/slow` 延迟 1 秒，配置：

```json
"maxConcurrency": 20,
"requestTimeout": "3s"
```

压测：

```bash
hey -z 10s -c 100 'http://127.0.0.1:9000/api/slow'
```

预期：

```text
进入后端的请求返回 200
超出并发上限的请求快速返回 503
```

如果把 `requestTimeout` 改成 `300ms`，进入后端的慢请求会变成 504。

---

## 后端宕机压测

关闭某个 backend 后压测：

```bash
hey -z 10s -c 100 'http://127.0.0.1:9000/api/hello'
```

预期：

- 如果还有其他 upstream 可用，请求应被转发到可用 upstream。
- 如果所有 upstream 都不可用，返回 503 `no available upstream`。

---

## pprof 分析

压测同时采集 CPU profile：

```bash
curl -o cpu.pb.gz \
  -H 'X-Admin-Token: dev-secret' \
  'http://127.0.0.1:9001/debug/pprof/profile?seconds=30'

go tool pprof -http=:0 cpu.pb.gz
```

如果 `log.(*Logger).output`、`fmt.Fprintf`、`os.(*File).Write` 占比明显，说明访问日志可能是瓶颈。

采集 heap：

```bash
curl -o heap.pb.gz \
  -H 'X-Admin-Token: dev-secret' \
  'http://127.0.0.1:9001/debug/pprof/heap'

go tool pprof -http=:0 heap.pb.gz
```

查看 goroutine：

```bash
curl \
  -H 'X-Admin-Token: dev-secret' \
  'http://127.0.0.1:9001/debug/pprof/goroutine?debug=2'
```

---

## 压测记录模板

````markdown
# Benchmark result

## Environment

- OS:
- CPU:
- Memory:
- Go version:
- Gateway commit:
- Config:
- Tool:

## Case

Command:

```bash
hey -z 30s -c 100 http://127.0.0.1:9000/api/hello
```

Result:

- Requests/sec:
- Avg latency:
- P95:
- P99:
- Status code distribution:
- Gateway metrics:

Observation:
````

已有记录见：[benchmark-2026-06-10.md](benchmark-2026-06-10.md)。
