# 订阅权益编排平台

这是一个用于处理订阅计划、权益状态、策略计算和异步命令的 Go 后端。HTTP 层接收订阅命令，应用层执行校验和编排，领域层表达权益规则，仓储层负责状态保存，worker 模块支持异步处理；`web` 是轻量业务控制台。

## 启动

    go run ./cmd/api
    curl http://localhost:8080/healthz
