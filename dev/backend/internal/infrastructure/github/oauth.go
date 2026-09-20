package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
	"golang.org/x/oauth2"
)

type OAuth struct {
	config  *oauth2.Config
	userURL string
	client  *http.Client
}

func New(clientID, clientSecret, redirectURL, authorizeURL, tokenURL, userURL string) *OAuth {
	return &OAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  authorizeURL,
				TokenURL: tokenURL,
			},
			RedirectURL: redirectURL,
			Scopes:      []string{"read:user", "user:email"},
		},
		userURL: userURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *OAuth) AuthorizationURL(state, challenge string) string {
	return o.config.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (usecase.OAuthUser, error) {
	exchangeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	exchangeCtx = context.WithValue(exchangeCtx, oauth2.HTTPClient, o.client)

	token, err := o.config.Exchange(exchangeCtx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return usecase.OAuthUser{}, err
	}

	request, err := http.NewRequestWithContext(exchangeCtx, http.MethodGet, o.userURL, nil)
	if err != nil {
		return usecase.OAuthUser{}, err
	}

	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	request.Header.Set("Accept", "application/vnd.github+json")

	response, err := o.client.Do(request)
	if err != nil {
		return usecase.OAuthUser{}, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return usecase.OAuthUser{}, fmt.Errorf("github user endpoint returned %d", response.StatusCode)
	}

	var user struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil || user.ID == 0 {
		return usecase.OAuthUser{}, fmt.Errorf("invalid github user response")
	}

	display := user.Name
	if display == "" {
		display = user.Login
	}

	return usecase.OAuthUser{ID: fmt.Sprint(user.ID), DisplayName: display, Email: user.Email}, nil
}
