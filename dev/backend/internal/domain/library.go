package domain

import (
	"strings"
	"unicode/utf8"
)

const (
	DefaultPageSize = 10
	MaxPageSize     = 100
	MaxSearchRunes  = 200
	MaxFolderRunes  = 100
)

type ListQuery struct {
	Query    string
	Tags     []string
	FolderID *string
	Page     int
	PageSize int
}

type KnowledgePage struct {
	Items    []Knowledge
	Total    int
	Page     int
	PageSize int
}

type Folder struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
	Version  int64   `json:"version"`
}

func NormalizeListQuery(q ListQuery) (ListQuery, error) {
	q.Query = strings.TrimSpace(q.Query)
	if utf8.RuneCountInString(q.Query) > MaxSearchRunes {
		return q, ErrBadRequest
	}

	if q.Page < 1 {
		return q, ErrBadRequest
	}

	if q.PageSize == 0 {
		q.PageSize = DefaultPageSize
	}

	if q.PageSize < 1 || q.PageSize > MaxPageSize {
		return q, ErrBadRequest
	}

	if q.Page-1 > int(^uint(0)>>1)/q.PageSize {
		return q, ErrBadRequest
	}

	for i := range q.Tags {
		q.Tags[i] = strings.TrimSpace(q.Tags[i])
		if q.Tags[i] == "" {
			return q, ErrBadRequest
		}
	}

	return q, nil
}

func ValidateFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > MaxFolderRunes {
		return "", &ValidationError{Detail: FieldError{Field: "name", Reason: "invalid_length"}}
	}

	return name, nil
}
