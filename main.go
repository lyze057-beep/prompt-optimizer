package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	OllamaBaseURL = "http://localhost:11434"
	OllamaModel   = "qwen2.5:3b"
	ServerPort    = ":8080"
)

type OptimizeRequest struct {
	Input string `json:"input"`
	Tool  string `json:"tool"`
	Style string `json:"style"`
}

type OptimizeResponse struct {
	Positive    string `json:"positive"`
	Negative    string `json:"negative"`
	Explanation string `json:"explanation"`
	Tips        string `json:"tips"`
}

type OllamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaChatResponse struct {
	Message ChatMessage `json:"message"`
}

func buildPrompt(userInput, tool, style string) (string, string) {
	system := fmt.Sprintf(`你是专业的AI图像生成提示词工程师。
你的任务：将用户的中文描述转化为规范的%s图像生成提示词。
风格要求：%s

输出规则（必须严格遵守）：
- 只输出一个JSON对象，不能有任何其他文字
- 不能有markdown代码块标记
- JSON结构固定为：{"positive":"...","negative":"...","explanation":"...","tips":"..."}
- positive：英文提示词，结构为「主体, 环境背景, 光线, 镜头视角, 风格品质词」
- negative：适合%s的负向提示词（英文）
- explanation：中文解释positive各部分的含义
- tips：一句针对%s的实用建议（中文）`, tool, style, tool, tool)

	user := fmt.Sprintf(`请将以下描述优化为提示词：
%s

记住：只输出JSON，不要有任何其他内容。`, userInput)

	return system, user
}

func callOllama(userInput, tool, style string) (*OptimizeResponse, error) {
	system, user := buildPrompt(userInput, tool, style)

	body, _ := json.Marshal(OllamaChatRequest{
		Model: OllamaModel,
		Messages: []ChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false,
		Format: "json",
	})

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(OllamaBaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Ollama 连接失败: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var chatResp OllamaChatResponse
	if err := json.Unmarshal(raw, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 Ollama 响应失败: %v", err)
	}

	text := strings.TrimSpace(chatResp.Message.Content)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("模型未返回 JSON，原始输出: %s", text)
	}
	jsonStr := text[start : end+1]

	var result OptimizeResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %v\n内容: %s", err, jsonStr)
	}

	if result.Positive == "" {
		return nil, fmt.Errorf("模型返回内容不完整: %s", jsonStr)
	}

	return &result, nil
}

func handleOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OptimizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"请求格式错误"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Input) == "" {
		http.Error(w, `{"error":"输入不能为空"}`, http.StatusBadRequest)
		return
	}
	if req.Tool == "" {
		req.Tool = "Stable Diffusion"
	}
	if req.Style == "" {
		req.Style = "写实摄影"
	}

	log.Printf("[optimize] tool=%s style=%s input=%s", req.Tool, req.Style, req.Input)

	result, err := callOllama(req.Input, req.Tool, req.Style)
	if err != nil {
		log.Printf("[error] %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(OllamaBaseURL + "/api/tags")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "ollama offline"})
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "model": OllamaModel})
}

func main() {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("./static")))
	mux.HandleFunc("/api/optimize", handleOptimize)
	mux.HandleFunc("/api/health", handleHealth)

	log.Printf("服务启动: http://localhost%s", ServerPort)
	log.Printf("使用模型: %s @ %s", OllamaModel, OllamaBaseURL)

	if err := http.ListenAndServe(ServerPort, mux); err != nil {
		log.Fatal(err)
	}
}
