# 订阅变更失败后的半完成状态

材料详情已经被缓存时，如果缓存服务在计划变更期间不可用，变更请求应失败且订阅状态和旧详情缓存都必须保持不变，不能出现数据库已改、页面请求却报错的半完成状态。

复现与回归命令：

```bash
GOTOOLCHAIN=local go test ./internal/application -run '^TestPlanChangeLeavesStateAndCacheUntouchedWhenInvalidationFails$' -count=20
```
