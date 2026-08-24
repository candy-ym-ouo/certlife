# Bug 005 复现说明

## 基线

`origin/green_base_bug_005`

## 涉及文件

- `internal/store/certstore.go`
- `internal/service/certservice.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

证书存储层和服务层都在结构体中缓存并复用列表切片。并发分页查询会同时改写共享底层数组，使不同请求的证书列表互相覆盖，并产生稳定的数据竞争和跨层状态污染。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug005_ConcurrentCertificateListsAreIsolated$'
```

## 预期结果

每个请求获得独立切片，分页数量和总数保持一致，race detector 不报告冲突。

## 基线缺陷表现

并发列表结果可能被其他请求覆盖，返回数量异常，或由 race detector 报告共享切片竞争。

## 当前工作区结果

存储层直接返回本次查询创建的切片，服务层不再复制到共享缓存，复现命令应稳定通过。
