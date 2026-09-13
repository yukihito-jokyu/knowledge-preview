package domain

import (
	"errors"
	"regexp"
	"time"
)

const (
	MaxSourceBytes       = 10 * 1024 * 1024
	MaxVersion     int64 = 9007199254740991
)

var (
	ErrBadRequest      = errors.New("bad request")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrTooLarge        = errors.New("payload too large")
	ErrUnavailable     = errors.New("dependency unavailable")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidID(id string) bool { return uuidPattern.MatchString(id) }

type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}
type ValidationError struct{ Detail FieldError }

func (e *ValidationError) Error() string { return "domain validation failed" }
func (e *ValidationError) Unwrap() error { return ErrValidation }

type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Knowledge struct {
	ID             string
	OwnerID        string
	Title          string
	Format         string
	Tags           []string
	LearningStatus string
	Folder         *Folder
	Version        int64
	UpdatedAt      time.Time
	Visibility     string
	PublicID       *string
	SourceKey      string
	HTMLKey        string
	HTMLSanitized  bool
}
type Draft struct {
	Knowledge
	DraftID     string
	CommittedID string
	CommitHash  string
}
type Principal struct {
	OwnerID   string
	SessionID string
}
type PreviewGrant struct {
	Hash        string
	OwnerID     string
	SessionID   string
	KnowledgeID string
	Version     int64
}
