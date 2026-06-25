package output

import (
	"encoding/json"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/contract"
)

// allErrorCodes enumerates every ErrorCode this tool can emit. Keep in sync with
// the const block in json.go; the conformance test asserts each is part of the
// canonical fleet contract (contract/contract.json, single-sourced from the
// ai-native-cli-spec template) with the exact exit code and retryability.
var allErrorCodes = []ErrorCode{
	ErrConfig, ErrAuth, ErrForbidden, ErrNotFound, ErrConflict,
	ErrRateLimit, ErrServer, ErrValidation, ErrConfirmRequired, ErrCancelled,
	ErrTimeout, ErrNetwork, ErrIntegrity, ErrIO, ErrInterrupted, ErrUnknown,
}

// independentCodeTable is a hardcoded expected table of the 16 core error codes.
// It is intentionally NOT derived from contract.Codes so it can catch a wrong
// contract.json or a wrong Codes entry — the delegating assertion above cannot.
// Canonical mapping: E_USAGE/E_VALIDATION=2; E_NOT_FOUND=3;
// E_AUTH/E_FORBIDDEN/E_CONFIG=4; E_CONFIRMATION_REQUIRED=5; E_CONFLICT=6;
// E_NETWORK/E_RATE_LIMITED/E_SERVER=7 (retryable); E_TIMEOUT=8 (retryable);
// E_INTEGRITY/E_IO/E_UNKNOWN=1; E_INTERRUPTED=130 (retryable).
var independentCodeTable = map[string]struct {
	exit      int
	retryable bool
}{
	"E_USAGE":                 {exit: 2, retryable: false},
	"E_VALIDATION":            {exit: 2, retryable: false},
	"E_NOT_FOUND":             {exit: 3, retryable: false},
	"E_AUTH":                  {exit: 4, retryable: false},
	"E_FORBIDDEN":             {exit: 4, retryable: false},
	"E_CONFIG":                {exit: 4, retryable: false},
	"E_CONFIRMATION_REQUIRED": {exit: 5, retryable: false},
	"E_CONFLICT":              {exit: 6, retryable: false},
	"E_NETWORK":               {exit: 7, retryable: true},
	"E_RATE_LIMITED":          {exit: 7, retryable: true},
	"E_SERVER":                {exit: 7, retryable: true},
	"E_TIMEOUT":               {exit: 8, retryable: true},
	"E_INTEGRITY":             {exit: 1, retryable: false},
	"E_IO":                    {exit: 1, retryable: false},
	"E_UNKNOWN":               {exit: 1, retryable: false},
	"E_INTERRUPTED":           {exit: 130, retryable: true},
}

// TestContractConformance_ErrorCodes asserts every emitted error code is in the
// canonical contract (core ∪ this tool's ext) with the exact exit + retryable.
// This is the CI-red guard against the drift the fleet audit found (misnamed
// codes, wrong exit-code mappings).
func TestContractConformance_ErrorCodes(t *testing.T) {
	for _, c := range allErrorCodes {
		spec, ok := contract.Codes[string(c)]
		if !ok {
			t.Errorf("error code %q is not in the canonical contract (core∪ext)", c)
			continue
		}
		if got := ExitCodeForErrorCode(c); got != spec.Exit {
			t.Errorf("exit drift for %q: tool=%d contract=%d", c, got, spec.Exit)
		}
		if got := RetryableErrorCode(c); got != spec.Retryable {
			t.Errorf("retryable drift for %q: tool=%v contract=%v", c, got, spec.Retryable)
		}
	}
}

// TestContractConformance_IndependentTable asserts the 16 core codes against a
// hardcoded expected table that does NOT delegate to contract.Codes, catching a
// wrong contract.json entry that the delegation-based assertion cannot detect.
func TestContractConformance_IndependentTable(t *testing.T) {
	for code, want := range independentCodeTable {
		if got := ExitCodeForErrorCode(ErrorCode(code)); got != want.exit {
			t.Errorf("ExitCodeForErrorCode(%q)=%d want %d", code, got, want.exit)
		}
		if got := RetryableErrorCode(ErrorCode(code)); got != want.retryable {
			t.Errorf("RetryableErrorCode(%q)=%v want %v", code, got, want.retryable)
		}
	}
}

func TestContractConformance_SchemaVersion(t *testing.T) {
	if SchemaVersion != contract.SchemaVersion {
		t.Fatalf("schema_version drift: output=%q contract=%q", SchemaVersion, contract.SchemaVersion)
	}
}

// TestContractConformance_EnvelopeKeys asserts the success and error envelopes
// (and meta) carry only the canonical top-level keys, catching extra/renamed
// fields (e.g. a stray meta.timestamp). The success envelope is built with a
// non-empty data payload so "data" is present and the success_keys requirement
// (which includes "data") is actually exercised.
func TestContractConformance_EnvelopeKeys(t *testing.T) {
	checkEnvelopeKeys(t, SuccessEnvelope(map[string]any{"x": 1}), contract.SuccessEnvelopeKeys, "success")
	checkEnvelopeKeys(t, ErrorEnvelope("m", 0, ErrValidation), contract.ErrorEnvelopeKeys, "error")
}

func checkEnvelopeKeys(t *testing.T, env Envelope, canonical []string, label string) {
	t.Helper()
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal %s envelope: %v", label, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("unmarshal %s envelope: %v", label, err)
	}
	// Flag UNEXPECTED top-level keys.
	for k := range top {
		if !contains(canonical, k) {
			t.Errorf("%s envelope has unexpected top-level key %q (canonical: %v)", label, k, canonical)
		}
	}
	// All canonical keys must be present (data/error are omitempty but still
	// required by the contract when a success/error result is emitted).
	for _, req := range canonical {
		if _, ok := top[req]; !ok {
			t.Errorf("%s envelope missing required key %q", label, req)
		}
	}
	var meta map[string]json.RawMessage
	if raw, ok := top["meta"]; ok {
		_ = json.Unmarshal(raw, &meta)
	}
	// Every MetaRequiredKey must be PRESENT in meta.
	for _, req := range contract.MetaRequiredKeys {
		if _, ok := meta[req]; !ok {
			t.Errorf("meta missing required key %q (canonical MetaRequiredKeys: %v)", req, contract.MetaRequiredKeys)
		}
	}
	// No key beyond meta_required+meta_optional may appear in meta.
	allowed := append(append([]string{}, contract.MetaRequiredKeys...), contract.MetaOptionalKeys...)
	for k := range meta {
		if !contains(allowed, k) {
			t.Errorf("meta has unexpected key %q (canonical: %v)", k, allowed)
		}
	}
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
