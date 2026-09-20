package github

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExchangeUsesContextTimeoutForTokenProvider(t *testing.T) {
	oauth := New(
		"client",
		"secret",
		"https://oauth.test/callback",
		"https://oauth.test/authorize",
		"https://oauth.test/token",
		"https://oauth.test/user",
	)
	oauth.client = &http.Client{Transport: blockingTransport{}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := oauth.Exchange(ctx, "code", "verifier")
	require.Error(t, err)
}

type blockingTransport struct{}

func (blockingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}
