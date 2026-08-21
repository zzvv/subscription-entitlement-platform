# BUG 014：相同订阅编号跨租户覆盖通知回执

订阅编号在不同租户中可以重复，但回执存储只使用订阅编号和动作作为键。两个租户分别确认同一个编号后，后一个租户会覆盖前一个租户的回执。

```bash
GOTOOLCHAIN=local go test ./internal/application -run '^TestConfirmKeepsReceiptsIsolatedWhenTenantsReuseSubscriptionID$' -count=20
```
