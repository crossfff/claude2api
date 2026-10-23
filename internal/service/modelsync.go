package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
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
	Accounts  []string           `json:"accounts,omitempty"`
	Failed    []string           `json:"failed,omitempty"`
	Attempts  []ModelProbeResult `json:"attempts,omitempty"`
}

// maxSyncAccounts 是自动同步时最多遍历的账号数。
// 不同订阅等级能看到的模型不同，合并多个账号才能得到最全清单。
const maxSyncAccounts = 5

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

// SyncModels 从 claude.ai 网页端拉取模型清单并写入缓存。
//
// email 为空时遍历号池中最多 maxSyncAccounts 个可用账号（等级从高到低），
// 把各自可见的模型合并去重，得到「最全」清单；指定 email 时只同步该账号。
func SyncModels(email string) (*ModelSyncResult, error) {
	email = strings.TrimSpace(email)
	if email != "" {
		account := AccountByEmail(email)
		if account == nil {
			err := fmt.Errorf("账号不存在或不可用: %s", email)
			models.SetDynamicError(err.Error())
			return nil, err
		}
		return syncFromAccount(account)
	}

	accounts := syncCandidates()
	if len(accounts) == 0 {
		err := fmt.Errorf("号池中没有可用账号，无法从 web 端拉取模型")
		models.SetDynamicError(err.Error())
		return nil, err
	}

	merged := make([]models.ModelSpec, 0, 16)
	seen := make(map[string]bool, 16)
	var attempts []ModelProbeResult
	var succeeded, failed []string
	var lastErr error

	for i, account := range accounts {
		if i > 0 {
			// 账号之间稍作停顿，降低触发上游风控的概率。
			time.Sleep(time.Second)
		}
		sync, err := fetchFromAccount(account)
		if sync != nil {
			attempts = append(attempts, sync.Attempts...)
		}
		if err != nil {
			failed = append(failed, account.Email)
			lastErr = err
			continue
		}
		succeeded = append(succeeded, account.Email)
		for _, spec := range sync.Models {
			if seen[spec.ID] {
				continue
			}
			seen[spec.ID] = true
			merged = append(merged, spec)
		}
	}

	if len(merged) == 0 {
		message := "所有账号都未能拉到模型列表"
		if lastErr != nil {
			message = lastErr.Error()
		}
		models.SetDynamicError(message)
		return &ModelSyncResult{Attempts: attempts, Failed: failed}, fmt.Errorf("%s", message)
	}

	source := "web(merged)"
	if len(succeeded) == 1 {
		source = "web"
	}
	if err := models.SetDynamic(source, merged); err != nil {
		models.SetDynamicError(err.Error())
		return nil, fmt.Errorf("保存模型缓存失败: %w", err)
	}
	return &ModelSyncResult{
		Email:     succeeded[0],
		Source:    source,
		Count:     len(merged),
		Models:    merged,
		UpdatedAt: time.Now(),
		Accounts:  succeeded,
		Failed:    failed,
		Attempts:  attempts,
	}, nil
}

// syncFromAccount 同步单个账号并立即落盘。
func syncFromAccount(account *repository.Account) (*ModelSyncResult, error) {
	sync, err := fetchFromAccount(account)
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
		Accounts:  []string{account.Email},
		Attempts:  sync.Attempts,
	}, nil
}

// fetchFromAccount 只做拉取，不写缓存，便于多账号合并。
func fetchFromAccount(account *repository.Account) (*WebModelSync, error) {
	client := NewClaudeAI(SessionKey(account), config.Get().Proxy, account.Email, account.OrgUUID)
	return client.FetchWebModels()
}

// syncCandidates 返回参与自动同步的账号，按等级从高到低排序并限制数量。
func syncCandidates() []*repository.Account {
	accounts := repository.LoadAccounts()
	usable := make([]*repository.Account, 0, len(accounts))
	for i := range accounts {
		if AccountUsable(&accounts[i]) {
			usable = append(usable, &accounts[i])
		}
	}
	sort.SliceStable(usable, func(i, j int) bool {
		return PlanRank(usable[i].Plan) > PlanRank(usable[j].Plan)
	})
	if len(usable) > maxSyncAccounts {
		usable = usable[:maxSyncAccounts]
	}
	return usable
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
