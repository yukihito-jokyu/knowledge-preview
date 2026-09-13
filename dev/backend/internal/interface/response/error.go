package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
)

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Details []domain.FieldError `json:"details,omitempty"`
}

// WriteError はドメインエラーをHTTPレスポンスへ変換して返す。
func WriteError(c *gin.Context, err error) {
	status, body := toAPIError(err)

	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		body.Details = []domain.FieldError{validation.Detail}
	}

	c.AbortWithStatusJSON(status, errorResponse{Error: body})
}

func toAPIError(err error) (int, apiError) {
	switch {
	case errors.Is(err, domain.ErrBadRequest):
		return http.StatusBadRequest, apiError{Code: "bad_request", Message: "invalid request"}
	case errors.Is(err, domain.ErrUnauthenticated):
		return http.StatusUnauthorized, apiError{Code: "unauthenticated", Message: "authentication is required"}
	case errors.Is(err, domain.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, apiError{
			Code:    "payload_too_large",
			Message: "source exceeds the size limit",
		}
	case errors.Is(err, domain.ErrUnavailable):
		return http.StatusServiceUnavailable, apiError{Code: "unavailable", Message: "a dependency is unavailable"}
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, apiError{Code: "not_found", Message: "resource was not found"}
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, apiError{Code: "forbidden", Message: "operation is not permitted"}
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, apiError{Code: "conflict", Message: "resource conflicts with the current state"}
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, apiError{
			Code:    "validation_failed",
			Message: "request violates a domain rule",
		}
	default:
		return http.StatusInternalServerError, apiError{Code: "internal", Message: "an unexpected error occurred"}
	}
}
