# Bug 001 复现说明

## 基线

`origin/green_base_bug_001`

## 涉及文件

- `internal/task/scheduler.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

手动触发任务时，`Scheduler.TriggerNow` 没有把调用方传入的 `context.Context` 继续传给任务，而是改用新的后台上下文。调用方取消请求后，下游任务仍会继续执行，取消信号在调度层被截断。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug001_TriggerNowPreservesCancelledContext$'
```

## 预期结果

任务收到已取消的上下文，返回 `context canceled`，且不会继续按正常路径完成。

## 缺陷表现

任务看不到调用方的取消状态并返回成功，测试报告任务结果与已取消状态不符。
