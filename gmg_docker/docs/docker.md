# Docker 使用说明

本项目提供 Dockerfile 与 docker-compose.yml，可以一键启动：

```text
gateway
backend1
backend2
```

---

## 启动

```bash
make docker-up
```

等价于：

```bash
docker compose up --build
```

默认 admin token：

```text
dev-token
```

覆盖 token：

```bash
GATEWAY_ADMIN_TOKEN=your-token make docker-up
```

---

## 端口

| 服务 | 容器端口 | 宿主机端口 | 说明 |
| --- | ---: | ---: | --- |
| gateway main | 9000 | 9000 | 业务入口 |
| gateway admin | 9001 | 9001 | 管理、metrics、pprof |
| backend1 | 8081 | 8081 | 示例后端 1 |
| backend2 | 8082 | 8082 | 示例后端 2 |

---

## 验证

```bash
make docker-smoke
```

或手动执行：

```bash
curl -i http://127.0.0.1:9000/ping
curl -i 'http://127.0.0.1:9000/api/hello?name=docker'
curl -i -H 'X-Admin-Token: dev-token' http://127.0.0.1:9001/admin/routes
curl -s http://127.0.0.1:9001/metrics | head
```

多次请求业务接口，可以看到 `X-Gateway-Upstream` 在 `backend-1` 和 `backend-2` 之间轮询。

---

## Docker 专用配置

Compose 使用：

```text
configs/gateway.docker.json
```

其中 upstream 使用 service name：

```json
"upstreams": [
  {
    "id": "backend-1",
    "url": "http://backend1:8081"
  },
  {
    "id": "backend-2",
    "url": "http://backend2:8082"
  }
]
```

不要写成 `localhost:8081`。在容器里，`localhost` 指 gateway 容器自身。

admin 监听地址在 Docker 中使用：

```json
"addr": ":9001"
```

这样宿主机端口映射才能访问。

---

## 热加载

修改 `configs/gateway.docker.json` 后执行：

```bash
make docker-reload
```

或：

```bash
curl -X POST \
  -H 'X-Admin-Token: dev-token' \
  http://127.0.0.1:9001/admin/reload
```

注意：容器里的配置文件来自镜像内的 `/app/configs/gateway.docker.json`。如果你希望运行中修改宿主机配置并 reload，需要额外挂载配置文件到容器。

示例：

```yaml
volumes:
  - ./configs/gateway.docker.json:/app/configs/gateway.docker.json:ro
```

---

## 构建参数

Dockerfile 支持：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `GO_VERSION` | `1.26` | Go 镜像版本 |
| `ALPINE_MIRROR` | `https://mirrors.aliyun.com/alpine` | Alpine apk 镜像源 |
| `GOPROXY` | `https://goproxy.cn,direct` | Go module proxy |
| `APP` | `gateway` | 构建目标：`gateway`、`backend`、`backend2` |

示例：

```bash
GO_VERSION=1.26 \
ALPINE_MIRROR=https://mirrors.aliyun.com/alpine \
GOPROXY=https://goproxy.cn,direct \
make docker-build
```

---

## 构建卡在 apk add

如果构建长时间卡在：

```text
RUN apk add --no-cache ca-certificates tzdata
```

通常是 Alpine 源访问慢。当前 Dockerfile 默认使用阿里云 Alpine 源。也可以覆盖：

```bash
ALPINE_MIRROR=https://mirrors.aliyun.com/alpine make docker-build
```

---

## 构建卡在 go mod download

通常是 Go module 下载慢。可以覆盖：

```bash
GOPROXY=https://goproxy.cn,direct make docker-build
```

---

## 查看状态

```bash
make docker-ps
make docker-logs
```

---

## 停止

```bash
make docker-down
```
