# Metrics 说明

项目提供两类 metrics：

| 接口 | 格式 | 用途 |
| --- | --- | --- |
| `/admin/metrics` | JSON | 本地调试、接口查看 |
| `/metrics` | Prometheus text | Prometheus 抓取 |

---

## JSON metrics

```bash
curl -s \
  -H 'X-Admin-Token: dev-secret' \
  http://127.0.0.1:9001/admin/metrics
```

主要结构：

```json
{
  "uptime": "1m23s",
  "uptimeSeconds": 83.12,
  "total": {
    "requests": 100,
    "bytesWritten": 1024,
    "avgLatencyMs": 1.2,
    "maxLatencyMs": 20.5,
    "statusCodes": {
      "200": 98,
      "503": 2
    },
    "durationHistogram": {
      "buckets": {
        "0.001": 10,
        "0.005": 80,
        "+Inf": 100
      },
      "sumSeconds": 0.12,
      "count": 100
    }
  },
  "routes": {
    "demo": {}
  }
}
```

---

## Prometheus 指标

```bash
curl -s http://127.0.0.1:9001/metrics
```

### 进程运行时间

```text
gateway_uptime_seconds
```

### 全局请求数

```text
gateway_http_requests_total
```

### 全局响应字节数

```text
gateway_http_bytes_written_total
```

### 全局状态码统计

```text
gateway_http_status_requests_total{status="200"}
gateway_http_status_requests_total{status="503"}
```

### route 请求数

```text
gateway_route_http_requests_total{route="demo"}
```

### route 响应字节数

```text
gateway_route_http_bytes_written_total{route="demo"}
```

### route 状态码统计

```text
gateway_route_http_status_requests_total{route="demo",status="200"}
gateway_route_http_status_requests_total{route="demo",status="429"}
gateway_route_http_status_requests_total{route="demo",status="503"}
gateway_route_http_status_requests_total{route="demo",status="504"}
```

### 延迟平均值和最大值

```text
gateway_http_request_duration_avg_milliseconds
gateway_http_request_duration_max_milliseconds
gateway_route_http_request_duration_avg_milliseconds{route="demo"}
gateway_route_http_request_duration_max_milliseconds{route="demo"}
```

这些指标便于本地快速观察，但分位数分析应优先使用 Histogram。

---

## Histogram

全局 Histogram：

```text
gateway_http_request_duration_seconds_bucket{le="0.001"}
gateway_http_request_duration_seconds_bucket{le="0.005"}
gateway_http_request_duration_seconds_bucket{le="0.01"}
gateway_http_request_duration_seconds_bucket{le="0.025"}
gateway_http_request_duration_seconds_bucket{le="0.05"}
gateway_http_request_duration_seconds_bucket{le="0.1"}
gateway_http_request_duration_seconds_bucket{le="0.25"}
gateway_http_request_duration_seconds_bucket{le="0.5"}
gateway_http_request_duration_seconds_bucket{le="1"}
gateway_http_request_duration_seconds_bucket{le="2.5"}
gateway_http_request_duration_seconds_bucket{le="5"}
gateway_http_request_duration_seconds_bucket{le="10"}
gateway_http_request_duration_seconds_bucket{le="+Inf"}
gateway_http_request_duration_seconds_sum
gateway_http_request_duration_seconds_count
```

route 维度 Histogram：

```text
gateway_route_http_request_duration_seconds_bucket{route="demo",le="0.1"}
gateway_route_http_request_duration_seconds_sum{route="demo"}
gateway_route_http_request_duration_seconds_count{route="demo"}
```

当前 bucket 边界：

```text
1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, +Inf
```

---

## PromQL 示例

route 维度 QPS：

```promql
sum by (route) (
  rate(gateway_route_http_requests_total[5m])
)
```

route 维度 5xx 错误率：

```promql
sum by (route) (
  rate(gateway_route_http_status_requests_total{status=~"5.."}[5m])
)
/
sum by (route) (
  rate(gateway_route_http_requests_total[5m])
)
```

route 维度 P95：

```promql
histogram_quantile(
  0.95,
  sum by (route, le) (
    rate(gateway_route_http_request_duration_seconds_bucket[5m])
  )
)
```

route 维度 P99：

```promql
histogram_quantile(
  0.99,
  sum by (route, le) (
    rate(gateway_route_http_request_duration_seconds_bucket[5m])
  )
)
```

全局 P95：

```promql
histogram_quantile(
  0.95,
  rate(gateway_http_request_duration_seconds_bucket[5m])
)
```

状态码分布：

```promql
sum by (status) (
  rate(gateway_http_status_requests_total[5m])
)
```

---

## 当前限制

目前 metrics 还没有直接暴露：

- upstream 级请求数
- upstream 级失败数
- active health 状态指标
- passive health 状态指标
- circuit breaker 状态指标
- reload 成功/失败次数

这些可以作为后续增强方向。
