package response

import (
	"time"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

type KnowledgeDetail struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Format         string         `json:"format"`
	Source         string         `json:"source"`
	Tags           []string       `json:"tags"`
	LearningStatus string         `json:"learningStatus"`
	Folder         *domain.Folder `json:"folder"`
	Version        int64          `json:"version"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Visibility     string         `json:"visibility"`
	PublicID       *string        `json:"publicId"`
	PublicURL      *string        `json:"publicUrl"`
	Warnings       []Warning      `json:"warnings,omitempty"`
}
type Warning struct {
	Code string `json:"code"`
}

func Detail(k domain.Knowledge, source, appOrigin string, warning bool) KnowledgeDetail {
	tags := k.Tags
	if tags == nil {
		tags = []string{}
	}

	result := KnowledgeDetail{
		ID:             k.ID,
		Title:          k.Title,
		Format:         k.Format,
		Source:         source,
		Tags:           tags,
		LearningStatus: k.LearningStatus,
		Folder:         k.Folder,
		Version:        k.Version,
		UpdatedAt:      k.UpdatedAt,
		Visibility:     k.Visibility,
		PublicID:       k.PublicID,
	}
	if k.PublicID != nil {
		url := appOrigin + "/public/knowledge/" + *k.PublicID
		result.PublicURL = &url
	}

	if warning || k.HTMLSanitized {
		result.Warnings = []Warning{{Code: "html_sanitized"}}
	}

	return result
}

type KnowledgeSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Format    string    `json:"format"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type CommitResult struct {
	KnowledgeID string `json:"knowledgeId"`
	EditPath    string `json:"editPath"`
}

func Committed(id string) CommitResult {
	return CommitResult{KnowledgeID: id, EditPath: "/knowledge/" + id + "/edit"}
}

type PendingDraft struct {
	DraftID        string         `json:"draftId"`
	Status         string         `json:"status"`
	Format         string         `json:"format"`
	Source         string         `json:"source"`
	Title          string         `json:"title"`
	Tags           []string       `json:"tags"`
	LearningStatus string         `json:"learningStatus"`
	Folder         *domain.Folder `json:"folder"`
	Version        int64          `json:"version"`
}

func Draft(d domain.Draft, source string) any {
	if d.CommittedID != "" {
		return struct {
			DraftID string `json:"draftId"`
			Status  string `json:"status"`
			CommitResult
		}{d.DraftID, "committed", Committed(d.CommittedID)}
	}

	tags := d.Tags
	if tags == nil {
		tags = []string{}
	}

	return PendingDraft{
		DraftID:        d.DraftID,
		Status:         "pending",
		Format:         d.Format,
		Source:         source,
		Title:          d.Title,
		Tags:           tags,
		LearningStatus: d.LearningStatus,
		Folder:         d.Folder,
		Version:        d.Version,
	}
}
