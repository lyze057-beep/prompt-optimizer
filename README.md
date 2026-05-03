项目说明：
- Go 标准库实现，零第三方依赖
- 本地 Ollama + qwen2.5:3b 驱动，数据不出本地
- 支持 Midjourney / SD / ComfyUI 等多种工具
- 输出正向提示词、负向提示词、中文释义、使用建议

本地运行：
1. 启动 Ollama：ollama serve
2. 运行服务：go run main.go
3. 浏览器访问：http://localhost:8080
