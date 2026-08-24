# Bug 002 复现说明

## 基线

`origin/green_base_bug_002`

## 涉及文件

- `internal/store/notificationstore.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

`NotificationStore.Pending` 把查询结果写入结构体上的共享切片。多个 goroutine 并发查询不同数量的待发送通知时，会同时重用和改写同一底层数组，造成数据竞争、返回结果互相覆盖以及跨请求状态污染。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug002_ConcurrentPendingReadsAreIsolated$'
```

## 预期结果

每次查询都返回独立结果，所有并发调用稳定完成，race detector 不报告共享内存访问冲突。

## 缺陷表现

并发结果可能被其他查询覆盖、出现异常空结果，或由 race detector 报告共享切片读写冲突。
