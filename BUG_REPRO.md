# 公开复现

BUG BASE 执行：`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-gocache go test ./internal/application -run '^TestEntitlementsAreIsolatedBySubscription$' -count=20`。修复前稳定失败，修复后稳定通过。
