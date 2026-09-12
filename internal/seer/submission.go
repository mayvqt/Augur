package seer

import (
	"errors"
	"net/http"
)

// A dispatched POST has no idempotency key. Losing its acknowledgement does not
// prove rejection; the caller must not offer an automatic or one-click retry.
type submissionError struct {
	message string
	cause   error
}

func (e *submissionError) Error() string       { return e.message }
func (e *submissionError) UserMessage() string { return e.message }
func (e *submissionError) SafeToRetry() bool   { return false }
func (e *submissionError) Unwrap() error       { return e.cause }

var ErrSubmissionUnknown = &submissionError{message: "Seerr may have accepted your request, but its response could not be confirmed. Check `/requests` or Seerr before starting another request."}

// IsDefiniteRejection is only used to discard a saved mutation intent when
// Seerr explicitly rejects it. Transport and server failures remain uncertain.
func IsDefiniteRejection(err error) bool {
	var response *responseError
	return errors.As(err, &response) && response.statusCode >= 400 && response.statusCode < 500 && response.statusCode != http.StatusRequestTimeout
}
