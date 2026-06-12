# 配置说明

配置文件默认路径：

```bash
configs/gateway.json
```

启动时可指定：

```bash
go run ./cmd/gateway -config configs/gateway.json
```

Docker Compose 使用：

```bash
/app/configs/gateway.docker.json
```

---

## 环境变量展开

字符串字段支持环境变量占位符：

```json
"token": "${GATEWAY_ADMIN_TOKEN}"
```

也支持默认值：

```json
"addr": "${GATEWAY_ADMIN_ADDR:127.0.0.1:9001}"
```

语义：

- `${VAR}`：环境变量必须存在且非空，否则配置加载失败。
- `${VAR:default}`：环境变量不存在或为空时使用默认值。
- 默认值里可以包含冒号，例如 `${ADDR:127.0.0.1:9001}`。

当前只支持字符串字段展开，不支持把 int/bool 字段写成 `${VAR}`。

---

## 顶层结构

```json
{
  "server": {},
  "admin": {},
  "routes": []
}
```

---

## server

```json
"server": {
  "addr": ":9000",
  "requestTimeout": "3s",
  "shutdownTimeout": "10s",
  "rateLimitRPS": 0,
  "rateLimitBurst": 0,
  "maxConcurrency": 0
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `addr` | string | 业务 HTTP server 监听地址 |
| `requestTimeout` | duration string | 单个业务请求超时时间 |
| `shutdownTimeout` | duration string | 优雅关闭等待时间 |
| `rateLimitRPS` | int | 全局 RPS 限流。`0` 表示关闭 |
| `rateLimitBurst` | int | 全局限流突发容量 |
| `maxConcurrency` | int | 全局并发限制。`0` 表示关闭 |

---

## admin

```json
"admin": {
  "enabled": true,
  "addr": "127.0.0.1:9001",
  "token": "${GATEWAY_ADMIN_TOKEN}",
  "metricsRequireToken": false,
  "pprofEnabled": true
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `enabled` | bool | 是否启用 admin server |
| `addr` | string | admin server 监听地址 |
| `token` | string | `/admin/*` 和 pprof 的访问 token |
| `metricsRequireToken` | bool | `/metrics` 是否也要求 token |
| `pprofEnabled` | bool | 是否启用 `/debug/pprof/*` |

建议本地开发使用：

```json
"addr": "127.0.0.1:9001"
```

Docker 容器内使用：

```json
"addr": ":9001"
```

---

## routes

```json
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
    ]
  }
]
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 路由 ID，用于日志、metrics 和 admin 展示 |
| `prefix` | string | 匹配的请求路径前缀 |
| `stripPrefix` | string | 转发到后端前剥离的路径前缀 |
| `target` | string | 单 upstream 兼容字段 |
| `upstreams` | array | 多 upstream 配置 |
| `rateLimitRPS` | int | 路由级 RPS 限流 |
| `rateLimitBurst` | int | 路由级限流突发容量 |
| `maxConcurrency` | int | 路由级并发限制 |
| `healthCheck` | object | upstream 级主动健康检查配置 |
| `passiveHealth` | object | upstream 级被动健康检查配置 |
| `circuitBreaker` | object | upstream 级熔断器配置 |

`target` 与 `upstreams` 兼容规则：

- 只配置 `target`：内部会生成一个 `default` upstream。
- 只配置 `upstreams`：`target` 会默认使用第一个 upstream 的 URL，用于兼容 admin 展示。
- 推荐新配置使用 `upstreams`。

---

## healthCheck

```json
"healthCheck": {
  "enabled": true,
  "path": "/health",
  "interval": "3s",
  "timeout": "1s"
}
```

主动健康检查是 upstream 级别。每个 upstream 都会独立检查：

```text
<upstream.url> + healthCheck.path
```

如果某个 upstream 健康检查失败，只会跳过该 upstream，不会影响整个 route。

---

## passiveHealth

```json
"passiveHealth": {
  "enabled": true,
  "failureThreshold": 3,
  "successThreshold": 1,
  "unhealthyDuration": "10s"
}
```

被动健康检查基于真实代理请求结果：

- 连接失败、超时、后端返回 5xx：记为失败。
- 后端返回 2xx/3xx/4xx：记为成功。

连续失败达到阈值后，upstream 会临时标记为 unhealthy，负载均衡选择时会跳过。

---

## circuitBreaker

```json
"circuitBreaker": {
  "enabled": true,
  "failureThreshold": 5,
  "openDuration": "10s",
  "halfOpenMaxRequests": 2
}
```

熔断器也是 upstream 级别，包含三种状态：

```text
CLOSED -> OPEN -> HALF_OPEN -> CLOSED
```

- `CLOSED`：正常放行。
- `OPEN`：快速拒绝，不访问该 upstream。
- `HALF_OPEN`：冷却时间到期后，允许少量试探请求。

---

## 热加载限制

`POST /admin/reload` 支持热加载：

- routes
- upstreams
- rateLimitRPS/rateLimitBurst
- maxConcurrency
- healthCheck
- passiveHealth
- circuitBreaker
- requestTimeout

以下配置变更需要重启：

- `server.addr`
- `server.shutdownTimeout`
- `admin.enabled`
- `admin.addr`
- `admin.token`
- `admin.metricsRequireToken`
- `admin.pprofEnabled`
