# Bug 003 复现说明

## 基线

`origin/green_base_bug_003`

## 涉及文件

- `internal/service/deployservice.go`
- `internal/api/handler_deploy.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

部署服务在结构体中保存验证结果映射，但构造函数没有初始化该 map。API 层把验证结果传入 `RememberVerification` 后，服务层直接向 nil map 写入，触发运行时崩溃。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug003_RememberVerificationDoesNotCrash$'
```

## 预期结果

验证结果可以被服务安全记录，调用过程不发生 panic。

## 缺陷表现

执行写入时触发 `assignment to entry in nil map`，测试捕获到 panic 并失败。
