package service

import "strings"

// PlanRank 返回订阅等级的排序值，数值越大权限越高。
// 未知等级返回 -1，表示无法判断、不做限制。
func PlanRank(plan string) int {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "free":
		return 0
	case "pro":
		return 1
	case "max":
		return 2
	case "team":
		return 3
	case "enterprise":
		return 4
	}
	return -1
}

// DetectPlan 从 claude.ai 返回的等级线索推断订阅等级。
//
// claude.ai 的 /api/account 会在 memberships[].organization 中返回
// rate_limit_tier（如 default_claude_max_20x）、organization_type
// （如 claude_pro）和 billing_type。按参数顺序取第一个能识别的线索。
func DetectPlan(hints ...string) string {
	for _, hint := range hints {
		lower := strings.ToLower(strings.TrimSpace(hint))
		if lower == "" {
			continue
		}
		switch {
		case strings.Contains(lower, "enterprise"):
			return "enterprise"
		case strings.Contains(lower, "team"):
			return "team"
		case strings.Contains(lower, "max"):
			return "max"
		case strings.Contains(lower, "pro"):
			return "pro"
		case strings.Contains(lower, "free"):
			return "free"
		}
	}
	return ""
}

// PlanAllows 判断账号等级是否满足模型要求的最低等级。
// 账号等级未知时不限制，避免误伤尚未刷新等级的账号。
func PlanAllows(accountPlan, requiredPlan string) bool {
	accountRank := PlanRank(accountPlan)
	if accountRank < 0 {
		return true
	}
	requiredRank := PlanRank(requiredPlan)
	if requiredRank < 0 {
		return true
	}
	return accountRank >= requiredRank
}
