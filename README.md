# go-mini-gateway

`go-mini-gateway` 是一个基于 Go 标准库实现的轻量级反向代理网关项目。它的目标不是替代 Nginx、Envoy 或 APISIX，而是通过一个可运行、可测试、可压测、可观测、可容器化的项目，系统练习网关类基础设施的核心能力。

当前项目覆盖：

- 配置化反向代理
- 多路由与路径前缀剥离
- 多 upstream 与 round-robin 负载均衡
- 主动健康检查、被动健康检查、熔断器
- 全局/路由级 RPS 限流
- 全局/路由级并发限制
- 请求超时、502/503/504 错误语义
- 访问日志、内存 metrics、Prometheus 文本指标与 Histogram
- 独立 admin server、token 鉴权、pprof
- 配置热加载
- 单元测试、集成测试、压测记录、Docker Compose 交付

---

## 架构概览

```text
client
  │
  ▼
main server :9000
  │
  ├── /ping
  ├── /health
  ├── /version
  └── /api/*
        │
        ▼
      route runtime
        │
        ├── timeout middleware
        ├── global rate limiter
        ├── global concurrency limiter
        ├── route rate limiter
        ├── route concurrency limiter
        └── proxy handler
              │
              ├── round-robin upstream selector
              ├── active health check filter
              ├── passive health filter
              ├── circuit breaker filter
              └── httputil.ReverseProxy
                    │
                    ├── backend-1
                    └── backend-2

admin server :9001
  │
  ├── /admin/routes
  ├── /admin/health
  ├── /admin/stats
  ├── /admin/metrics
  ├── /admin/reload
  ├── /metrics
  └── /debug/pprof/*
```

业务接口和管理接口分离。业务流量只进入 main server，管理、指标、pprof 和热加载接口只暴露在 admin server。

---

## 快速启动

### 方式一：本地 Go 运行

准备环境变量：

```bash
export GATEWAY_ADMIN_TOKEN=dev-secret
```

分别启动后端和网关：

```bash
go run ./cmd/backend
```

```bash
go run ./cmd/backend2
```

```bash
go run ./cmd/gateway -config configs/gateway.json
```

验证：

```bash
curl -i http://127.0.0.1:9000/ping
curl -i 'http://127.0.0.1:9000/api/hello?name=sw'
curl -i -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/routes
```

### 方式二：Docker Compose

```bash
make docker-up
```

或：

```bash
docker compose up --build
```

默认 admin token 为 `dev-token`。可以覆盖：

```bash
GATEWAY_ADMIN_TOKEN=your-token make docker-up
```

验证：

```bash
make docker-smoke
```

更多 Docker 说明见：[docs/docker.md](docs/docker.md)。

---

## 配置示例

```json
{
  "server": {
    "addr": ":9000",
    "requestTimeout": "3s",
    "shutdownTimeout": "10s",
    "rateLimitRPS": 0,
    "rateLimitBurst": 0,
    "maxConcurrency": 0
  },
  "admin": {
    "enabled": true,
    "addr": "127.0.0.1:9001",
    "token": "${GATEWAY_ADMIN_TOKEN}",
    "metricsRequireToken": false,
    "pprofEnabled": true
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "stripPrefix": "/api",
      "upstreams": [
        {
          "id": "backend-1",
          "url": "http://localhost:8081"
        },
        {
          "id": "backend-2",
          "url": "http://localhost:8082"
        }
      ],
      "rateLimitRPS": 100,
      "rateLimitBurst": 100,
      "maxConcurrency": 20,
      "healthCheck": {
        "enabled": true,
        "path": "/health",
        "interval": "3s",
        "timeout": "1s"
      },
      "passiveHealth": {
        "enabled": true,
        "failureThreshold": 3,
        "successThreshold": 1,
        "unhealthyDuration": "10s"
      },
      "circuitBreaker": {
        "enabled": true,
        "failureThreshold": 5,
        "openDuration": "10s",
        "halfOpenMaxRequests": 2
      }
    }
  ]
}
```

完整配置说明见：[docs/configuration.md](docs/configuration.md)。

---

## 管理接口

所有 `/admin/*` 接口都需要请求头：

```http
X-Admin-Token: <token>
```

常用接口：

```bash
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/routes
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/health
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/stats
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/metrics
curl -X POST -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/reload
```

Prometheus 指标：

```bash
curl http://127.0.0.1:9001/metrics
```

pprof：

```bash
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/debug/pprof/
```

接口说明见：[docs/api.md](docs/api.md)。

---

## 观测能力

当前支持两类 metrics：

1. `/admin/metrics`：JSON 格式，便于调试。
2. `/metrics`：Prometheus 文本格式，支持 Histogram。

核心指标包括：

- `gateway_http_requests_total`
- `gateway_http_status_requests_total`
- `gateway_route_http_requests_total`
- `gateway_route_http_status_requests_total`
- `gateway_http_request_duration_seconds_bucket`
- `gateway_route_http_request_duration_seconds_bucket`

P95 PromQL 示例：

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(gateway_route_http_request_duration_seconds_bucket[5m])
  )
)
```

更多指标说明见：[docs/metrics.md](docs/metrics.md)。

---

## 测试

运行全部测试：

```bash
make test
```

或：

```bash
go test ./...
```

当前测试覆盖：

- config 加载、环境变量展开、校验
- rate limiter、concurrency limiter
- metrics 与 Prometheus 输出
- active health、passive health、circuit breaker
- admin token 鉴权
- routeID 记录
- 反向代理完整链路
- 多 upstream round-robin
- 配置热加载

---

## 压测

示例：

```bash
hey -z 30s -c 100 'http://127.0.0.1:9000/api/hello?name=benchmark'
```

已有压测记录见：[docs/benchmark-2026-06-10.md](docs/benchmark-2026-06-10.md)。

压测方法说明见：[docs/benchmark.md](docs/benchmark.md)。

---

## 设计文档

- [配置说明](docs/configuration.md)
- [API 说明](docs/api.md)
- [Metrics 说明](docs/metrics.md)
- [Docker 使用说明](docs/docker.md)
- [压测方法](docs/benchmark.md)
- [设计说明](docs/design.md)
- [故障排查](docs/troubleshooting.md)

---

## 当前能力边界

已支持：

- 多 upstream round-robin
- upstream 级主动健康检查
- upstream 级被动健康检查
- upstream 级熔断器
- 配置热加载
- Prometheus Histogram
- pprof
- Docker Compose 一键启动

暂未支持：

- 加权 round-robin
- least-connections
- 请求失败后自动重试其他 upstream
- 基于滑动窗口失败率的熔断
- Prometheus 直接暴露 upstream 健康/熔断状态指标
- admin server 监听端口热变更
- 配置文件自动监听
- 完整生产级访问日志策略

---

## 项目定位

这个项目适合作为 Go 工程能力和网关基础设施能力的综合练习项目。它重点展示：

- 如何用 Go 标准库构建 HTTP 代理服务
- 如何设计配置化路由和运行时状态
- 如何实现限流、并发保护、健康检查、熔断器
- 如何做测试、压测、指标、pprof 和容器化交付
- 如何在系统演进中逐步重构，而不是一次性过度设计
