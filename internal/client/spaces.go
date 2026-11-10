package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/42BV/normatik-cli/internal/api"
)

// ---- Modules ----

// ListModules — GET /public/v1/modules.
func (c *Client) ListModules(ctx context.Context) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListModules(ctx) })
}

// UpdateModule — PUT /public/v1/modules/{name} (system administrator).
func (c *Client) UpdateModule(ctx context.Context, name string, enabled bool) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UpdateModule(ctx, name, api.ModuleForm{Enabled: enabled})
	})
}

// ---- Spaces ----

// ListSpaces — GET /public/v1/spaces. all=true lists every space including hidden
// and deleted ones (administrators).
func (c *Client) ListSpaces(ctx context.Context, all bool) ([]byte, *APIError) {
	params := &api.ListSpacesParams{}
	if all {
		params.All = &all
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListSpaces(ctx, params) })
}

func (c *Client) GetSpace(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetSpace(ctx, id) })
}

func (c *Client) CreateSpace(ctx context.Context, f api.SpaceForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateSpace(ctx, f) })
}

func (c *Client) UpdateSpace(ctx context.Context, id int64, f api.SpaceAppearanceForm, token ...string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateSpace(ctx, id, nil, f, withSettingsToken(token)) })
}

func (c *Client) DeleteSpace(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteSpace(ctx, id) })
}

func (c *Client) PurgeSpace(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PurgeSpace(ctx, id) })
}

func (c *Client) MakeSpaceDefault(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.MakeSpaceDefault(ctx, id) })
}

func (c *Client) SetSpaceHidden(ctx context.Context, id int64, hidden bool) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SetSpaceHidden(ctx, id, api.SpaceHiddenForm{Hidden: &hidden})
	})
}

func (c *Client) GetSpaceAccess(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetSpaceAccess(ctx, id) })
}

func (c *Client) UpdateSpaceAccess(ctx context.Context, id int64, f api.SpaceAccessForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateSpaceAccess(ctx, id, f) })
}

func (c *Client) GetSpaceLanding(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetSpaceLanding(ctx, id) })
}

func (c *Client) UpdateSpaceLanding(ctx context.Context, id int64, f api.LandingSettingsForm, token ...string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UpdateSpaceLanding(ctx, id, nil, f, withSettingsToken(token))
	})
}

// ---- Definitions of a space ----

// Definition kinds a space lists and can own.
const (
	DefinitionPageTypes     = "page-types"
	DefinitionEnums         = "enums"
	DefinitionWorkItemTypes = "work-item-types"
)

// ListSpaceDefinitions lists the definitions of one kind available in a space
// (the global ones and those local to it, with origin).
func (c *Client) ListSpaceDefinitions(ctx context.Context, id int64, kind string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		switch kind {
		case DefinitionEnums:
			return c.api.ListSpaceEnums(ctx, id)
		case DefinitionWorkItemTypes:
			return c.api.ListSpaceWorkItemTypes(ctx, id)
		default:
			return c.api.ListSpacePageTypes(ctx, id)
		}
	})
}

// CreateSpacePageType creates a page type local to one space.
func (c *Client) CreateSpacePageType(ctx context.Context, id int64, f api.PageTypeForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateSpacePageType(ctx, id, f) })
}

// CreateSpaceEnum creates an enumeration local to one space.
func (c *Client) CreateSpaceEnum(ctx context.Context, id int64, f api.DomainEnumForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateSpaceEnum(ctx, id, f) })
}

// CreateSpaceWorkItemType creates a work item type local to one space.
func (c *Client) CreateSpaceWorkItemType(ctx context.Context, id int64, f api.WorkItemTypeCreateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateSpaceWorkItemType(ctx, id, f) })
}

// ---- Space resolution ----

// ResolveSpace finds a space by its lowercase key. all=true uses the
// administrator listing, so hidden and deleted spaces resolve too; all=false
// resolves among the spaces the caller can see. ok=false means no space carries
// that key; callers render their own not-found.
func (c *Client) ResolveSpace(ctx context.Context, key string, all bool) (space api.SpaceResult, ok bool, apiErr *APIError) {
	body, apiErr := c.ListSpaces(ctx, all)
	if apiErr != nil {
		return api.SpaceResult{}, false, apiErr
	}
	var spaces []api.SpaceResult
	if err := json.Unmarshal(body, &spaces); err != nil {
		return api.SpaceResult{}, false, fail(err)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, s := range spaces {
		if s.Key == key {
			return s, true, nil
		}
	}
	return api.SpaceResult{}, false, nil
}

// ---- Page move to space ----

// MoveToSpace — POST /public/v1/pages/{id}/move-to-space (needs the Spaces module).
func (c *Client) MoveToSpace(ctx context.Context, id int64, f api.PageMoveToSpaceForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.MoveToSpace(ctx, id, f) })
}

// PreviewMoveToSpace — POST /public/v1/pages/{id}/move-to-space/preview.
func (c *Client) PreviewMoveToSpace(ctx context.Context, id int64, f api.PageMoveToSpaceForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PreviewMoveToSpace(ctx, id, f) })
}

// ---- Bootstrap of the first system administrator ----

// GrantUserSysAdmin — POST /public/v1/users/{id}/sys-admin. The server decides
// who may call it: a system administrator, or an administrator key while no
// system administrator exists.
func (c *Client) GrantUserSysAdmin(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GrantUserSysAdmin(ctx, id) })
}

// ---- Scope support ----

// GetPageSpaceKey returns the lowercase key of the space a page really lives
// in (cheap GET /pages/{id} without macro resolution). The CLI scope check
// compares it with --space: a flag is a scope, never a move.
func (c *Client) GetPageSpaceKey(ctx context.Context, id int64) (string, *APIError) {
	body, apiErr := c.GetPage(ctx, id, nil, false)
	if apiErr != nil {
		return "", apiErr
	}
	var page struct {
		Space string `json:"space"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return "", fail(err)
	}
	return page.Space, nil
}

// PreviewSpaceAccessImpact — GET /public/v1/spaces/{id}/access-impact: how many
// users gain or lose access if this configuration is saved.
func (c *Client) PreviewSpaceAccessImpact(ctx context.Context, id int64, accessMode string, userIDs, groupIDs []int64) ([]byte, *APIError) {
	params := &api.PreviewSpaceAccessImpactParams{AccessMode: api.PreviewSpaceAccessImpactParamsAccessMode(accessMode)}
	if len(userIDs) > 0 {
		params.UserIds = &userIDs
	}
	if len(groupIDs) > 0 {
		params.GroupIds = &groupIDs
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.PreviewSpaceAccessImpact(ctx, id, params) })
}
