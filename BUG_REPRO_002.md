# 公开复现

当订阅写入流程在 context 已进入取消状态后继续到达仓储提交阶段，仓储不得写入新的订阅状态。当前 `Store.Save` 只在获取写锁前检查一次 context；context 在检查后取消时，状态仍会被提交并造成后续请求观察到已取消操作的结果。

在仓库根目录执行：

`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-002-gocache go test ./internal/repository -run '^TestStoreDoesNotCommitAfterContextCancellation$' -count=20`

BUG BASE 预期稳定失败；修复后预期稳定通过。
