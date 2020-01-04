package internal

import (
	"fmt"

	"golang.org/x/xerrors"
)

const (
	// ExitCodeBuildFailure is the exit code for build failures.
	ExitCodeBuildFailure = 1
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
}

func (e *wrapError) Unwrap() error {
	return e.err
}

func (e *wrapError) Error() string {
	return e.message
}

// ConfigError indicates error caused for configuration issues.
type ConfigError struct {
	*wrapError
}

// NewConfigError creates a new ConfigError
func NewConfigError(message string, cause error) error {
	return &ConfigError{
		wrapError: &wrapError{
			err:     cause,
			message: message,
		},
	}
}

// ServiceError indicates error caused for external services like Google Cloud Platform.
type ServiceError struct {
	*wrapError
}

// NewServiceError creates a new ConfigError
func NewServiceError(message string, cause error) error {
	return &ServiceError{
		wrapError: &wrapError{
			err:     cause,
			message: message,
		},
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
	if xerrors.Is(err, &BuildResultError{}) {
		return ExitCodeBuildFailure
	}
	if xerrors.Is(err, &ConfigError{}) {
		return ExitCodeConfigurationError
	}
	if xerrors.Is(err, &ServiceError{}) {
		return ExitCodeServiceError
	}
	return ExitCodeUnexpectedError
}
