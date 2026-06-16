package cmd

type releaseReadiness struct {
	Level                      string   `json:"level"`
	FCCRequired                bool     `json:"fcc_required"`
	FCCStatus                  string   `json:"fcc_status"`
	MockUpstreamRequired       bool     `json:"mock_upstream_required"`
	MockUpstreamStatus         string   `json:"mock_upstream_status"`
	LiveSmokeRequiredForStable bool     `json:"live_smoke_required_for_stable"`
	LiveSmokeStatus            string   `json:"live_smoke_status"`
	Reason                     string   `json:"reason"`
	RequiredEvidence           []string `json:"required_evidence"`
}

func buildReleaseReadiness() releaseReadiness {
	return releaseReadiness{
		Level:                      "stable",
		FCCRequired:                true,
		FCCStatus:                  "verified",
		MockUpstreamRequired:       true,
		MockUpstreamStatus:         "verified",
		LiveSmokeRequiredForStable: true,
		LiveSmokeStatus:            "verified",
		Reason:                     "FCC and mock upstream/contract tests are verified; recorded live E2E evidence (docs/E2E-ACCEPTANCE-REPORT.md: 2026-06-17 commit author/scope query + commit diff verified live against GitLab EE 18.8.10, atop the 2026-06-12 full-suite run of 89 PASS / 1 SKIP-by-design plus manual keyring and confirm-chain smoke) supports stable.",
		RequiredEvidence: []string{
			"functional_contract_coverage_100",
			"mock_upstream_contract_tests",
			"recorded_live_smoke_for_stable",
		},
	}
}

func releaseReadinessCheckStatus() string {
	switch buildReleaseReadiness().Level {
	case "stable":
		return "pass"
	case "beta":
		return "warn"
	default:
		return "fail"
	}
}

func releaseReadinessCheckFix() string {
	switch buildReleaseReadiness().Level {
	case "stable":
		return ""
	case "beta":
		return "record live smoke/E2E evidence before declaring stable"
	default:
		return "close FCC and mock upstream coverage gaps before publishing"
	}
}
