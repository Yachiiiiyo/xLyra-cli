// Package protocol defines the control-plane v1 wire structures, response envelopes,
// and deterministic CLI exit codes shared with xlyra-server.
package protocol

import "strings"

// Deterministic CLI exit codes aligned with docs/xlyra-cli/design.md §4.2.
const (
	ExitOK             = 0 // Success
	ExitInternal       = 1 // Server internal error or UDS transport failure
	ExitInvalid        = 2 // Invalid CLI flags/arguments or server validation failure (400)
	ExitNotFound       = 3 // Target resource not found (404)
	ExitPolicyDenied   = 4 // Policy denied (Tier 0 / external secret / quota exceeded, 403/429)
	ExitApprovalDenied = 5 // User/classifier denied approval or approval timed out
	ExitConflict       = 6 // Ambiguous identifier, revision mismatch, or precondition failed (409)
)

// Control-plane HTTP headers over Unix Domain Socket.
const (
	HeaderProtocol      = "X-Xlyra-Ctl-Protocol"
	HeaderNonce         = "X-Xlyra-Ctl-Nonce"
	HeaderCLIVersion    = "X-Xlyra-Cli-Version"
	HeaderServerVersion = "X-Xlyra-Server-Version"
	HeaderSchemaVersion = "X-Xlyra-Ctl-Schema-Version"
)

// CallRequest is the unified request payload sent to POST http://ctl.local/v1/call.
type CallRequest struct {
	Protocol  int            `json:"protocol"`
	Resource  string         `json:"resource"`
	Action    string         `json:"action"`
	Target    string         `json:"target,omitempty"`
	SubTarget string         `json:"sub_target,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	Payload   any            `json:"payload,omitempty"`
	Options   CallOptions    `json:"options,omitempty"`
}

// CallOptions carries cross-cutting execution modifiers parsed from global CLI flags.
type CallOptions struct {
	DryRun  bool     `json:"dry_run,omitempty"`
	Fields  []string `json:"fields,omitempty"`
	IfMatch string   `json:"if_match,omitempty"`
}

// Envelope is the unified response structure returned by both ctlserver and xlyra-cli.
type Envelope struct {
	OK    bool         `json:"ok"`
	Data  any          `json:"data,omitempty"`
	Diff  []DiffItem   `json:"diff,omitempty"`
	Meta  *Meta        `json:"meta,omitempty"`
	Error *ErrorDetail `json:"error,omitempty"`
}

// DiffItem represents a single configuration path change computed on write or --dry-run.
type DiffItem struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// Meta describes execution tier, optimistic-locking revision, pagination, and side effects.
type Meta struct {
	Tier           int      `json:"tier,omitempty"`
	DryRun         bool     `json:"dry_run"`
	Revision       string   `json:"revision,omitempty"`
	Truncated      bool     `json:"truncated"`
	NextPage       any      `json:"next_page,omitempty"`
	AppliedEffects []string `json:"applied_effects,omitempty"`
}

// ErrorDetail carries a structured machine- and LLM-readable failure description.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// NewSuccessEnvelope builds a positive response envelope with optional data.
func NewSuccessEnvelope(data any) Envelope {
	return Envelope{
		OK:   true,
		Data: data,
	}
}

// NewErrorEnvelope builds a structured failure response envelope.
func NewErrorEnvelope(code, message, field, hint string) Envelope {
	return Envelope{
		OK: false,
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
			Field:   field,
			Hint:    hint,
		},
	}
}

// ExitCodeForResponse maps an HTTP status code and response envelope to a deterministic CLI exit code.
func ExitCodeForResponse(httpStatus int, env *Envelope) int {
	if env != nil && env.OK && (httpStatus == 0 || (httpStatus >= 200 && httpStatus < 300)) {
		return ExitOK
	}
	if env != nil && env.Error != nil {
		switch strings.ToLower(strings.TrimSpace(env.Error.Code)) {
		case "approval_denied", "approval_timeout", "user_denied", "classifier_denied":
			return ExitApprovalDenied
		case "policy_denied", "forbidden", "rate_limited", "quota_exceeded":
			return ExitPolicyDenied
		case "not_found":
			return ExitNotFound
		case "conflict", "ambiguous", "ambiguous_identifier", "revision_mismatch", "precondition_failed":
			return ExitConflict
		case "invalid_argument", "invalid_body", "invalid_request", "validation_error", "bad_request":
			return ExitInvalid
		}
	}
	switch httpStatus {
	case 400, 422:
		return ExitInvalid
	case 404:
		return ExitNotFound
	case 403, 429:
		return ExitPolicyDenied
	case 409, 412:
		return ExitConflict
	default:
		return ExitInternal
	}
}
