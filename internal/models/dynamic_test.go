package models

import "testing"

// setDynamicForTest 直接替换内存缓存，避免测试写盘。
func setDynamicForTest(specs []ModelSpec) func() {
	dynamic.mu.Lock()
	dynamic.models = specs
	dynamic.mu.Unlock()
	return func() {
		dynamic.mu.Lock()
		dynamic.models = nil
		dynamic.mu.Unlock()
	}
}

func TestCatalogMergeOrder(t *testing.T) {
	restore := setDynamicForTest([]ModelSpec{
		// 与内置同 ID：应更新上游 ID 与展示名。
		{ID: "claude-opus-5-5", UpstreamID: "claude-opus-5-5-20260922", Name: "Claude Opus 5.5 (web)", Tier: "pro", Thinking: true},
		// 内置没有：应追加。
		{ID: "claude-web-only", UpstreamID: "claude-web-only", Name: "Web Only", Tier: "free", Thinking: true},
	})
	defer restore()

	catalog := ModelCatalog()
	byID := map[string]ModelSpec{}
	for _, spec := range catalog {
		byID[spec.ID] = spec
	}

	opus, ok := byID["claude-opus-5-5"]
	if !ok {
		t.Fatal("清单中应包含 claude-opus-5-5")
	}
	if opus.UpstreamID != "claude-opus-5-5-20260922" {
		t.Fatalf("web 清单应覆盖上游 ID，实际 %q", opus.UpstreamID)
	}
	if opus.Name != "Claude Opus 5.5 (web)" {
		t.Fatalf("web 清单应覆盖展示名，实际 %q", opus.Name)
	}

	if _, ok := byID["claude-web-only"]; !ok {
		t.Fatal("web 清单中的新模型应被追加")
	}
}

func TestResolveModelUsesWebUpstream(t *testing.T) {
	restore := setDynamicForTest([]ModelSpec{
		{ID: "claude-opus-5-5", UpstreamID: "claude-opus-5-5-20260922", Name: "Claude Opus 5.5", Tier: "pro", Thinking: true},
	})
	defer restore()

	spec, thinking, ok := ResolveModel("claude-opus-5-5-thinking")
	if !ok {
		t.Fatal("应能解析 claude-opus-5-5-thinking")
	}
	if !thinking {
		t.Fatal("应识别为 thinking 请求")
	}
	if spec.UpstreamID != "claude-opus-5-5-20260922" {
		t.Fatalf("UpstreamID = %q, want claude-opus-5-5-20260922", spec.UpstreamID)
	}
}

func TestInferTier(t *testing.T) {
	cases := map[string]string{
		"claude-fable-5-1":     "pro",
		"claude-opus-5-5":      "pro",
		"claude-sonnet-5-5":    "free",
		"claude-haiku-5-5":     "free",
		"claude-unknown-model": "free",
	}
	for id, want := range cases {
		if got := InferTier(id); got != want {
			t.Fatalf("InferTier(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestSanitizeSpecs(t *testing.T) {
	got := sanitizeSpecs([]ModelSpec{
		{ID: "  claude-opus-5-5  "},
		{ID: "claude-opus-5-5", Name: "重复项"},
		{ID: ""},
		{ID: "claude-sonnet-5-5", Tier: "bogus"},
	})
	if len(got) != 2 {
		t.Fatalf("期望 2 条，实际 %d：%+v", len(got), got)
	}
	if got[0].ID != "claude-opus-5-5" || got[0].UpstreamID != "claude-opus-5-5" {
		t.Fatalf("第 1 条归一化错误：%+v", got[0])
	}
	if got[1].Tier != "free" {
		t.Fatalf("非法等级应回退推断值，实际 %q", got[1].Tier)
	}
}
