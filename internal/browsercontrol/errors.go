package browsercontrol

import "fmt"

// ErrorCode represents a stable machine-readable error category.
type ErrorCode string

const (
	ErrContextNotFound   ErrorCode = "context_not_found"
	ErrPageChanged       ErrorCode = "page_changed"
	ErrElementNotFound   ErrorCode = "element_not_found"
	ErrAmbiguousTarget   ErrorCode = "ambiguous_target"
	ErrInvalidState      ErrorCode = "invalid_state"
	ErrPolicyDenied      ErrorCode = "policy_denied"
	ErrDeadlineExceeded  ErrorCode = "deadline_exceeded"
	ErrCancelled         ErrorCode = "cancelled"
	ErrLimitExceeded     ErrorCode = "limit_exceeded"
	ErrUnsupported       ErrorCode = "unsupported"
	ErrInternal          ErrorCode = "internal"
)

// Error is a typed browser-control error with a stable code and safe message.
type Error struct {
	Code     ErrorCode
	Message  string
	Retryable bool
	Details  map[string]interface{}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Sentinel errors for common cases.
var (
	ErrContextNotFoundSentinel   = &Error{Code: ErrContextNotFound, Message: "context not found or closed", Retryable: true}
	ErrPageChangedSentinel      = &Error{Code: ErrPageChanged, Message: "element reference belongs to an earlier page revision", Retryable: true}
	ErrElementNotFoundSentinel  = &Error{Code: ErrElementNotFound, Message: "element reference cannot be resolved", Retryable: true}
	ErrAmbiguousTargetSentinel  = &Error{Code: ErrAmbiguousTarget, Message: "locator matched multiple elements", Retryable: true}
	ErrInvalidStateSentinel     = &Error{Code: ErrInvalidState, Message: "operation cannot run in current lifecycle state", Retryable: true}
	ErrPolicyDeniedSentinel     = &Error{Code: ErrPolicyDenied, Message: "security policy rejected the action", Retryable: false}
	ErrDeadlineExceededSentinel = &Error{Code: ErrDeadlineExceeded, Message: "operation timed out", Retryable: true}
	ErrCancelledSentinel        = &Error{Code: ErrCancelled, Message: "operation was cancelled", Retryable: true}
	ErrLimitExceededSentinel    = &Error{Code: ErrLimitExceeded, Message: "quota or limit exceeded", Retryable: false}
	ErrUnsupportedSentinel      = &Error{Code: ErrUnsupported, Message: "operation not supported by this version", Retryable: false}
	ErrInternalSentinel         = &Error{Code: ErrInternal, Message: "internal server error", Retryable: false}
)

// NewError creates a typed error with optional details.
func NewError(code ErrorCode, message string, retryable bool, details map[string]interface{}) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Details:   details,
	}
}
