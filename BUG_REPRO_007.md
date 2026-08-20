# 权益详情缓存串租复现

材料详情会读取订阅权益投影，并将结果保存在查询缓存中。两个租户拥有相同订阅范围时，先后读取各自详情，后一个租户必须仍看到自己的订阅记录，不能复用另一租户的缓存内容。

复现与回归命令：

```bash
GOTOOLCHAIN=local GOCACHE=/go-cache go test ./internal/application -run '^TestEntitlementDetailKeepsCachedProjectionScopedToTenant$' -count=20
```
