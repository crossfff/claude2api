package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"claude2api/internal/utils"

	fhttp "github.com/bogdanfinn/fhttp"
)

// ModelProbeResult 是一次上游模型探测的结果。
type ModelProbeResult struct {
	Endpoint   string `json:"endpoint"`
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
	Error      string `json:"error,omitempty"`
}

// ProbeModels 尝试从 claude.ai 拉取该账号实际可用的模型清单。
//
// claude.ai 未公开文档，模型选择器的数据端点可能随版本变化，
// 因此按候选路径逐个探测，把原始响应交回调用方判断。
func (claudeAI *ClaudeAI) ProbeModels() []ModelProbeResult {
	if err := claudeAI.WarmUp(); err != nil {
		return []ModelProbeResult{{Endpoint: "warmup", Error: err.Error()}}
	}
	if claudeAI.orgUUID == "" {
		if _, err := claudeAI.GetUserInfo(); err != nil {
			return []ModelProbeResult{{Endpoint: "/api/account", Error: err.Error()}}
		}
	}
	paths := []string{
		fmt.Sprintf("/api/organizations/%s/models", claudeAI.orgUUID),
		"/api/models",
		fmt.Sprintf("/api/organizations/%s/available_models", claudeAI.orgUUID),
		"/api/account",
	}
	out := make([]ModelProbeResult, 0, len(paths))
	for _, path := range paths {
		out = append(out, claudeAI.probeGet(path))
	}
	return out
}

// probeGet 发起一次只读探测请求。
func (claudeAI *ClaudeAI) probeGet(path string) ModelProbeResult {
	res := ModelProbeResult{Endpoint: path}
	req, err := claudeAI.request(fhttp.MethodGet, claudeAIBaseURL+path, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Del("content-type")
	req.Header.Set("accept", "application/json")
	req.Header.Set("referer", claudeAIBaseURL+"/new")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := claudeAI.client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Body = utils.Truncate(string(body), 8000)
	return res
}
