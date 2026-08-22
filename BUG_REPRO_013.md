# BUG 013：已有订阅确认时回执没有生成

普通流程已经写入订阅状态后，再提交确认请求。确认接口返回成功，但仓储层看到已有状态后直接结束，通知回执没有落库。

```bash
GOTOOLCHAIN=local go test ./internal/application -run '^TestConfirmAddsReceiptWhenSubscriptionAlreadyExists$' -count=20
```
