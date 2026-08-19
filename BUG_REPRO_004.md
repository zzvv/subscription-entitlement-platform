# BUG 004：等待仓储锁期间取消的订阅请求仍会落库

`Store.LoadOrStore` 只在进入等待前检查一次 `context.Context`。请求在等待互斥锁时被取消，锁释放后仍可能继续写入订阅状态并返回成功。

复现命令：

```bash
GOTOOLCHAIN=local GOCACHE=/go-cache go test ./internal/repository -run '^TestLoadOrStoreDoesNotPersistWhenContextCancelsWhileWaitingForLock$' -count=20
```
