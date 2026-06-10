# go-mini-gateway

`go-mini-gateway` 是一个使用 Go 标准库实现的轻量级 HTTP 反向代理网关。
项目当前提供路由转发、路径前缀剥离、请求超时、限流、并发保护、后端健康检查、
访问日志、运行指标和独立管理端口等基础能力。

该项目适合用于学习网关实现、内部服务代理、功能验证和性能实验。当前版本仍是
单机、静态配置、单上游模型，尚未包含生产网关常见的服务发现、多上游负载均衡、
熔断、重试、动态配置和分布式限流等能力。

## 功能

- 基于路径前缀的 HTTP 路由
- 路由前缀剥离
- 请求头转发和网关标识
- 请求 Trace ID 透传或自动生成
- 全局及路由级令牌桶限流
- 全局及路由级并发限制
- 主动后端健康检查
- 请求总超时控制
- 后端错误到 `502`、`504` 的转换
- 独立的管理 HTTP Server
- JSON 格式的运行状态接口
- Prometheus 文本格式 metrics
- 访问日志和优雅关闭
- 配置文件环境变量展开

## 目录结构

```text
.
├── cmd/
│   ├── gateway/          # 网关程序入口
│   ├── backend/          # 测试后端，监听 8081
│   └── backend2/         # 第二个测试后端，监听 8082
├── configs/
│   └── gateway.json      # 默认网关配置
├── docs/
│   └── benchmark-2026-06-10.md
├── internal/
│   ├── concurrency/      # 并发限制器及中间件
│   ├── config/           # 配置加载、环境变量展开和校验
│   ├── health/           # 后端健康检查
│   ├── metrics/          # 内存指标和 Prometheus 输出
│   ├── proxy/            # ReverseProxy 和请求改写
│   ├── ratelimit/        # 令牌桶限流器
│   └── server/           # HTTP Server、路由及管理接口
└── go.mod
```

## 环境要求

- Go 1.26 或兼容版本
- 可用的 HTTP 后端服务
- 可选：`curl`，用于调用示例
- 可选：`hey`，用于本地压测

项目当前只使用 Go 标准库，没有第三方运行时依赖。

## 快速开始

### 1. 启动测试后端

在第一个终端执行：

```bash
go run ./cmd/backend
```

测试后端监听 `:8081`，提供以下接口：

| 接口 | 说明 |
| --- | --- |
| `GET /hello` | 返回简单文本，并输出收到的 Trace ID |
| `GET /hello/v2` | 返回 Host 和转发请求头 |
| `POST /echo` | 返回请求方法、路径和请求体 |
| `GET /error` | 固定返回 `503` |
| `GET /slow` | 等待 5 秒后返回 |
| `GET /health` | 健康检查接口 |

### 2. 设置管理端 Token

默认配置要求提供 `GATEWAY_ADMIN_TOKEN`：

```bash
export GATEWAY_ADMIN_TOKEN=dev-secret
```

### 3. 启动网关

```bash
go run ./cmd/gateway -config configs/gateway.json
```

默认启动两个 HTTP Server：

- 业务端口：`http://127.0.0.1:9000`
- 管理端口：`http://127.0.0.1:9001`

### 4. 验证代理

```bash
curl 'http://127.0.0.1:9000/api/hello?name=world'
```

请求处理过程如下：

```text
GET /api/hello?name=world
        |
        | 匹配路由前缀 /api/
        | 剥离 /api
        v
GET http://localhost:8081/hello?name=world
```

网关返回的响应包含：

```text
X-Gateway: go-mini-gateway
X-Gateway-Route: demo
```

### 5. 验证管理接口

```bash
curl \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/routes

curl http://127.0.0.1:9001/metrics
```

## 构建和运行

构建网关：

```bash
go build -o bin/gateway ./cmd/gateway
```

构建测试后端：

```bash
go build -o bin/backend ./cmd/backend
go build -o bin/backend2 ./cmd/backend2
```

运行：

```bash
GATEWAY_ADMIN_TOKEN=dev-secret \
  ./bin/gateway -config configs/gateway.json
```

未指定 `-config` 时，默认读取 `configs/gateway.json`。

网关收到 `SIGINT` 或 `SIGTERM` 后会停止接收新连接，在
`server.shutdownTimeout` 指定的时间内等待正在处理的请求结束，然后关闭后端
空闲连接、限流器和健康检查任务。

## 配置

默认配置：

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
    "metricsRequireToken": false
  },
  "routes": [
    {
      "id": "demo",
      "prefix": "/api/",
      "stripPrefix": "/api",
      "target": "${DEMO_BACKEND_URL:http://localhost:8081}",
      "rateLimitRPS": 0,
      "rateLimitBurst": 0,
      "maxConcurrency": 0,
      "healthCheck": {
        "enabled": true,
        "path": "/health",
        "interval": "3s",
        "timeout": "1s"
      }
    }
  ]
}
```

### Server 配置

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `server.addr` | string | `:8080` | 业务 HTTP Server 监听地址 |
| `server.requestTimeout` | duration | `3s` | 单个业务请求的总超时 |
| `server.shutdownTimeout` | duration | `10s` | 优雅关闭等待时间 |
| `server.rateLimitRPS` | int | `0` | 全局允许速率，`0` 表示关闭 |
| `server.rateLimitBurst` | int | RPS 值 | 全局令牌桶容量 |
| `server.maxConcurrency` | int | `0` | 全局最大并发请求数，`0` 表示关闭 |

`duration` 使用 Go 时间格式，例如 `500ms`、`3s`、`2m`。

### Admin 配置

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `admin.enabled` | bool | `false` | 是否启动独立管理 HTTP Server |
| `admin.addr` | string | `127.0.0.1:9001` | 管理 Server 监听地址 |
| `admin.token` | string | 无 | 管理接口 Token，启用管理端时必填 |
| `admin.metricsRequireToken` | bool | `false` | `/metrics` 是否也要求 Token |

管理接口通过请求头认证：

```text
X-Admin-Token: <admin.token>
```

建议只将管理端口暴露在本机或受控管理网络。生产配置中不应直接写入明文 Token，
应通过环境变量或密钥管理系统注入。

### Route 配置

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `routes[].id` | string | 无 | 路由唯一标识 |
| `routes[].prefix` | string | `/` | 请求匹配前缀 |
| `routes[].stripPrefix` | string | `/` | 转发前从路径中剥离的前缀 |
| `routes[].target` | URL | 无 | 后端地址，必须包含 scheme 和 host |
| `routes[].rateLimitRPS` | int | `0` | 路由级允许速率，`0` 表示关闭 |
| `routes[].rateLimitBurst` | int | RPS 值 | 路由级令牌桶容量 |
| `routes[].maxConcurrency` | int | `0` | 路由级最大并发数，`0` 表示关闭 |
| `routes[].healthCheck.enabled` | bool | `false` | 是否启用主动健康检查 |
| `routes[].healthCheck.path` | string | `/health` | 后端健康检查路径 |
| `routes[].healthCheck.interval` | duration | `5s` | 健康检查周期 |
| `routes[].healthCheck.timeout` | duration | `1s` | 单次健康检查超时 |

配置约束：

- 至少需要一条路由。
- 路由 ID 和规范化后的路由前缀不能重复。
- `stripPrefix` 必须是 `prefix` 的前缀。
- 限流值和并发值不能为负数，当前最大值为 `100000`。
- 开启健康检查时，`timeout` 必须小于 `interval`。
- `target` 必须是包含协议和主机的完整 URL。

### 路径规范化

路由前缀会自动补齐开头和结尾的 `/`。例如：

```text
api -> /api/
```

`stripPrefix` 会自动补齐开头的 `/`，并移除非根路径末尾的 `/`。例如：

```text
api/ -> /api
```

对于 `prefix=/api/`，网关同时注册 `/api/` 和 `/api`。请求 `/api/hello`
在剥离 `/api` 后会以 `/hello` 转发给后端。

### 环境变量展开

所有字符串配置字段支持以下形式：

```text
${VARIABLE}
${VARIABLE:default-value}
```

示例：

```json
{
  "target": "${DEMO_BACKEND_URL:http://localhost:8081}"
}
```

- 环境变量存在且非空时使用环境变量值。
- 环境变量不存在或为空时，使用配置中的默认值。
- 没有默认值且环境变量不存在时，网关启动失败。

## 请求处理

业务请求大致经过以下链路：

```text
客户端
  -> 访问日志与 metrics
  -> 全局限流
  -> 全局并发限制
  -> 请求超时
  -> 路由匹配
  -> 路由限流
  -> 路由并发限制
  -> 后端健康状态判断
  -> ReverseProxy
  -> 后端
```

中间件通过包装方式构建，实际进入顺序以最外层中间件为准。限流和并发限制均为
非阻塞模式：容量不足时立即拒绝，不会在网关内部排队等待。

### 请求改写

转发时网关会：

- 将请求 URL 指向路由 `target`
- 按配置剥离路径前缀
- 保留查询参数
- 设置标准转发头
- 设置 `X-Gateway: go-mini-gateway`
- 设置 `X-Gateway-Route: <route-id>`
- 透传客户端的 `X-Trace-ID`
- 客户端未提供 Trace ID 时自动生成

后端响应会增加：

```text
X-Gateway: go-mini-gateway
X-Gateway-Route: <route-id>
```

### 限流

全局和路由级限流均为本进程内令牌桶：

- 启动时令牌桶处于满状态。
- 每个请求消耗一个令牌。
- 没有令牌时立即返回 `429 Too Many Requests`。
- 全局与路由限流同时启用时，请求必须同时通过两级限流。
- 多网关实例之间不共享令牌状态。

### 并发限制

并发限制使用非阻塞信号量：

- 请求进入时尝试占用一个并发槽位。
- 请求结束后释放槽位。
- 没有槽位时立即返回 `503 Service Unavailable`。
- 全局与路由并发限制可以同时启用。

### 健康检查

启用健康检查后，网关按周期向以下地址发起 `GET`：

```text
<route.target><healthCheck.path>
```

状态码在 `200-299` 范围内时视为健康。检查超时、网络错误或非 2xx 响应会将
后端标记为不健康；后续业务请求立即返回 `503 Service Unavailable`。

首次健康检查完成之前，业务请求默认允许通过。当前实现一次检查结果就会直接
切换健康状态，尚未提供连续失败阈值或连续成功恢复阈值。

### 后端连接池

每条路由使用独立的 `http.Transport`，当前参数固定为：

| 参数 | 值 |
| --- | ---: |
| `MaxIdleConns` | 100 |
| `MaxIdleConnsPerHost` | 20 |
| `MaxConnsPerHost` | 100 |
| `IdleConnTimeout` | 90 秒 |
| `ResponseHeaderTimeout` | 10 秒 |
| `TLSHandshakeTimeout` | 10 秒 |
| `ExpectContinueTimeout` | 1 秒 |

这些参数目前不能通过配置文件修改。

## HTTP 接口

### 业务端内置接口

以下接口位于 `server.addr`：

| 方法 | 路径 | 响应 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/ping` | `pong` | 进程探测 |
| `GET` | `/health` | `ok` | 网关自身健康接口 |
| `GET` | `/version` | `go-mini-gateway v0.1.0` | 当前静态版本 |

这些接口只表示网关进程能够处理 HTTP 请求，不汇总后端健康状态。

### 管理接口

以下接口位于 `admin.addr`：

| 方法 | 路径 | 认证 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/admin/routes` | Token | 查看规范化后的路由配置 |
| `GET` | `/admin/health` | Token | 查看所有路由的健康状态 |
| `GET` | `/admin/stats` | Token | 查看限流、并发和健康快照 |
| `GET` | `/admin/metrics` | Token | 查看 JSON 格式累计指标 |
| `GET` | `/metrics` | 可配置 | Prometheus 文本格式指标 |

业务端口不会暴露 `/admin/*` 和 `/metrics`。

调用示例：

```bash
curl \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/health
```

### Prometheus 指标

当前提供的主要指标包括：

```text
gateway_uptime_seconds
gateway_http_requests_total
gateway_http_bytes_written_total
gateway_http_request_duration_avg_milliseconds
gateway_http_request_duration_max_milliseconds
gateway_http_status_requests_total
gateway_route_http_requests_total
gateway_route_http_bytes_written_total
gateway_route_http_status_requests_total
gateway_route_http_request_duration_avg_milliseconds
gateway_route_http_request_duration_max_milliseconds
```

路由指标使用 `route` 标签，状态码指标使用 `status` 标签。

## 状态码

| 状态码 | 来源 | 说明 |
| ---: | --- | --- |
| `200-599` | 后端 | 正常情况下保留后端原始状态码 |
| `404` | 网关 | 没有匹配到路由或内置接口 |
| `405` | 网关 | 内置或管理接口使用了不支持的方法 |
| `429` | 网关 | 全局或路由级限流拒绝请求 |
| `502` | 网关 | 无法连接后端或发生非超时代理错误 |
| `503` | 网关 | 并发限制拒绝，或健康检查判定后端不可用 |
| `504` | 网关 | 请求 context 或后端网络操作超时 |

## 日志

网关使用标准库 `log` 输出日志，包括：

- Server 启动和停止
- 路由注册
- 后端健康状态变化
- 每个请求的访问日志
- 限流和并发限制拒绝
- 代理后端错误

访问日志示例：

```text
access route=demo method=GET path=/api/hello query="name=world" \
status=200 bytes=69 cost=1.2ms remote=127.0.0.1:50000 user_agent="curl/8.7.1"
```

当前访问日志为同步逐请求输出。高吞吐场景应关注日志目标的写入速度，否则日志
输出可能成为性能瓶颈。

## 测试

运行全部测试：

```bash
go test ./...
```

运行竞态检查：

```bash
go test -race ./...
```

运行指定包：

```bash
go test ./internal/server -run TestGateway -v
go test ./internal/health -v
go test ./internal/ratelimit -v
```

测试覆盖的主要行为包括：

- 路由代理和路径剥离
- 转发头与 Trace ID
- 后端状态码保留
- 后端不可用时返回 `502`
- 后端超时时返回 `504`
- 限流器、并发限制器及路由级集成行为
- 健康检查及不健康拒绝
- 管理端口隔离和 Token 认证
- metrics 记录及 Prometheus 输出
- 配置规范化、校验和环境变量展开

## 压测

仓库中的 [压测记录](docs/benchmark-2026-06-10.md) 包含测试环境、命令、结果和
限制条件。

该次本机短时测试的主要结果：

| 目标 | 并发 | RPS | P99 | 错误 |
| --- | ---: | ---: | ---: | --- |
| 后端直连 | 50 | 129,799 | 1.2 ms | 0 |
| 网关 | 50 | 31,386 | 4.2 ms | 0 |
| 网关 | 75 | 25,446 | 18.2 ms | 0 |
| 网关 | 100 | 13,915 | 62.1 ms | 75 个 504 |

该结果来自客户端、网关和后端运行在同一台机器的测试，只适合当前提交和环境下
的性能参考，不代表生产容量。

## 当前限制

当前版本尚未提供：

- 多上游实例和负载均衡
- 服务发现
- 熔断、半开恢复和被动健康检查
- 幂等请求重试和重试预算
- 动态配置及配置热加载
- 分布式限流
- 按客户端、IP、用户或 API Key 限流
- TLS Server 和客户端 mTLS 配置
- JWT、API Key 等业务认证
- 请求体和请求头大小限制
- 可配置的后端连接池
- 结构化日志、日志采样和异步日志
- 标准 Prometheus Histogram
- OpenTelemetry 链路追踪
- readiness 与 liveness 分离

在用于生产流量之前，应至少补充过载保护参数、健康检查阈值、HTTP 安全边界、
上游故障隔离、可观测性优化和长时间故障压测。

## 开发建议

修改代码后建议依次执行：

```bash
gofmt -w $(find cmd internal -name '*.go')
go test ./...
go test -race ./...
go vet ./...
```

涉及请求链路、限流、连接池或 metrics 的改动，应使用相同环境和参数重新执行
基准测试，并将结果与 [现有压测记录](docs/benchmark-2026-06-10.md) 对比。

