package service

import (
	"testing"
)

func TestExtractModels(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    []string
	}{
		{
			name:    "array of objects",
			payload: `[{"id":"claude-opus-5-5","display_name":"Claude Opus 5.5"},{"model":"claude-sonnet-5-5"}]`,
			want:    []string{"claude-opus-5-5", "claude-sonnet-5-5"},
		},
		{
			name:    "nested models array",
			payload: `{"data":{"models":[{"id":"claude-haiku-5-5","name":"Claude Haiku 5.5"}]}}`,
			want:    []string{"claude-haiku-5-5"},
		},
		{
			name:    "map keyed by model id",
			payload: `{"claude-fable-5-1":{"display_name":"Claude Fable 5.1"}}`,
			want:    []string{"claude-fable-5-1"},
		},
		{
			name:    "disabled model filtered",
			payload: `[{"id":"claude-opus-5-5","available":false},{"id":"claude-sonnet-5-5","available":true}]`,
			want:    []string{"claude-sonnet-5-5"},
		},
		{
			name:    "string under model key",
			payload: `{"default_model":"claude-sonnet-5-5"}`,
			want:    []string{"claude-sonnet-5-5"},
		},
		{
			name:    "ignore prose",
			payload: `{"description":"use claude-opus-5-5 for hard tasks"}`,
			want:    nil,
		},
		{
			name:    "ignore url",
			payload: `{"url":"https://claude.ai/chat"}`,
			want:    nil,
		},
		{
			name:    "dedupe",
			payload: `[{"id":"claude-opus-5-5"},{"id":"claude-opus-5-5"}]`,
			want:    []string{"claude-opus-5-5"},
		},
		{
			name:    "invalid json",
			payload: `not json`,
			want:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractModels([]byte(tc.payload))
			if len(got) != len(tc.want) {
				t.Fatalf("数量不符：got=%d want=%d (%+v)", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i].ID != want {
					t.Fatalf("第 %d 项 = %q, want %q", i, got[i].ID, want)
				}
				if got[i].UpstreamID != want {
					t.Fatalf("第 %d 项 UpstreamID = %q, want %q", i, got[i].UpstreamID, want)
				}
			}
		})
	}
}

func TestExtractModelsTierInference(t *testing.T) {
	got := ExtractModels([]byte(`[{"id":"claude-opus-5-5"},{"id":"claude-sonnet-5-5"},{"id":"claude-haiku-5-5"}]`))
	if len(got) != 3 {
		t.Fatalf("期望 3 个模型，实际 %d", len(got))
	}
	wantTier := map[string]string{
		"claude-opus-5-5":   "pro",
		"claude-sonnet-5-5": "free",
		"claude-haiku-5-5":  "free",
	}
	for _, spec := range got {
		if spec.Tier != wantTier[spec.ID] {
			t.Fatalf("%s 等级 = %q, want %q", spec.ID, spec.Tier, wantTier[spec.ID])
		}
	}
}

func TestLooksLikeModelID(t *testing.T) {
	valid := []string{"claude-opus-5-5", "claude-sonnet-4-5-20250929", "claude-haiku-5-5"}
	for _, v := range valid {
		if !looksLikeModelID(v) {
			t.Fatalf("%q 应识别为模型 ID", v)
		}
	}
	invalid := []string{"", "claude", "claude.ai", "https://claude.ai/chat", "Claude Opus", "gpt-5"}
	for _, v := range invalid {
		if looksLikeModelID(v) {
			t.Fatalf("%q 不应识别为模型 ID", v)
		}
	}
}
