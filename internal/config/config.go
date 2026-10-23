package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ModelOverride 用于覆盖或追加模型清单中的一项。
// 未填写的字段保持内置值，Disabled 为 true 时从清单中移除该模型。
type ModelOverride struct {
	ID         string   `json:"id" yaml:"id"`
	UpstreamID string   `json:"upstream_id" yaml:"upstream_id"`
	Name       string   `json:"name" yaml:"name"`
	Tier       string   `json:"tier" yaml:"tier"`
	Thinking   *bool    `json:"thinking" yaml:"thinking"`
	Aliases    []string `json:"aliases" yaml:"aliases"`
	Disabled   bool     `json:"disabled" yaml:"disabled"`
}

// Settings 是运行时配置。
type Settings struct {
	Proxy                      string          `json:"proxy" yaml:"proxy"`
	WebHost                    string          `json:"web_host" yaml:"web_host"`
	WebPort                    int             `json:"web_port" yaml:"-"`
	WebUCHost                  string          `json:"-" yaml:"web_uc_host"`
	AdminPassword              string          `json:"-" yaml:"admin_password"`
	StatusCheckIntervalSeconds int             `json:"status_check_interval_seconds" yaml:"status_check_seconds"`
	RetryCount                 int             `json:"retry_count" yaml:"retry_count"`
	ChatDelete                 bool            `json:"chat_delete" yaml:"delete_chat"`
	MaxChatHistoryLength       int             `json:"max_chat_history_length" yaml:"max_history_length"`
	RemoveInvalidAccount       bool            `json:"remove_invalid_account" yaml:"remove_invalid_account"`
	DetailedAPILog             bool            `json:"detailed_api_log" yaml:"detailed_api_log"`
	DefaultModel               string          `json:"default_model" yaml:"default_model"`
	ModelOverrides             []ModelOverride `json:"model_overrides" yaml:"model_overrides"`
}

var (
	baseDir    string
	configPath string

	current   Settings
	currentMu sync.RWMutex
)

// Load 加载配置。
func Load() {
	exe, err := os.Executable()
	if err == nil {
		baseDir = filepath.Dir(exe)
	} else {
		baseDir, _ = os.Getwd()
	}
	// 开发运行时优先使用当前目录的配置。
	if wd, err := os.Getwd(); err == nil {
		_, e1 := os.Stat(filepath.Join(wd, "config.yaml"))
		_, e2 := os.Stat(filepath.Join(wd, "config.example.yaml"))
		if e1 == nil || e2 == nil {
			baseDir = wd
		}
	}
	configPath = filepath.Join(baseDir, "config.yaml")
	ensureConfig(configPath)
	current = Settings{WebHost: "127.0.0.1", WebPort: 8787, WebUCHost: "localhost", StatusCheckIntervalSeconds: 21600, ChatDelete: true, MaxChatHistoryLength: 12000}
	data, _ := os.ReadFile(configPath)
	_ = yaml.Unmarshal(data, &current)
	current.Proxy = strings.TrimSpace(current.Proxy)
	current.AdminPassword = strings.TrimSpace(current.AdminPassword)
	current.WebHost = strings.TrimSpace(current.WebHost)
	if current.WebHost == "" {
		current.WebHost = "127.0.0.1"
	}
	current.WebUCHost = strings.TrimSpace(current.WebUCHost)
	if current.WebUCHost == "" {
		current.WebUCHost = "localhost"
	}
	if current.StatusCheckIntervalSeconds < 21600 {
		current.StatusCheckIntervalSeconds = 21600
	}
	if current.MaxChatHistoryLength <= 0 {
		current.MaxChatHistoryLength = 12000
	}
}

func Get() Settings {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

// ensureConfig 缺配置时复制示例文件。
func ensureConfig(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	example := filepath.Join(filepath.Dir(path), "config.example.yaml")
	data, err := os.ReadFile(example)
	if err != nil {
		slog.Warn("[配置] 未找到 config.yaml，也无 config.example.yaml 可复制，使用内置默认值", "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Warn("[配置] 复制 config.example.yaml 失败，使用内置默认值", "err", err)
		return
	}
	slog.Info("[配置] 未找到 config.yaml，已从 config.example.yaml 复制生成", "path", path)
}

// DataDir 返回数据目录。
func DataDir() string {
	return filepath.Join(baseDir, "data")
}

// DBPath 返回数据库路径。
func DBPath() string {
	return filepath.Join(DataDir(), "app.db")
}

func writeConfig() {
	data, _ := yaml.Marshal(&current)
	_ = os.WriteFile(configPath, data, 0o644)
}

func Update(patch map[string]any) Settings {
	currentMu.Lock()
	defer currentMu.Unlock()
	for k, v := range patch {
		switch k {
		case "proxy":
			current.Proxy = strings.TrimSpace(fmt.Sprint(v))
		case "admin_password":
			current.AdminPassword = strings.TrimSpace(fmt.Sprint(v))
		case "status_check_interval_seconds":
			current.StatusCheckIntervalSeconds = mustAtoi(v)
		case "retry_count":
			current.RetryCount = mustAtoi(v)
		case "chat_delete":
			current.ChatDelete = mustBool(v)
		case "max_chat_history_length":
			current.MaxChatHistoryLength = mustAtoi(v)
		case "remove_invalid_account":
			current.RemoveInvalidAccount = mustBool(v)
		case "detailed_api_log":
			current.DetailedAPILog = mustBool(v)
		case "default_model":
			current.DefaultModel = strings.TrimSpace(fmt.Sprint(v))
		case "model_overrides":
			if overrides, err := decodeModelOverrides(v); err == nil {
				current.ModelOverrides = overrides
			} else {
				slog.Warn("[配置] model_overrides 格式无效，已忽略", "err", err)
			}
		}
	}
	writeConfig()
	return current
}

// decodeModelOverrides 把前端/配置文件传入的任意结构归一化为模型覆盖项。
func decodeModelOverrides(v any) ([]ModelOverride, error) {
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var raw []ModelOverride
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make([]ModelOverride, 0, len(raw))
	for _, item := range raw {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			continue
		}
		item.UpstreamID = strings.TrimSpace(item.UpstreamID)
		item.Name = strings.TrimSpace(item.Name)
		item.Tier = strings.ToLower(strings.TrimSpace(item.Tier))
		out = append(out, item)
	}
	return out, nil
}

func mustAtoi(v any) int {
	n, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
	return n
}

func mustBool(v any) bool {
	b, _ := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(v)))
	return b
}
