package response

type Viewer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email,omitempty"`
}

type Authorization struct {
	URL string `json:"authorizeUrl"`
}
