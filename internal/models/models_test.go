package models

import "testing"

func TestResolveModel(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantID    string
		wantThink bool
		wantOK    bool
	}{
		{name: "exact", input: "claude-opus-5-5", wantID: "claude-opus-5-5", wantOK: true},
		{name: "thinking", input: "claude-opus-5-5-thinking", wantID: "claude-opus-5-5", wantThink: true, wantOK: true},
		{name: "empty falls back to default", input: "", wantID: DefaultModelID(), wantOK: true},
		{name: "unknown passes through", input: "my-custom-model", wantID: "my-custom-model", wantOK: false},
		{name: "unknown thinking passes through", input: "my-custom-model-thinking", wantID: "my-custom-model", wantThink: true, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, thinking, ok := ResolveModel(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if spec.ID != tc.wantID {
				t.Fatalf("id = %q, want %q", spec.ID, tc.wantID)
			}
			if thinking != tc.wantThink {
				t.Fatalf("thinking = %v, want %v", thinking, tc.wantThink)
			}
		})
	}
}

func TestTierFiltering(t *testing.T) {
	free := ForTier(TierFree)
	for _, spec := range free {
		if spec.Tier != "free" {
			t.Fatalf("free 等级不应包含 %s（要求 %s）", spec.ID, spec.Tier)
		}
	}
	max := ForTier(TierMax)
	if len(max) <= len(free) {
		t.Fatalf("max 等级可用模型数应多于 free：max=%d free=%d", len(max), len(free))
	}
	seen := map[string]bool{}
	for _, spec := range max {
		seen[spec.ID] = true
	}
	if !seen["claude-fable-5-1"] {
		t.Fatal("max 等级应包含 claude-fable-5-1")
	}
}

func TestParseTier(t *testing.T) {
	for _, name := range TierNames() {
		if _, ok := ParseTier(name); !ok {
			t.Fatalf("ParseTier(%q) 应成功", name)
		}
	}
	if _, ok := ParseTier("unknown"); ok {
		t.Fatal("未知等级不应解析成功")
	}
}

func TestCatalogHasUpstreamID(t *testing.T) {
	for _, spec := range ModelCatalog() {
		if spec.ID == "" || spec.UpstreamID == "" {
			t.Fatalf("模型 %+v 缺少 ID 或 UpstreamID", spec)
		}
		if _, ok := ParseTier(spec.Tier); !ok {
			t.Fatalf("模型 %s 的等级 %q 非法", spec.ID, spec.Tier)
		}
	}
}
