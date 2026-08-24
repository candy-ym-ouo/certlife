# Bug 006 复现说明

## 基线

`origin/green_base_bug_006`

## 涉及文件

- `internal/service/deployservice.go`
- `internal/task/jobs.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

静态部署验证在循环中打开文件并使用 `defer` 关闭，所有文件描述符都会一直保留到整个验证函数返回。目标数量增加时，打开文件数随循环线性增长，最终可能触发资源耗尽。通知调度的无缓冲结果通道也会在上下文先取消时留下阻塞发送的 goroutine。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug006_StaticVerificationReleasesEachFile$'
```

## 预期结果

每个静态目标处理完成后立即释放文件资源，峰值文件描述符数量不会随目标总数持续增长。

## 基线缺陷表现

循环结束前大量文件描述符保持打开，目标数较大时峰值显著升高，并可能导致 `too many open files`。

## 当前工作区结果

文件在单次迭代内关闭，异步通知结果使用容量为 1 的通道，复现命令应稳定通过。
