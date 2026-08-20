# 订阅变更后权益详情仍为旧缓存

材料详情会缓存订阅权益投影。订阅计划变更成功后，下一次详情查询必须读取新的订阅状态，不能继续显示变更前的计划。

复现与回归命令：

```bash
GOTOOLCHAIN=local GOCACHE=/go-cache go test ./internal/application -run '^TestPlanChangeInvalidatesCachedEntitlementDetail$' -count=20
```
