# 故障排查

## Docker build 卡在 apk add

现象：

```text
RUN apk add --no-cache ca-certificates tzdata
```

耗时很久。

原因通常是 Alpine 官方源访问慢。当前 Dockerfile 已支持：

```dockerfile
ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
```

也可以构建时覆盖：

```bash
docker compose build --build-arg ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
```

如果卡在 `go mod download`，可以设置：

```bash
docker compose build --build-arg GOPROXY=https://goproxy.cn,direct
```

---

## Docker 中 gateway 访问不到 backend

在 Docker Compose 网络里，不要使用：

```text
http://localhost:8081
```

`localhost` 指的是 gateway 容器自己。

应该使用 service name：

```text
http://backend1:8081
http://backend2:8082
```

Docker 专用配置文件是：

```text
configs/gateway.docker.json
```

---

## admin 端口宿主机访问不到

本地运行可使用：

```json
"addr": "127.0.0.1:9001"
```

Docker 容器内建议使用：

```json
"addr": ":9001"
```

否则容器可能只监听容器内 loopback，宿主机端口映射访问不到。

---

## backend1 挂了导致整个 route 不通

如果出现这个现象，检查是否使用了旧版本 route 级 health check。

当前正确模型是 upstream 级主动健康检查：

```text
backend1 不通，只跳过 backend1
backend2 继续可用
```

验证：

```bash
curl -s -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/routes
```

查看每个 upstream 的 `activeHealth`。

---

## 请求大量 429

说明 RPS 限流生效。

检查配置：

```json
"rateLimitRPS": 100,
"rateLimitBurst": 100
```

查看 metrics：

```bash
curl -s http://127.0.0.1:9001/metrics | grep 'status="429"'
```

---

## 请求大量 503

常见原因：

1. 并发限制触发：`concurrency limit exceeded`
2. 所有 upstream 都不可用：`no available upstream`
3. active health 全部 unhealthy
4. passive health 摘除了所有 upstream
5. circuit breaker 全部 open

检查：

```bash
curl -s -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/routes
curl -s -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/health
curl -s -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/stats
```

---

## 请求大量 504

说明 Gateway 等后端响应超时。

检查：

```json
"requestTimeout": "3s"
```

如果后端接口本身慢于该值，会返回 504。

---

## reload 失败

执行：

```bash
curl -X POST -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/admin/reload
```

如果返回 400，说明新配置加载失败或修改了不可热加载字段。

当前不可热加载：

- `server.addr`
- `server.shutdownTimeout`
- `admin.enabled`
- `admin.addr`
- `admin.token`
- `admin.metricsRequireToken`
- `admin.pprofEnabled`

失败时旧配置仍然生效。

---

## pprof 访问 404

检查配置：

```json
"pprofEnabled": true
```

检查是否访问 admin server，而不是 main server：

```bash
curl -H 'X-Admin-Token: dev-secret' http://127.0.0.1:9001/debug/pprof/
```

---

## pprof 访问 401

pprof 也套了 admin token 鉴权，需要加：

```http
X-Admin-Token: <token>
```

---

## 压测 QPS 很低

可能原因：

- 访问日志 IO 成为瓶颈
- 后端成为瓶颈
- hey 客户端成为瓶颈
- Docker Desktop 网络开销
- `MaxConnsPerHost` 限制
- CPU 被打满

建议同时采集 CPU profile：

```bash
curl -o cpu.pb.gz \
  -H 'X-Admin-Token: dev-secret' \
  'http://127.0.0.1:9001/debug/pprof/profile?seconds=30'

go tool pprof -http=:0 cpu.pb.gz
```
