# BUG 012：重复确认返回未持久化的订阅

同一租户范围已有订阅时，第二次确认请求被仓储层按幂等语义接受，但应用层仍把第二个请求构造出的订阅返回给客户端。客户端看到的订阅 ID 与后续查询到的已持久化订阅不一致。

```bash
GOTOOLCHAIN=local go test ./internal/application -run '^TestRepeatedConfirmationReturnsPersistedSubscription$' -count=20
```
