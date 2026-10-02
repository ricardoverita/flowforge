package fault

import "errors"

type Kind string

const (
	Validation     Kind = "validation"
	NotFound       Kind = "not_found"
	Conflict       Kind = "conflict"
	Infrastructure Kind = "infrastructure"
	Transient      Kind = "transient"
	Permanent      Kind = "permanent"
)

// Error keeps the stable, public failure classification separate from its cause.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func New(kind Kind, code, message string) error {
	return &Error{Kind: kind, Code: code, Message: message}
}

func Wrap(kind Kind, code, message string, cause error) error {
	return &Error{Kind: kind, Code: code, Message: message, Cause: cause}
}

func KindOf(err error) Kind {
	var f *Error
	if errors.As(err, &f) {
		return f.Kind
	}
	return Infrastructure
}

func Is(err error, kind Kind) bool { return err != nil && KindOf(err) == kind }

// CodeOf returns the stable classification without logging the underlying cause.
func CodeOf(err error) string {
	var f *Error
	if errors.As(err, &f) && f.Code != "" {
		return f.Code
	}
	return "internal_error"
}
