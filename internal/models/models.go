// Package models 维护对外模型与 claude.ai 上游模型之间的对应关系。
//
// 这里是模型清单的唯一数据源：适配层（/v1/models、请求模型归一化）和
// 调度层（按账号等级挑选账号）都从这里读取，避免多处硬编码产生偏差。
package models

import (
	"strings"

	"claude2api/internal/config"
)

// Tier 是 Claude.ai 订阅等级，数值越大权限越高。
type Tier int

const (
	TierFree Tier = iota
	TierPro
	TierMax
	TierTeam
	TierEnterprise
)

var tierNames = [...]string{"free", "pro", "max", "team", "enterprise"}

// String 返回等级名称。
func (t Tier) String() string {
	if t < 0 || int(t) >= len(tierNames) {
		return tierNames[TierFree]
	}
	return tierNames[t]
}

// ParseTier 解析等级名称；未知名称返回 free 与 false。
func ParseTier(name string) (Tier, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, n := range tierNames {
		if n == name {
			return Tier(i), true
		}
	}
	return TierFree, false
}

// TierNames 返回全部等级名称，按权限从低到高排列。
func TierNames() []string {
	out := make([]string, len(tierNames))
	copy(out, tierNames[:])
	return out
}

// ModelSpec 描述对外模型与 claude.ai 上游模型之间的对应关系。
//
//   - ID：对外暴露的模型名，客户端在请求里填写它；
//   - UpstreamID：真正传给 claude.ai 的 model 字段；
//   - Tier：该模型要求的最低订阅等级（free/pro/max/team/enterprise）；
//   - Thinking：是否额外提供 ID+"-thinking" 形式；
//   - Aliases：兼容别名，请求命中别名时同样转发到 UpstreamID。
type ModelSpec struct {
	ID         string   `json:"id" yaml:"id"`
	UpstreamID string   `json:"upstream_id" yaml:"upstream_id"`
	Name       string   `json:"name" yaml:"name"`
	Tier       string   `json:"tier" yaml:"tier"`
	Thinking   bool     `json:"thinking" yaml:"thinking"`
	Aliases    []string `json:"aliases" yaml:"aliases"`
}

// builtinModels 是内置模型清单，对应 Anthropic 2026-10 的官方模型。
// 可用配置项 model_overrides 覆盖或追加，无需改代码。
var builtinModels = []ModelSpec{
	{
		ID: "claude-fable-5-1", UpstreamID: "claude-fable-5-1",
		Name: "Claude Fable 5.1", Tier: "max", Thinking: true,
	},
	{
		ID: "claude-opus-5-5", UpstreamID: "claude-opus-5-5",
		Name: "Claude Opus 5.5", Tier: "pro", Thinking: true,
	},
	{
		ID: "claude-sonnet-5-5", UpstreamID: "claude-sonnet-5-5",
		Name: "Claude Sonnet 5.5", Tier: "free", Thinking: true,
	},
	{
		ID: "claude-haiku-5-5", UpstreamID: "claude-haiku-5-5",
		Name: "Claude Haiku 5.5", Tier: "free", Thinking: true,
	},
}

// ModelCatalog 返回当前生效的模型清单：内置清单叠加配置覆盖。
func ModelCatalog() []ModelSpec {
	out := make([]ModelSpec, 0, len(builtinModels))
	for _, spec := range builtinModels {
		out = append(out, spec)
	}

	for _, override := range config.Get().ModelOverrides {
		id := strings.TrimSpace(override.ID)
		if id == "" {
			continue
		}
		index := -1
		for i := range out {
			if out[i].ID == id {
				index = i
				break
			}
		}
		if override.Disabled {
			if index >= 0 {
				out = append(out[:index], out[index+1:]...)
			}
			continue
		}
		if index < 0 {
			out = append(out, ModelSpec{ID: id, UpstreamID: id, Name: id, Tier: "free", Thinking: true})
			index = len(out) - 1
		}
		spec := &out[index]
		if v := strings.TrimSpace(override.UpstreamID); v != "" {
			spec.UpstreamID = v
		}
		if v := strings.TrimSpace(override.Name); v != "" {
			spec.Name = v
		}
		if v := strings.TrimSpace(override.Tier); v != "" {
			if _, ok := ParseTier(v); ok {
				spec.Tier = strings.ToLower(v)
			}
		}
		if override.Thinking != nil {
			spec.Thinking = *override.Thinking
		}
		if len(override.Aliases) > 0 {
			spec.Aliases = append([]string(nil), override.Aliases...)
		}
	}
	return out
}

// DefaultModelID 返回未指定 model 时使用的模型名。
func DefaultModelID() string {
	catalog := ModelCatalog()
	if want := strings.TrimSpace(config.Get().DefaultModel); want != "" {
		for _, spec := range catalog {
			if spec.ID == want {
				return spec.ID
			}
		}
	}
	for _, spec := range catalog {
		if spec.Tier == "free" {
			return spec.ID
		}
	}
	if len(catalog) > 0 {
		return catalog[0].ID
	}
	return "claude-sonnet-5-5"
}

// ResolveModel 解析请求里的模型名，返回匹配的规格与是否启用 thinking。
// 未登记的模型名按“原样透传上游”处理，保持对旧客户端的兼容。
func ResolveModel(name string) (ModelSpec, bool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultModelID()
	}
	thinking := strings.HasSuffix(name, "-thinking")
	base := strings.TrimSuffix(name, "-thinking")

	for _, spec := range ModelCatalog() {
		if spec.ID == base || spec.UpstreamID == base {
			if thinking && !spec.Thinking {
				thinking = false
			}
			return spec, thinking, true
		}
		for _, alias := range spec.Aliases {
			if strings.TrimSpace(alias) == base {
				if thinking && !spec.Thinking {
					thinking = false
				}
				return spec, thinking, true
			}
		}
	}
	return ModelSpec{ID: base, UpstreamID: base, Name: base, Tier: "free"}, thinking, false
}

// AllowedForTier 判断某等级是否可用该模型。
func AllowedForTier(spec ModelSpec, tier Tier) bool {
	required, ok := ParseTier(spec.Tier)
	if !ok {
		required = TierFree
	}
	return tier >= required
}

// ForTier 返回指定等级可用的模型清单。
func ForTier(tier Tier) []ModelSpec {
	catalog := ModelCatalog()
	out := make([]ModelSpec, 0, len(catalog))
	for _, spec := range catalog {
		if AllowedForTier(spec, tier) {
			out = append(out, spec)
		}
	}
	return out
}
