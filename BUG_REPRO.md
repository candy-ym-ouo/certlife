# Bug 007 复现说明

## 基线

`origin/green_base_bug_007`

## 涉及文件

- `internal/service/deployservice.go`
- `internal/tlsutil/tlsutil.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

部署验证调用 TLS 检查时丢弃了上游上下文，改用 `context.Background()`。当远端建立连接后不继续握手，调用方取消验证也无法中断下游网络操作，导致请求长时间挂起并占用 goroutine 和连接。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug007_VerifyStopsWhenContextIsCancelled$'
```

## 预期结果

取消验证上下文后，TLS 检查立即结束并向上返回 `context.Canceled`。

## 基线缺陷表现

下游 TLS 操作继续使用后台上下文，取消信号无法传递，测试等待超时。

## 当前工作区结果

服务层把调用方上下文传给 `VerifyTLSContext`，复现命令应稳定通过。
