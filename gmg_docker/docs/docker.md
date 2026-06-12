# Docker usage

## Start all services

```bash
make docker-up
```

This starts:

- `gateway` on `localhost:9000` and admin on `localhost:9001`
- `backend1` on `localhost:8081`
- `backend2` on `localhost:8082`

The default admin token is `dev-token`.

Override it with:

```bash
GATEWAY_ADMIN_TOKEN=your-token make docker-up
```

## Smoke test

```bash
make docker-smoke
```

Or run manually:

```bash
curl -i http://127.0.0.1:9000/ping
curl -i 'http://127.0.0.1:9000/api/hello?name=docker'
curl -i -H 'X-Admin-Token: dev-token' http://127.0.0.1:9001/admin/routes
curl -s http://127.0.0.1:9001/metrics | head
```

## Reload config

After editing `configs/gateway.docker.json`, run:

```bash
make docker-reload
```

or:

```bash
curl -X POST \
  -H 'X-Admin-Token: dev-token' \
  http://127.0.0.1:9001/admin/reload
```

## Stop services

```bash
make docker-down
```
