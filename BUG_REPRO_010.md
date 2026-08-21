# BUG 010：取消后的保存仍会写入订阅状态

当请求在应用层继续向仓储层保存订阅状态时，如果 context 在保存的初次检查之后被取消，仓储层仍会把状态写入内存。调用方收到取消错误，但后续查询能看到本不应存在的订阅，形成状态污染。

验证命令：

```bash
GOTOOLCHAIN=local go test ./internal/repository -run '^TestSaveDoesNotWriteAfterCancellationDuringLockWait$' -count=20
```

预期：BUG BASE 稳定失败；修复后通过。
