# BUG 005：临时回执故障会污染后续重试

确认订阅时如果通知回执暂时写入失败，后续重试即使故障已经恢复，仓储仍会重复返回上一次的错误，导致订阅无法确认。

复现命令：

```bash
GOTOOLCHAIN=local GOCACHE=/go-cache go test ./internal/application -run '^TestTransientReceiptFailureDoesNotPoisonNextConfirmation$' -count=20
```
