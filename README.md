# CertLife — 证书生命周期服务（Certificate Lifecycle Service）

> 用 Go 构建证书生命周期服务：登记证书元信息，执行到期检查、续期任务、部署验证和失败通知；分层设计、数据库迁移与持久化、API、后台任务、测试及完整验证。

## 一、项目定位

CertLife 是一个面向 TLS/SSL 证书全生命周期管理的后端服务。它负责：

1. **登记证书元信息**：证书主体、SAN、签发者、有效期、算法、归属人、环境、部署目标等。
2. **到期检查**：定时扫描证书剩余有效期，自动推进证书状态（有效 / 即将到期 / 已过期）。
3. **续期任务**：按策略自动或手动触发续期，走完"申请签发 → 下发 → 部署 → 验证"流水线。
4. **部署验证**：把新证书部署到目标主机后，通过真实 TLS 握手校验序列号、SAN 覆盖与有效期。
5. **失败通知**：到期告警、续期失败、部署失败、验证失败通过邮件/Webhook 通知，带重试与去重。
6. **配套能力**：REST API、简单 Web 前端、审计日志、仪表盘统计、后台任务手动触发、数据库迁移与持久化。

## 二、硬性约束指标（后续实现必须满足）

| 指标 | 要求 | 说明 |
|------|------|------|
| Go 代码行数（不含测试） | 大于 2000 且小于 2200 | 以 `scripts/count_lines.sh` 统计为准，仅统计 `*.go` 且排除 `*_test.go` |
| Go 代码文件数 | 大于 20 且小于 25（即 21–24） | 不含测试文件、不含前端文件 |
| 前端 | 简单界面 | 原生 HTML/CSS/JS，无构建步骤 |
| 分层 | 分层设计 | config / model / store / service / task / api / tlsutil |
| 数据库 | 迁移 + 持久化 | SQLite，启动时自动执行内嵌迁移 |
| 测试与验证 | 完整验证 | 单元 / 集成 / API / 任务测试 + 端到端验证脚本 |

> 行数与文件数的精确预算见 [docs/08-代码结构与行数预算.md](docs/08-代码结构与行数预算.md)。

## 三、功能一览

- 证书登记、编辑、删除、列表（关键字 / 状态 / 环境 / 标签过滤，分页）
- 证书详情：元信息、剩余天数、状态、续期历史、部署记录、通知记录、审计轨迹
- 到期检查任务：自动推进状态、产生"即将到期 / 已过期"告警
- 续期：自动续期（策略）+ 手动续期（API / 界面触发），可重试
- 部署与验证：多目标部署、TLS 握手校验、指纹 / SAN / 有效期校验、失败回滚
- 通知：邮件（SMTP）与 Webhook 双通道、按事件类型配置规则、重试与去重、日报
- 审计日志：所有关键变更留痕
- 仪表盘统计：证书总数、状态分布、近 7 天到期、任务执行概况
- 后台任务：5 个定时任务 + 手动触发接口 `POST /api/v1/tasks/run`
- 简单前端：仪表盘、证书列表/详情、续期、部署、通知规则页面

## 四、技术栈

| 层面 | 选型 |
|------|------|
| 语言 | Go 1.22+ |
| HTTP | 标准库 `net/http` + 自研轻量路由 |
| 数据库 | SQLite（`modernc.org/sqlite`，纯 Go，免 CGO） |
| 迁移 | 自研迁移执行器（内嵌 SQL + `schema_migrations` 版本表） |
| 定时任务 | 自研调度器（`time.Ticker` 驱动的任务注册表） |
| 证书签发 | `Issuer` 接口 + Mock 签发器（本地验证用，可替换为 ACME 实现） |
| 前端 | 原生 HTML/CSS/JS，由 Go `embed` 提供静态资源 |
| 测试 | 标准库 `testing` + `httptest` |

## 五、快速开始（实现完成后）

```bash
# 1. 构建并启动
go build -o bin/certlife ./cmd/certlife
./bin/certlife -config config.yaml

# 2. 注册一张测试证书
curl -X POST http://127.0.0.1:8080/api/v1/certificates \
  -H "X-API-Key: dev-key" -H "Content-Type: application/json" \
  -d '{"name":"api.example.com","domain":"api.example.com","valid_from":"2024-01-01T00:00:00Z","valid_until":"2024-01-08T00:00:00Z",...}'

# 3. 手动触发到期检查
curl -X POST http://127.0.0.1:8080/api/v1/tasks/run \
  -H "X-API-Key: dev-key" -H "Content-Type: application/json" \
  -d '{"name":"expiry_checker"}'

# 4. 端到端验证
bash scripts/verify.sh
```

## 六、目录结构（规划）

```
certlife/
├── go.mod
├── config.yaml                  # 示例配置
├── Makefile
├── README.md
├── docs/                        # 项目文档（本目录）
│   ├── 01-需求与业务逻辑.md
│   ├── 02-架构设计.md
│   ├── 03-数据库设计.md
│   ├── 04-API设计.md
│   ├── 05-后台任务设计.md
│   ├── 06-前端设计.md
│   ├── 07-测试与验证方案.md
│   ├── 08-代码结构与行数预算.md
│   └── 09-实施计划.md
├── cmd/
│   └── certlife/main.go         # 程序入口
├── internal/
│   ├── config/                  # 配置加载
│   ├── model/                   # 领域模型与状态机
│   ├── store/                   # 持久化（仓储 + 迁移）
│   ├── service/                 # 业务逻辑层
│   ├── task/                    # 后台任务与调度器
│   ├── api/                     # HTTP 路由与处理器
│   └── tlsutil/                 # 证书解析与 TLS 校验工具
├── web/                         # 简单前端（embed 提供）
│   ├── index.html
│   ├── app.js
│   └── style.css
├── sql/migrations/              # 迁移 SQL（与内嵌迁移保持一致，供人工审查）
├── scripts/
│   ├── count_lines.sh           # 行数/文件数约束校验
│   └── verify.sh                # 端到端验证脚本
└── e2e/                         # 端到端测试（可选，Go）
```

## 七、文档索引

| 文档 | 内容 |
|------|------|
| [docs/01-需求与业务逻辑.md](docs/01-需求与业务逻辑.md) | 功能需求、业务规则、状态机、非功能需求 |
| [docs/02-架构设计.md](docs/02-架构设计.md) | 分层架构、依赖方向、关键流程时序 |
| [docs/03-数据库设计.md](docs/03-数据库设计.md) | 表结构、字段、迁移策略 |
| [docs/04-API设计.md](docs/04-API设计.md) | 全部 REST 端点定义与示例 |
| [docs/05-后台任务设计.md](docs/05-后台任务设计.md) | 调度器与 5 个任务细节 |
| [docs/06-前端设计.md](docs/06-前端设计.md) | 页面与交互设计 |
| [docs/07-测试与验证方案.md](docs/07-测试与验证方案.md) | 测试分层、端到端验证、验收清单 |
| [docs/08-代码结构与行数预算.md](docs/08-代码结构与行数预算.md) | 24 个 Go 文件、2175 行的精确预算 |
| [docs/09-实施计划.md](docs/09-实施计划.md) | 实施阶段与里程碑 |
