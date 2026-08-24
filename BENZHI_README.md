# BENZHI_README

基于 Go 实现的洞穴测绘归档验收工作台 Web 项目，一款后端服务，洞穴测绘归档验收工作台提供归档包建档、测站测段修订、拓扑质量检查、人工裁决、返修复验、成果冻结和不可变验收凭据签发的完整中文浏览器流程。

## 项目说明
- 项目：benzhi-project-e4778259-ded2-4443-a455-ada19e3796c2
- 项目用途：洞穴测绘归档验收工作台提供归档包建档、测站测段修订、拓扑质量检查、人工裁决、返修复验、成果冻结和不可变验收凭据签发的完整中文浏览器流程。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19473
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-e4778259-ded2-4443-a455-ada19e3796c2-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-e4778259-ded2-4443-a455-ada19e3796c2-arm64 linux/arm64
docker run -it benzhi-project-e4778259-ded2-4443-a455-ada19e3796c2-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19473`
