# Docker 评测说明

这是订阅权益编排平台 016 题的 Go 评测环境，工具链为 Go 1.22，模块语言版本为 Go 1.22。镜像使用官方多架构基础镜像，依赖在构建阶段下载。

构建：`./build_benzhi_docker.sh subscription-entitlement-platform-016 linux/arm64` 或将平台替换为 `linux/amd64`。

容器内测试：`GOTOOLCHAIN=local GOCACHE=/tmp/subscription-entitlement-016-cache go test ./...`。
