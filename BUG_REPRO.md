# Bug 009 复现说明

## 基线

`origin/green_base_bug_009`

## 涉及文件

- `internal/store/deploymentstore.go`
- `internal/service/deployservice.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

部署流程读取不存在的目标时吞掉存储层错误并把目标指针设为 nil，随后继续访问目标字段，形成跨存储层和服务层的数据流断裂并触发 nil 指针解引用。错误既没有被分类，也没有传回调用方。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug009_MissingDeploymentTargetReturnsError$'
```

## 预期结果

目标不存在时不发生 panic，服务层返回可识别的 not found 错误。

## 基线缺陷表现

服务层在错误路径上继续解引用 nil 目标，进程进入 panic；调用方无法获得正常错误结果。

## 当前工作区结果

存储层提供明确的目标不存在错误，服务层保留并传播错误，复现命令应稳定通过。
