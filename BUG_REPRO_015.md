# BUG 015：并发详情查询会复活已失效的旧缓存

详情查询从仓储读到旧投影后尚未写缓存时，计划变更已保存新计划并删除缓存。旧查询恢复后又把旧投影写回缓存，后续详情会看到过期计划。

```bash
CGO_ENABLED=1 GOTOOLCHAIN=local go test -race ./internal/application -run '^TestPlanChangeDoesNotAllowInFlightDetailToRestoreStaleCache$' -count=20
```
