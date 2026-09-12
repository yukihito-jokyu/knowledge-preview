package domain

import "errors"

var (
	ErrNotFound   = errors.New("resource not found")
	ErrForbidden  = errors.New("operation is forbidden")
	ErrConflict   = errors.New("resource conflict")
	ErrValidation = errors.New("domain validation failed")
)
