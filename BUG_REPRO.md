# Bug 004 复现说明

## 基线

`origin/green_base_bug_004`

## 涉及文件

- `internal/store/certstore.go`
- `internal/service/certservice.go`
- `internal/bugverification/bug_regression_test.go`

## 问题机制

存储层返回 `sql.ErrNoRows` 后，服务层把它转换为 `ErrNotFound`，但使用 `%v` 拼接错误文本，没有通过 `%w` 保留错误链。上层只能看到字符串，`errors.Is` 无法跨服务边界识别 `service.ErrNotFound`。

## 复现命令

```bash
go test -race ./internal/bugverification -count=20 -run '^TestBug004_NotFoundSurvivesServiceBoundary$'
```

## 预期结果

查询不存在的证书时返回可由 `errors.Is(err, service.ErrNotFound)` 识别的错误。

## 缺陷表现

错误文本包含 not found 信息，但错误链已经丢失，`errors.Is` 返回 false。
