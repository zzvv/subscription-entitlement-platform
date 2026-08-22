# BUG 006：并发确认会重复生成通知并覆盖订阅状态

两个操作员同时确认同一租户下的同一订阅范围时，提交路径没有把已存在的订阅视为幂等结果。两个请求都会写入自己的通知回执，后完成的请求还会覆盖先完成的订阅状态。

复现命令：

```bash
GOTOOLCHAIN=local GOCACHE=/go-cache go test ./internal/application -run '^TestConcurrentConfirmationsKeepOneSubscriptionStateAndReceipt$' -count=20
```
