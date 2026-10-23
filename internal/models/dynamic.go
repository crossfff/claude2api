package models

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"claude2api/internal/config"
)

// 模型来源标记。
const (
	SourceBuiltin = "builtin"
	SourceConfig  = "config"
	SourceWeb     = "web"
)

// DynamicSnapshot 是最近一次从 claude.ai 网页端拉取的模型清单快照。
type DynamicSnapshot struct {
	Models    []ModelSpec `json:"models"`
	Source    string      `json:"source"`
	UpdatedAt time.Time   `json:"updated_at"`
	LastError string      `json:"last_error,omitempty"`
}

type dynamicState struct {
	mu        sync.RWMutex
	models    []ModelSpec
	source    string
	updatedAt time.Time
	lastError string
}

var dynamic dynamicState

// dynamicFilePath 返回 web 端模型缓存文件路径。
func dynamicFilePath() string {
	return filepath.Join(config.DataDir(), "web_models.json")
}

// LoadDynamic 从磁盘恢复上次拉取到的 web 端模型清单。
// 启动时调用一次即可，读不到缓存时静默跳过。
func LoadDynamic() {
	data, err := os.ReadFile(dynamicFilePath())
	if err != nil {
		return
	}
	var saved DynamicSnapshot
	if err := json.Unmarshal(data, &saved); err != nil {
		slog.Warn("[模型] web 端模型缓存损坏，已忽略", "err", err)
		return
	}
	dynamic.mu.Lock()
	dynamic.models = sanitizeSpecs(saved.Models)
	dynamic.source = saved.Source
	dynamic.updatedAt = saved.UpdatedAt
	dynamic.mu.Unlock()
	if len(saved.Models) > 0 {
		slog.Info("[模型] 已加载 web 端模型缓存", "count", len(saved.Models), "source", saved.Source, "updated_at", saved.UpdatedAt)
	}
}

// SetDynamic 更新 web 端模型清单并落盘。
func SetDynamic(source string, specs []ModelSpec) error {
	cleaned := sanitizeSpecs(specs)
	now := time.Now()

	dynamic.mu.Lock()
	dynamic.models = cleaned
	dynamic.source = source
	dynamic.updatedAt = now
	dynamic.lastError = ""
	snapshot := DynamicSnapshot{Models: cleaned, Source: source, UpdatedAt: now}
	dynamic.mu.Unlock()

	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dynamicFilePath(), data, 0o644)
}

// SetDynamicError 记录最近一次同步失败的原因，不影响已有缓存。
func SetDynamicError(message string) {
	dynamic.mu.Lock()
	dynamic.lastError = strings.TrimSpace(message)
	dynamic.mu.Unlock()
}

// Snapshot 返回当前动态清单的副本与元信息。
func Snapshot() DynamicSnapshot {
	dynamic.mu.RLock()
	defer dynamic.mu.RUnlock()
	out := DynamicSnapshot{
		Source:    dynamic.source,
		UpdatedAt: dynamic.updatedAt,
		LastError: dynamic.lastError,
	}
	out.Models = append([]ModelSpec(nil), dynamic.models...)
	return out
}

// ClearDynamic 清空动态清单与磁盘缓存。
func ClearDynamic() error {
	dynamic.mu.Lock()
	dynamic.models = nil
	dynamic.source = ""
	dynamic.updatedAt = time.Time{}
	dynamic.lastError = ""
	dynamic.mu.Unlock()
	if err := os.Remove(dynamicFilePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// InferTier 在无法从上游获得等级时，按模型系列推断最低可用等级。
func InferTier(id string) string {
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "fable"), strings.Contains(lower, "opus"):
		return "pro"
	case strings.Contains(lower, "sonnet"), strings.Contains(lower, "haiku"):
		return "free"
	}
	return "free"
}

// sanitizeSpecs 归一化上游解析出的模型条目。
func sanitizeSpecs(specs []ModelSpec) []ModelSpec {
	out := make([]ModelSpec, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		id := strings.TrimSpace(spec.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		spec.ID = id
		if strings.TrimSpace(spec.UpstreamID) == "" {
			spec.UpstreamID = id
		}
		if strings.TrimSpace(spec.Name) == "" {
			spec.Name = id
		}
		if _, ok := ParseTier(spec.Tier); !ok {
			spec.Tier = InferTier(id)
		} else {
			spec.Tier = strings.ToLower(strings.TrimSpace(spec.Tier))
		}
		out = append(out, spec)
	}
	return out
}
