# 公开复现

计划变更会先删除详情缓存，再写入订阅状态。持久化写入失败时，旧状态仍在，但旧缓存已经被删掉，导致一次失败操作改变了跨层可见状态。

执行：`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-019-rerun-cache go test ./internal/application -run '^TestPlanChangeLeavesCachedDetailWhenPersistenceFails$' -count=20`。
BUG BASE 稳定失败，修复后应稳定通过。
