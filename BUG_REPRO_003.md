# 公开复现

订阅确认流程需要同时保存订阅状态和通知回执。若回执存储失败，操作必须返回错误且状态与回执都不可见；否则后续读取会观察到没有对应通知回执的半截订阅状态。

在仓库根目录执行：

`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-003-gocache go test ./internal/application -run '^TestConfirmLeavesNoPartialStateWhenReceiptCommitFails$' -count=20`

BUG BASE 预期稳定失败；修复后预期稳定通过。
