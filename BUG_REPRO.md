# Bug 010 复现说明

## 基线

`origin/green_base_bug_010`

## 涉及文件

- `internal/service/stats.go`
- `internal/store/certstore.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

统计服务把包含 map、slice 和计数器的 `Stats` 缓存在服务结构体中，并让所有 Dashboard 请求共同修改。并发请求会产生数据竞争，聚合值在请求之间累加，返回对象也会继续受到其他请求写入影响。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug010_ConcurrentDashboardRequestsAreIsolated$'
```

## 预期结果

每次 Dashboard 调用使用独立聚合状态，所有请求都返回准确且一致的统计结果，race detector 不报告冲突。

## 基线缺陷表现

并发响应共享 map 和计数器，统计值出现累加或覆盖，并由 race detector 报告并发读写。

## 当前工作区结果

每次请求创建新的 `Stats` 值及其内部集合，复现命令应稳定通过。
