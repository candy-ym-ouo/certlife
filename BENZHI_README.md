# CertLife 评测与打包说明

CertLife 是一个 Go 1.22+ 编写的 TLS/SSL 证书生命周期管理服务，提供证书登记、到期检查、续期、部署验证、通知、审计日志、后台任务和简单 Web 界面。数据默认持久化到 SQLite，前端静态资源由 Go `embed` 提供，不需要 Node.js 构建步骤。

## 本机编译与测试

```bash
go mod download
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

## 启动服务

```bash
go run ./cmd/certlife -config config.yaml
```

默认监听地址与开发 API Key 以 `config.yaml` 的示例配置为准。也可用命令行参数覆盖：

```bash
go run ./cmd/certlife \
  -config config.yaml \
  -addr 127.0.0.1:8080 \
  -db certlife.db \
  -api-key dev-key
```

启动后可访问健康检查：

```bash
curl http://127.0.0.1:8080/api/v1/health
```

## 完整验证

```bash
bash scripts/count_lines.sh
bash scripts/verify.sh
```

`scripts/verify.sh` 会构建临时二进制、启动隔离的服务实例，并验证健康检查、证书创建、到期检查、续期、通知、续期记录和审计日志等关键流程。

## Docker 双架构构建

评测镜像保留完整 Go 工具链，并在镜像构建阶段下载依赖和执行 `go build ./...`。

```bash
./build_benzhi_docker.sh certlife linux/arm64
docker run --rm --platform linux/arm64 certlife:latest \
  bash -lc 'go version && go build ./... && go test ./...'

./build_benzhi_docker.sh certlife linux/amd64
docker run --rm --platform linux/amd64 certlife:latest \
  bash -lc 'go version && go build ./... && go test ./...'
```

进入交互式容器：

```bash
docker run --rm -it certlife:latest
```
