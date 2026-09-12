package seer

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
