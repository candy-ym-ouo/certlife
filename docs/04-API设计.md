# 04 · API 设计

## 1. 通用约定

- Base URL：`/api/v1`；前缀 `X-API-Key: <key>` 鉴权（401 未携带/错误）。
- 请求/响应：`application/json`；时间一律 RFC3339 UTC。
- 统一响应结构：

```json
{ "code": 0, "message": "ok", "data": { } }
```

- 错误码映射：

| HTTP | 业务场景 |
|------|----------|
| 400 | 参数校验失败（`code=40001`）、无效日期（`code=40002`） |
| 401 | API Key 缺失或错误 |
| 404 | 资源不存在（`code=40401`） |
| 409 | 进行中续期冲突（`code=40901`）、重复操作（`code=40902`） |
| 500 | 内部错误（`code=50000`） |

- 列表统一返回：`{ "data": [...], "total": N, "page": 1, "size": 20 }`。
- 幂等：`POST /tasks/run`、续期触发、到期检查均幂等或可重入。

## 2. 端点总表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/health` | 健康检查 |
| GET | `/api/v1/stats` | 仪表盘统计 |
| POST | `/api/v1/certificates` | 登记证书 |
| GET | `/api/v1/certificates` | 证书列表（过滤+分页） |
| GET | `/api/v1/certificates/{id}` | 证书详情 |
| PUT | `/api/v1/certificates/{id}` | 更新证书 |
| DELETE | `/api/v1/certificates/{id}` | 软删除证书 |
| POST | `/api/v1/certificates/{id}/renew` | 触发续期 |
| GET | `/api/v1/certificates/{id}/renewals` | 证书续期历史 |
| GET | `/api/v1/certificates/{id}/deployments` | 证书部署记录 |
| GET | `/api/v1/certificates/{id}/notifications` | 证书通知记录 |
| GET | `/api/v1/renewals` | 续期列表 |
| POST | `/api/v1/renewals/{id}/retry` | 重试失败续期 |
| GET | `/api/v1/deployments` | 部署记录列表 |
| GET | `/api/v1/targets` | 部署目标列表 |
| POST | `/api/v1/targets` | 新建部署目标 |
| PUT | `/api/v1/targets/{id}` | 更新部署目标 |
| DELETE | `/api/v1/targets/{id}` | 删除部署目标 |
| GET | `/api/v1/notifications` | 通知记录列表 |
| GET | `/api/v1/notification-rules` | 规则列表 |
| POST | `/api/v1/notification-rules` | 新建规则 |
| PUT | `/api/v1/notification-rules/{id}` | 更新规则 |
| DELETE | `/api/v1/notification-rules/{id}` | 删除规则 |
| GET | `/api/v1/audit-logs` | 审计日志列表 |
| GET | `/api/v1/tasks` | 任务列表与最近执行结果 |
| POST | `/api/v1/tasks/run` | 手动触发任务 |
| GET | `/web/*` | 前端静态资源 |

## 3. 端点详述

### 3.1 GET /api/v1/health

```json
{ "code":0, "message":"ok", "data": { "status":"up", "db":"ok", "tasks_running":2 } }
```

### 3.2 POST /api/v1/certificates（登记）

请求体：

```json
{
  "name": "api-gateway",
  "domain": "api.example.com",
  "sans": ["api.example.com", "v2.api.example.com"],
  "issuer": "Internal CA",
  "serial_number": "1A2B3C4D",
  "algorithm": "ECDSA",
  "key_bits": 256,
  "valid_from": "2024-01-01T00:00:00Z",
  "valid_until": "2024-12-31T00:00:00Z",
  "owner": "sre@example.com",
  "environment": "prod",
  "tags": ["gateway", "tls"],
  "contacts": ["sre@example.com", "oncall@example.com"],
  "notify_threshold_days": 30,
  "auto_renew": true,
  "renew_threshold_days": 14,
  "deploy_target_ids": [1]
}
```

响应 `201`：`data` 为完整 Certificate（含计算出的 `status`、`days_to_expire`）。
校验失败 `400`；`valid_from >= valid_until` → `400`。

### 3.3 GET /api/v1/certificates

Query 参数：`q`（关键字，匹配 name/domain/issuer）、`status`、`environment`、`tag`、`page`（默认 1）、`size`（默认 20，上限 100）。

### 3.4 GET /api/v1/certificates/{id}

返回证书 + 冗余聚合：`active_renewal`（进行中续期）、`next_expiry_days`。

### 3.5 PUT /api/v1/certificates/{id}

全量更新（除 `id/created_at`）。更新后重算 `status/days_to_expire`，写审计。

### 3.6 DELETE /api/v1/certificates/{id}

软删除，`204`。审计 `cert.delete`。

### 3.7 POST /api/v1/certificates/{id}/renew

请求体（可选）：`{ "force": true }`。
- 无进行中续期 → 创建 `pending` 续期，`202` 返回续期对象；
- 已存在进行中续期 → `409`（`code=40901`）；
- 证书不存在 → `404`；
- 仅当 `auto_renew=false` 且 `force=false` 且剩余天数 > 阈值时返回 `400`（"未到续期窗口"）。

### 3.8 列表类端点

- `GET /certificates/{id}/renewals?page=&size=` → Renewal[]
- `GET /certificates/{id}/deployments?page=&size=` → Deployment[]（含 verification）
- `GET /certificates/{id}/notifications?page=&size=` → Notification[]
- `GET /renewals?status=&certificate_id=&page=&size=`
- `GET /deployments?status=&renewal_id=&page=&size=`
- `GET /notifications?status=&event_type=&certificate_id=&page=&size=`
- `GET /audit-logs?action=&resource_type=&resource_id=&page=&size=`

### 3.9 POST /api/v1/renewals/{id}/retry

仅 `failed` 状态可重试 → 重置 `pending`、`attempt+1`，`202`；否则 `409`。

### 3.10 targets

- `POST /api/v1/targets`：`{name, host, port, scheme, method, verify_tls, enabled}` → `201`；
- `PUT /api/v1/targets/{id}`、`DELETE /api/v1/targets/{id}`（删除前检查是否被证书引用，被引用则 `409`）。

### 3.11 notification-rules

- `POST /api/v1/notification-rules`：`{name, event_types[], environments[], channels[], recipients[], webhook_url, enabled}` → `201`；
- 校验：channel 含 `webhook` 时 `webhook_url` 必填；channel 含 `email` 时 `recipients` 非空。

### 3.12 GET /api/v1/stats

```json
{ "code":0, "message":"ok", "data": {
  "total": 12, "by_status": {"valid":8,"expiring_soon":3,"expired":1},
  "expiring_next_7d": [ {"id":1,"name":"api-gateway","days_to_expire":3} ],
  "expiring_next_30d": 5,
  "renewals_30d": {"total":4,"succeeded":3,"failed":1},
  "notifications_today": {"sent":6,"failed":0}
}}
```

### 3.13 GET /api/v1/tasks

```json
{ "code":0, "message":"ok", "data": [
  { "name":"expiry_checker", "interval":"6h0m0s", "last_run_at":"...", "last_result":"ok", "last_error":"", "runs":42 }
]}
```

### 3.14 POST /api/v1/tasks/run

请求体：`{ "name": "expiry_checker" }`。
- 任务存在 → 立即执行（同步等待完成），返回执行结果 `{ "name": "...", "result": "ok", "detail": "scanned=12 valid=8 expiring=3 expired=1" }`；
- 任务不存在 → `404`；
- 任务正在运行 → `409`（`code=40903`）。

## 4. 认证与中间件

- `X-API-Key` 与配置 `api_key` 比对（常量时间比较）；失败返回 401。
- 中间件链：`recover`（500 + 错误日志）→ `accessLog`（slog：method/path/status/duration）→ `auth`（除 `/api/v1/health` 与 `/web/*`）。
- Webhook 出站签名：`X-CertLife-Sign: sha256=<hex>`（HMAC-SHA256 of body，密钥来自配置），供回调方校验。

## 5. 前端页面 ↔ API 映射（简表）

| 页面 | 主要调用 |
|------|----------|
| 仪表盘 | `GET /stats`、`GET /certificates?status=expiring_soon` |
| 证书列表 | `GET /certificates`、`POST /certificates`、`DELETE /certificates/{id}` |
| 证书详情 | `GET /certificates/{id}`、`POST /certificates/{id}/renew`、`GET .../renewals`、`GET .../deployments`、`GET .../notifications` |
| 部署目标 | `GET/POST/PUT/DELETE /targets` |
| 通知规则 | `GET/POST/PUT/DELETE /notification-rules`、`GET /notifications` |
| 任务页 | `GET /tasks`、`POST /tasks/run` |
