# API 说明

当前服务分为两个 HTTP server：

- main server：业务端口，默认 `:9000`
- admin server：管理端口，默认 `127.0.0.1:9001`

---

## main server

### GET /ping

健康探测接口。

```bash
curl -i http://127.0.0.1:9000/ping
```

响应：

```text
pong
```

---

### GET /health

网关自身健康状态。

```bash
curl -i http://127.0.0.1:9000/health
```

响应：

```text
ok
```

---

### GET /version

版本信息。

```bash
curl -i http://127.0.0.1:9000/version
```

响应：

```text
go-mini-gateway v0.1.0
```

---

### 业务代理路径

由配置里的 `routes[].prefix` 决定，例如：

```json
"prefix": "/api/",
"stripPrefix": "/api"
```

请求：

```bash
curl -i 'http://127.0.0.1:9000/api/hello?name=sw'
```

转发到后端：

```text
/hello?name=sw
```

Gateway 会添加或透传：

| Header | 说明 |
| --- | --- |
| `X-Gateway` | 固定为 `go-mini-gateway` |
| `X-Gateway-Route` | 当前路由 ID |
| `X-Gateway-Upstream` | 当前选择的 upstream ID |
| `X-Trace-ID` | 客户端传入则透传，否则自动生成 |
| `X-Forwarded-For` | 代理客户端地址 |
| `X-Forwarded-Host` | 原始 Host |
| `X-Forwarded-Proto` | 原始协议 |

---

## admin server

`/admin/*` 接口都需要：

```http
X-Admin-Token: <token>
```

### GET /admin/routes

查看路由、upstream、健康状态、被动健康检查状态和熔断器状态。

```bash
curl -s \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/routes
```

返回示例：

```json
[
  {
    "id": "demo",
    "prefix": "/api/",
    "stripPrefix": "/api",
    "target": "http://localhost:8081",
    "upstreams": [
      {
        "id": "backend-1",
        "url": "http://localhost:8081",
        "activeHealth": {
          "checked": true,
          "healthy": true
        },
        "passiveHealth": {
          "enabled": true,
          "healthy": true,
          "available": true
        },
        "circuitBreaker": {
          "enabled": true,
          "state": "closed",
          "available": true
        }
      }
    ]
  }
]
```

---

### GET /admin/health

查看 route/upstream 主动健康检查聚合状态。

```bash
curl -s \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/health
```

---

### GET /admin/stats

查看当前限流器、并发限制器、健康检查等运行状态。

```bash
curl -s \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/stats
```

---

### GET /admin/metrics

查看 JSON 格式 metrics 快照。

```bash
curl -s \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/metrics
```

---

### POST /admin/reload

重新加载配置文件。

```bash
curl -X POST \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/reload
```

成功响应：

```json
{
  "success": true,
  "message": "config reloaded",
  "reloadedAt": "2026-06-12T19:05:00.123456+08:00",
  "routes": []
}
```

失败响应：

```json
{
  "success": false,
  "message": "load config failed: ...",
  "reloadedAt": "2026-06-12T19:05:00.123456+08:00"
}
```

失败时旧配置继续生效。

---

### GET /metrics

Prometheus 文本格式指标。

```bash
curl -s http://127.0.0.1:9001/metrics
```

如果 `admin.metricsRequireToken=true`，则该接口也需要 `X-Admin-Token`。

---

### GET /debug/pprof/*

pprof 性能分析接口。需要 `admin.pprofEnabled=true`，并且需要 admin token。

常用命令：

```bash
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/debug/pprof/
```

采集 CPU profile：

```bash
curl -o cpu.pb.gz \
  -H 'X-Admin-Token: dev-secret' \
  'http://127.0.0.1:9001/debug/pprof/profile?seconds=30'

go tool pprof -http=:0 cpu.pb.gz
```
