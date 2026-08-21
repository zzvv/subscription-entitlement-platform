# BUG 011：取消计划变更后缓存与状态分裂

计划变更先删除权益详情缓存，再保存新的订阅状态。如果保存阶段 context 被取消，持久化状态仍保持旧值，但缓存已经被删除，后续请求会观察到不一致的读取路径。

```bash
GOTOOLCHAIN=local go test ./internal/application -run '^TestCancelledPlanChangeDoesNotLeaveCacheInvalidated$' -count=20
```
