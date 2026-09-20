package domain

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrForbidden    = errors.New("operation is forbidden")
	ErrConflict     = errors.New("resource conflict")
	ErrValidation   = errors.New("domain validation failed")
	ErrOAuth        = errors.New("oauth provider failed")
	ErrRefreshReuse = errors.New("refresh token was reused")
	ErrRateLimited  = errors.New("request rate limit exceeded")
)
