package apperr

import (
	"errors"
	"fmt"
)

var (
	ErrConflict     = errors.New("conflict")
	ErrNotFound     = errors.New("not found")
	ErrValidation   = errors.New("validation")
	ErrUnauthorized = errors.New("unauthorized")
)

type DuplicateImageConflictError struct {
	DuplicateImageID int64
	Message          string
}

func (e *DuplicateImageConflictError) Error() string {
	if e == nil {
		return ErrConflict.Error()
	}
	return fmt.Sprintf("%s: %s", ErrConflict, e.Message)
}

func (e *DuplicateImageConflictError) Unwrap() error {
	return ErrConflict
}

func Validationf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

func Conflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

func DuplicateImageConflictf(imageID int64, format string, args ...any) error {
	return &DuplicateImageConflictError{
		DuplicateImageID: imageID,
		Message:          fmt.Sprintf(format, args...),
	}
}

func NotFoundf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNotFound, fmt.Sprintf(format, args...))
}

func Unauthorizedf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnauthorized, fmt.Sprintf(format, args...))
}
