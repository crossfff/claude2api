package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"claude2api/internal/config"
	"claude2api/internal/models"
	"claude2api/internal/repository"
	"claude2api/internal/utils"

	fhttp "github.com/bogdanfinn/fhttp"
)

// WebModelSync 是一次从 claude.ai 网页端拉取模型的结果。
type WebModelSync struct {
	Source   string             `json:"source"`
	Models   []models.ModelSpec `json:"models"`
	Attempts []ModelProbeResult `json:"attempts,omitempty"`
}

// ModelSyncResult 是同步接口的返回体。
type ModelSyncResult struct {
	Email     string             `json:"email"`
	Plan      string             `json:"plan"`
	Source    string             `json:"source"`
	Count     int                `json:"count"`
	Models    []models.ModelSpec `json:"models"`
	UpdatedAt time.Time          `json:"updated_at"`
	Attempts  []ModelProbeResult `json:"attempts,omitempty"`
}

// modelPaths 是网页端可能返回模型清单的候选路径，按命中概率排序。
func (claudeAI *ClaudeAI) modelPaths() []string {
	paths := []string{"/api/bootstrap"}
	if claudeAI.orgUUID != "" {
		paths = append(paths,
			fmt.Sprintf("/api/organizations/%s/models", claudeAI.orgUUID),
			fmt.Sprintf("/api/organizations/%s/available_models", claudeAI.orgUUID),
		)
	}
	return append(paths, "/api/models", "/api/account")
}

// FetchWebModels 从 claude.ai 网页端拉取该账号可见的模型清单。
//
// 上游没有公开文档，且不同版本可能更换端点，因此依次尝试候选路径，
// 第一个能解析出模型的响应即为结果；其余尝试记录在 Attempts 里便于排查。
func (claudeAI *ClaudeAI) FetchWebModels() (*WebModelSync, error) {
	if err := claudeAI.WarmUp(); err != nil {
		return nil, fmt.Errorf("初始化网页会话失败: %w", err)
	}
	if claudeAI.orgUUID == "" {
		if _, err := claudeAI.GetUserInfo(); err != nil {
			return nil, fmt.Errorf("查询账号信息失败: %w", err)
		}
	}

	paths := claudeAI.modelPaths()
	result := &WebModelSync{Attempts: make([]ModelProbeResult, 0, len(paths))}
	for _, path := range paths {
		body, status, err := claudeAI.fetchJSONBody(path, 4<<20)
		attempt := ModelProbeResult{Endpoint: path, StatusCode: status}
		if err != nil {
			attempt.Error = err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		specs := ExtractModels([]byte(body))
		attempt.Body = utils.Truncate(body, 2000)
		result.Attempts = append(result.Attempts, attempt)
		if status >= 400 || len(specs) == 0 {
			continue
		}
		result.Source = path
		result.Models = specs
		return result, nil
	}
	return result, fmt.Errorf("上游响应中未识别到模型列表")
}

// fetchJSONBody 读取完整响应体（按 limit 截断），用于解析而非展示。
func (claudeAI *ClaudeAI) fetchJSONBody(path string, limit int64) (string, int, error) {
	req, err := claudeAI.request(fhttp.MethodGet, claudeAIBaseURL+path, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Del("content-type")
	req.Header.Set("accept", "application/json")
	req.Header.Set("referer", claudeAIBaseURL+"/new")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := claudeAI.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

// ExtractModels 从任意 JSON 响应里提取模型标识。
//
// 不同端点的结构差异很大（数组、对象、以模型名为 key 的 map），
// 因此采用宽容遍历：优先识别带 id/model 字段的对象，其次识别
// 以 claude- 开头的 map key 与 model 相关字段下的字符串值。
func ExtractModels(payload []byte) []models.ModelSpec {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil
	}
	collector := &modelCollector{seen: map[string]bool{}}
	collector.walk(root, "", 0)
	return collector.list
}

type modelCollector struct {
	seen map[string]bool
	list []models.ModelSpec
}

func (c *modelCollector) walk(node any, hint string, depth int) {
	if depth > 12 {
		return
	}
	switch value := node.(type) {
	case []any:
		for _, item := range value {
			c.walk(item, hint, depth+1)
		}
	case map[string]any:
		if id, name, ok := modelFromObject(value); ok {
			c.add(id, name)
			return
		}
		for key, child := range value {
			// 以模型名作为 key 的 map：{"claude-sonnet-5-5": {...}}
			if looksLikeModelID(key) {
				name := ""
				if obj, ok := child.(map[string]any); ok {
					name = pickString(obj, "display_name", "displayName", "name", "title", "label")
				}
				c.add(key, name)
			}
			c.walk(child, key, depth+1)
		}
	case string:
		// 仅在 model 相关字段下收集字符串，避免把文案里的模型名当成清单。
		if strings.Contains(strings.ToLower(hint), "model") && looksLikeModelID(value) {
			c.add(value, "")
		}
	}
}

func (c *modelCollector) add(id, name string) {
	id = strings.TrimSpace(id)
	if id == "" || c.seen[id] {
		return
	}
	c.seen[id] = true
	c.list = append(c.list, models.ModelSpec{
		ID:         id,
		UpstreamID: id,
		Name:       strings.TrimSpace(name),
		Tier:       models.InferTier(id),
		Thinking:   true,
	})
}

// modelFromObject 判断一个 JSON 对象是否描述了一个模型。
func modelFromObject(obj map[string]any) (string, string, bool) {
	for _, flag := range []string{"available", "enabled", "is_available", "isAvailable"} {
		if enabled, ok := obj[flag].(bool); ok && !enabled {
			return "", "", false
		}
	}
	id := pickString(obj, "id", "model", "model_id", "modelId", "slug", "value")
	if !looksLikeModelID(id) {
		return "", "", false
	}
	return id, pickString(obj, "display_name", "displayName", "name", "title", "label"), true
}

func pickString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// looksLikeModelID 判断字符串是否像 claude.ai 的模型标识。
func looksLikeModelID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 80 {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(value), "claude-") {
		return false
	}
	return !strings.ContainsAny(value, " \t\r\n/\\")
}

// SyncModels 用号池账号从 claude.ai 网页端拉取模型并写入缓存。
// email 为空时自动挑选等级最高的可用账号（能看到的模型最全）。
func SyncModels(email string) (*ModelSyncResult, error) {
	account := pickSyncAccount(email)
	if account == nil {
		err := fmt.Errorf("号池中没有可用账号，无法从 web 端拉取模型")
		models.SetDynamicError(err.Error())
		return nil, err
	}

	client := NewClaudeAI(SessionKey(account), config.Get().Proxy, account.Email, account.OrgUUID)
	sync, err := client.FetchWebModels()
	if err != nil {
		models.SetDynamicError(err.Error())
		result := &ModelSyncResult{Email: account.Email, Plan: account.Plan}
		if sync != nil {
			result.Attempts = sync.Attempts
		}
		return result, err
	}
	if err := models.SetDynamic(sync.Source, sync.Models); err != nil {
		models.SetDynamicError(err.Error())
		return nil, fmt.Errorf("保存模型缓存失败: %w", err)
	}
	return &ModelSyncResult{
		Email:     account.Email,
		Plan:      account.Plan,
		Source:    sync.Source,
		Count:     len(sync.Models),
		Models:    sync.Models,
		UpdatedAt: time.Now(),
		Attempts:  sync.Attempts,
	}, nil
}

// pickSyncAccount 选择用于同步模型的账号。
func pickSyncAccount(email string) *repository.Account {
	if strings.TrimSpace(email) != "" {
		return AccountByEmail(strings.TrimSpace(email))
	}
	accounts := repository.LoadAccounts()
	var best *repository.Account
	for i := range accounts {
		if !AccountUsable(&accounts[i]) {
			continue
		}
		if best == nil || PlanRank(accounts[i].Plan) > PlanRank(best.Plan) {
			best = &accounts[i]
		}
	}
	return best
}

// StartModelSyncLoop 按配置间隔从 web 端刷新模型清单。
// 间隔为负数表示关闭自动同步。
func StartModelSyncLoop() {
	if config.Get().ModelSyncIntervalSeconds < 0 {
		slog.Info("[模型] web 端自动同步已关闭")
		return
	}
	go func() {
		// 启动后稍等，避开账号巡检与首批请求。
		time.Sleep(15 * time.Second)
		for {
			interval := config.Get().ModelSyncIntervalSeconds
			if interval < 0 {
				slog.Info("[模型] web 端自动同步已关闭")
				return
			}
			if interval == 0 {
				interval = 3600
			}
			if result, err := SyncModels(""); err != nil {
				slog.Warn("[模型] web 端同步失败", "err", err)
			} else {
				slog.Info("[模型] web 端同步完成",
					"count", result.Count, "source", result.Source, "email", result.Email, "plan", result.Plan)
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()
}
