package internal

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"

	"golang.org/x/xerrors"
	"google.golang.org/api/googleapi"
)

const (
	// ExitCodeResultSuccess is the exit code for scceeded builds
	ExitCodeResultSuccess = 0
	// ExitCodeResultUnknown is the exit code for builds with unexpected or unknown statuses.
	ExitCodeResultUnknown = 10
	// ExitCodeResultFailure is the exit code for failed builds
	ExitCodeResultFailure = 11
	// ExitCodeResultInternalError is the exit code for builds failed for internal errors
	ExitCodeResultInternalError = 12
	// ExitCodeResultTimeout is the exit code for timed-out builds
	ExitCodeResultTimeout = 13
	// ExitCodeResultCancelled is the exit code for cancelled builds
	ExitCodeResultCancelled = 14
	// ExitCodeUnexpectedError is the exit code for unexpected errors.
	ExitCodeUnexpectedError = 100
	// ExitCodeConfigurationError is the exit code for configuration errors.
	ExitCodeConfigurationError = 101
	// ExitCodeServiceError is the exit code for service errors such as Google Cloud Platform services.
	ExitCodeServiceError = 102
)

type wrapError struct {
	err     error
	message string
	frame   xerrors.Frame
}

func newWrapError(message string, err error) *wrapError {
	return &wrapError{
		err:     err,
		message: message,
		frame:   xerrors.Caller(2),
	}
}

func (e *wrapError) Unwrap() error {
	return e.err
}

func (e *wrapError) Error() string {
	return e.message
}

func (e *wrapError) Format(s fmt.State, v rune) {
	xerrors.FormatError(e, s, v)
}

func (e *wrapError) FormatError(p xerrors.Printer) error {
	p.Print(e.Error())
	e.frame.Format(p)
	return e.Unwrap()
}

// ConfigError indicates error caused for configuration issues.
type ConfigError struct {
	*wrapError
}

// NewConfigError creates a new ConfigError
func NewConfigError(message string, cause error) error {
	return &ConfigError{
		wrapError: newWrapError(message, cause),
	}
}

// ServiceError indicates error caused for external services like Google Cloud Platform.
type ServiceError struct {
	*wrapError
}

// NewServiceError creates a new ConfigError
func NewServiceError(message string, cause error) error {
	return &ServiceError{
		wrapError: newWrapError(message, cause),
	}
}

// BuildResultError indicates build failures.
type BuildResultError struct {
	BuildID string
	Status  string
}

func (err *BuildResultError) Error() string {
	return fmt.Sprintf("Build %v failed with %v", err.BuildID, err.Status)
}

// NewBuildResultError create a new BuildResultError
func NewBuildResultError(buildID, status string) error {
	return &BuildResultError{
		BuildID: buildID,
		Status:  status,
	}
}

// ExitCodeForError returns the exit code appropriate for the passed error.
func ExitCodeForError(err error) int {
	var e1 *BuildResultError
	if xerrors.As(err, &e1) {
		return ExitCodeForStatus(e1.Status)
	}
	var e2 *ConfigError
	if xerrors.As(err, &e2) {
		return ExitCodeConfigurationError
	}
	var e3 *ServiceError
	if xerrors.As(err, &e3) {
		return ExitCodeServiceError
	}
	return ExitCodeUnexpectedError
}

// ExitCodeForStatus returns the exit code for the build status.
func ExitCodeForStatus(status string) int {
	//   "STATUS_UNKNOWN" - Status of the build is unknown.
	//   "QUEUED" - Build or step is queued; work has not yet begun.
	//   "WORKING" - Build or step is being executed.
	//   "SUCCESS" - Build or step finished successfully.
	//   "FAILURE" - Build or step failed to complete successfully.
	//   "INTERNAL_ERROR" - Build or step failed due to an internal cause.
	//   "TIMEOUT" - Build or step took longer than was allowed.
	//   "CANCELLED" - Build or step was canceled by a user.
	if status == "SUCCESS" {
		return ExitCodeResultSuccess
	}
	if status == "FAILURE" {
		return ExitCodeResultFailure
	}
	if status == "INTERNAL_ERROR" {
		return ExitCodeResultInternalError
	}
	if status == "TIMEOUT" {
		return ExitCodeResultTimeout
	}
	if status == "CANCELLED" {
		return ExitCodeResultCancelled
	}
	return ExitCodeResultUnknown
}

func isIgnorableGcsError(err error) bool {
	if err == nil {
		return true
	}

	if xerrors.Is(err, storage.ErrObjectNotExist) {
		return true
	}

	var apiError *googleapi.Error
	if !xerrors.As(err, &apiError) {
		return false
	}
	// We can ignore 404 (the log file isn't ready yet) and 416 (no new contents)
	if apiError.Code == http.StatusNotFound || apiError.Code == http.StatusRequestedRangeNotSatisfiable {
		return true
	}
	return false
}

// IsRetryableError returns whether the error makes sense to retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var e1 *ConfigError
	if xerrors.As(err, &e1) {
		return false
	}

	if xerrors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var apiError *googleapi.Error
	if xerrors.As(err, &apiError) {
		// Retry for 429 (Too Many Requests) and server side errors.
		return apiError.Code == http.StatusTooManyRequests || apiError.Code >= 500
	}

	// Unknown errors are retryable
	return true
}

// Backoff calculates sleep time for back off
type Backoff struct {
	attempt       int
	nextSleepMsec int
	maxSleepMsec  int
}

// NewBackoff returns the default backoff configuration
func NewBackoff() *Backoff {
	return &Backoff{
		attempt:       1,
		nextSleepMsec: 100,
		maxSleepMsec:  5000,
	}
}

// Sleep sleeps with backoff algorithm.
func (b *Backoff) Sleep() {
	time.Sleep(time.Duration(b.nextSleepMsec) * time.Millisecond)
	b.attempt++
	b.nextSleepMsec *= 2
	if b.nextSleepMsec > b.maxSleepMsec {
		b.nextSleepMsec = b.maxSleepMsec
	}
}

// Attempt returns the current attempt count.
func (b *Backoff) Attempt() int {
	return b.attempt
}
