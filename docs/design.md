# 设计说明

本文描述 `go-mini-gateway` 当前核心设计。

---

## 设计目标

项目重点不是完整替代生产网关，而是覆盖网关类基础设施常见能力：

- 配置化路由
- 反向代理
- 负载均衡
- 限流与并发保护
- 健康检查
- 熔断
- 可观测性
- 热加载
- 容器化交付

---

## Server 拆分

当前有两个 HTTP server：

```text
main server
  处理业务请求

admin server
  处理管理接口、metrics、pprof、reload
```

这样做的好处：

1. 管理接口不会和业务路由混在一起。
2. admin server 可以只绑定 `127.0.0.1`。
3. 业务全局限流/并发限制不会影响 admin 接口。
4. pprof 不会暴露在业务端口。

---

## 请求链路

业务请求经过：

```text
accessLogMiddleware
  ↓
global rate limiter
  ↓
global concurrency limiter
  ↓
timeoutMiddleware
  ↓
ServeMux
  ↓
route rate limiter
  ↓
route concurrency limiter
  ↓
withRouteID
  ↓
proxy.Handler
  ↓
upstream selector
  ↓
httputil.ReverseProxy
```

状态码语义：

| 状态码 | 含义 |
| --- | --- |
| 200/其他后端状态码 | 后端正常响应，Gateway 原样透传 |
| 429 | RPS 限流触发 |
| 503 | 并发限制、无可用 upstream、健康检查/熔断导致不可用 |
| 504 | Gateway 等待后端超时 |
| 502 | Gateway 连接后端失败或代理错误 |

---

## Route / Upstream 模型

当前核心模型：

```text
Route
  ├── id
  ├── prefix
  ├── stripPrefix
  ├── rateLimiter
  ├── concurrencyLimiter
  └── proxy.Handler
        └── Upstreams[]
              ├── url
              ├── activeHealth
              ├── passiveHealth
              └── circuitBreaker
```

从多 upstream 开始，健康状态、被动健康检查和熔断器都升级为 upstream 级别。

这样可以保证：

```text
backend-1 不可用，只跳过 backend-1
backend-2 仍然可以继续承接流量
```

---

## 负载均衡

当前实现：round-robin。

选择逻辑：

```text
从 nextIndex 开始遍历 upstream
  ↓
跳过 active health unavailable
  ↓
跳过 passive health unavailable
  ↓
跳过 circuit breaker unavailable
  ↓
选中第一个 available upstream
```

如果没有可用 upstream：

```text
503 no available upstream
```

当前暂未实现：

- weight
- least-connections
- request retry

---

## 主动健康检查

主动健康检查是 upstream 级别。

```text
每个 upstream 独立请求：upstream.url + healthCheck.path
```

特点：

- 第一次检查完成前 fail-open。
- 某个 upstream 检查失败时，只影响该 upstream。
- admin 接口可查看每个 upstream 的 active health snapshot。

---

## 被动健康检查

被动健康检查基于真实代理请求结果：

```text
代理错误 / 超时 / 5xx
  记录失败

2xx / 3xx / 4xx
  记录成功
```

连续失败达到阈值后，upstream 会被临时标记为 unhealthy。`unhealthyDuration` 到期后允许试探请求，成功达到 `successThreshold` 后恢复。

---

## 熔断器

熔断器是 upstream 级别，经典三态：

```text
CLOSED
  正常请求
  连续失败达到 failureThreshold 后进入 OPEN

OPEN
  快速拒绝
  openDuration 后允许进入 HALF_OPEN

HALF_OPEN
  允许少量试探请求
  成功恢复 CLOSED
  失败重新 OPEN
```

熔断器和被动健康检查的区别：

| 机制 | 关注点 |
| --- | --- |
| passive health | 这个 upstream 当前是否健康 |
| circuit breaker | 最近失败过多时，是否临时保护 upstream 和 Gateway |

---

## Metrics 设计

metrics 使用内存聚合，记录：

- 总请求数
- route 请求数
- 状态码分布
- 响应字节数
- 平均延迟/最大延迟
- Histogram

当前实现简单直观，适合学习和本地压测。高 QPS 下，全局锁可能成为潜在瓶颈，后续可以考虑：

- 分片计数器
- 原子计数器
- per-route 独立 registry
- upstream 级 metrics

---

## 配置热加载

热加载使用 handler 原子替换模型：

```text
读取新配置
  ↓
校验不可热加载字段
  ↓
构建新 runtime
  ↓
原子替换 main handler
  ↓
关闭旧 runtime 资源
```

为什么不直接修改 `http.ServeMux`：

1. `ServeMux` 不支持删除旧路由。
2. 重复注册 pattern 会 panic。
3. 修改旧 mux 容易和正在处理的请求产生并发问题。

当前不支持热加载监听端口和 admin 配置，因为这涉及 server 生命周期。

---

## 资源释放

关闭或热加载时需要释放：

- proxy transport idle connections
- rate limiter ticker
- active health checker goroutine

当前通过 `closeRuntimeResources` 统一处理。

---

## 当前设计边界

当前仍是学习项目，不是完整生产网关。已知边界：

- 没有请求重试。
- 没有 TLS 终止。
- 没有动态服务发现。
- 没有配置文件自动 watch。
- 没有加权负载均衡。
- 没有分布式限流。
- 没有多实例共享健康状态。
- admin token 仍然是简单静态 token。
