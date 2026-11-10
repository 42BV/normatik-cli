package client

import (
	"context"
	"net/http"

	"github.com/42BV/normatik-cli/internal/api"
)

const SettingsTokenHeader = "X-Normatik-Settings-Token"

// SettingsSnapshot binds a read body to the server's conditional-write token.
type SettingsSnapshot struct {
	Body  []byte
	Token string
}

func (c *Client) settingsSnapshot(call LowLevel) (SettingsSnapshot, *APIError) {
	var token string
	body, apiErr := c.DoRaw(func() (*http.Response, error) {
		response, err := call()
		if response != nil {
			token = response.Header.Get(SettingsTokenHeader)
		}
		return response, err
	})
	if apiErr != nil {
		return SettingsSnapshot{}, apiErr
	}
	if token == "" {
		return SettingsSnapshot{}, decodeError(http.StatusPreconditionRequired, []byte(`{"status":428,"errorCode":"UPDATE_PRECONDITION_REQUIRED","title":"Conditional update unavailable","detail":"The server returned no settings token; the update was not sent.","hint":"Use a server that supports conditional settings updates, then retry."}`))
	}
	return SettingsSnapshot{Body: body, Token: token}, nil
}

func (c *Client) GetSpaceSettingsSnapshot(ctx context.Context, id int64) (SettingsSnapshot, *APIError) {
	return c.settingsSnapshot(func() (*http.Response, error) { return c.api.GetSpace(ctx, id) })
}

func (c *Client) GetSpaceLandingSnapshot(ctx context.Context, id int64) (SettingsSnapshot, *APIError) {
	return c.settingsSnapshot(func() (*http.Response, error) { return c.api.GetSpaceLanding(ctx, id) })
}

func (c *Client) GetLandingSettingsSnapshot(ctx context.Context, space string) (SettingsSnapshot, *APIError) {
	return c.settingsSnapshot(func() (*http.Response, error) {
		return c.api.GetLandingSettings(ctx, &api.GetLandingSettingsParams{Space: spaceParam(space)})
	})
}

func withSettingsToken(tokens []string) api.RequestEditorFn {
	return func(_ context.Context, request *http.Request) error {
		if len(tokens) > 0 {
			request.Header.Set(SettingsTokenHeader, tokens[0])
		}
		return nil
	}
}
