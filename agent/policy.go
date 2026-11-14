package main

import "strings"

const (
	policyAllow    = "ALLOW"
	policyApproval = "REQUIRE_APPROVAL"
	policyDeny     = "DENY"
)

// policyOf 只根据分类和步骤文本判断。事件 message 里的命令不会变成步骤。
func policyOf(classification string, steps []string) string {
	if len(steps) == 1 && isDrainStep(steps[0]) {
		return policyApproval
	}
	for _, step := range steps {
		if mutating(step) {
			return policyDeny
		}
	}
	switch classification {
	case "http_5xx_spike", "port_unreachable", "stuck_suspected", "certificate_expiring":
		return policyApproval
	case "service_restarted":
		return policyAllow
	case "service_unavailable", "collect_error", "certificate_read_error", "unknown":
		return policyDeny
	default:
		return policyDeny
	}
}

func mutating(step string) bool {
	s := strings.ToLower(step)
	for _, frag := range []string{
		"config apply", "config rollback", "service reload",
		"site ", "cert renew", "cert replace", "restart", "upstream drain", "upstream undrain",
	} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

// isDrainStep 只接受「upstream drain <host>」或「upstream undrain <host>」。
func isDrainStep(step string) bool {
	fields := strings.Fields(step)
	if len(fields) != 3 || fields[0] != "upstream" {
		return false
	}
	if fields[1] != "drain" && fields[1] != "undrain" {
		return false
	}
	return fields[2] != "" && !strings.ContainsAny(fields[2], ";&|$`'\"\\")
}

func statusFor(classification, decision string, steps []string) string {
	switch classification {
	case "collect_error", "certificate_read_error", "unknown", "service_unavailable":
		return "ESCALATED"
	}
	if decision == policyAllow {
		return "COMPLETED"
	}
	if decision == policyApproval && executableDrain(steps) {
		return "WAITING_APPROVAL"
	}
	if decision == policyApproval {
		return "WAITING_HUMAN"
	}
	return "ESCALATED"
}

func executableDrain(steps []string) bool {
	return len(steps) == 1 && isDrainStep(steps[0])
}
