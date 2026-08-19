# 公开复现

当订阅写入流程的 context 在首次检查后取消时，仓储不得提交新的订阅状态。当前实现会继续写入，导致后续请求观察到已取消操作的结果。

在仓库根目录执行：

`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-002-gocache go test ./internal/repository -run '^TestStoreDoesNotCommitAfterContextCancellation$' -count=20`

BUG BASE 预期稳定失败；修复后预期稳定通过。
