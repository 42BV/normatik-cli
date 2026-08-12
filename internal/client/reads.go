package client

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/42BV/normatik-cli/internal/api"
)

// ---- Users ----

func (c *Client) ListUsers(ctx context.Context, status string, page, size int, sort []string) ([]byte, *APIError) {
	params := &api.ListUsersParams{}
	if status != "" {
		s := api.ListUsersParamsStatus(status)
		params.Status = &s
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListUsers(ctx, params, pageableEditor(page, size, sort))
	})
}

func (c *Client) SearchUsers(ctx context.Context, query string, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SearchUsers(ctx, &api.SearchUsersParams{Query: query}, pageableEditor(page, size, sort))
	})
}

func (c *Client) GetUser(ctx context.Context, id int64, expand []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetUser(ctx, id, &api.GetUserParams{}, expandEditor(expand))
	})
}

// ---- Groups ----

func (c *Client) ListGroups(ctx context.Context, status string, page, size int, sort []string) ([]byte, *APIError) {
	params := &api.ListGroupsParams{}
	if status != "" {
		s := api.ListGroupsParamsStatus(status)
		params.Status = &s
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListGroups(ctx, params, pageableEditor(page, size, sort))
	})
}

func (c *Client) SearchGroups(ctx context.Context, query string, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SearchGroups(ctx, &api.SearchGroupsParams{Query: query}, pageableEditor(page, size, sort))
	})
}

func (c *Client) GetGroup(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetGroup(ctx, id) })
}

// ---- Page types ----

func (c *Client) ListPageTypes(ctx context.Context) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListPageTypes(ctx) })
}

func (c *Client) GetPageType(ctx context.Context, id int64, expand []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetPageType(ctx, id, &api.GetPageTypeParams{}, expandEditor(expand))
	})
}

func (c *Client) GetAvailablePropertyDescriptors(ctx context.Context, pageTypeID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetPageTypePropertyDescriptors(ctx, pageTypeID) })
}

func (c *Client) GetChainLinkOptions(ctx context.Context, pageTypeID, pageID int64) ([]byte, *APIError) {
	params := &api.GetPageTypeChainLinkOptionsParams{}
	if pageID > 0 {
		params.PageId = &pageID
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetPageTypeChainLinkOptions(ctx, pageTypeID, params) })
}

// ---- Property descriptors ----

func (c *Client) GetPropertyDescriptor(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetPropertyDescriptor(ctx, id) })
}

// ---- Work item types ----

func (c *Client) ListWorkItemTypes(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListWorkItemTypes(ctx, &api.ListWorkItemTypesParams{}, pageableEditor(page, size, sort))
	})
}

func (c *Client) GetWorkItemType(ctx context.Context, slug string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetWorkItemType(ctx, slug) })
}

// ---- Work items (scoped under a page) ----

func (c *Client) ListWorkItems(ctx context.Context, pageID int64, typ string) ([]byte, *APIError) {
	params := &api.ListWorkItemsParams{}
	if typ != "" {
		params.Type = &typ
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListWorkItems(ctx, pageID, params) })
}

func (c *Client) GetWorkItem(ctx context.Context, pageID, workItemID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetWorkItem(ctx, pageID, workItemID) })
}

func (c *Client) GetWorkItemTransitions(ctx context.Context, pageID, workItemID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListWorkItemTransitions(ctx, pageID, workItemID) })
}

// ---- Domain enums ----

func (c *Client) ListDomainEnums(ctx context.Context, expand []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListDomainEnums(ctx, &api.ListDomainEnumsParams{}, expandEditor(expand))
	})
}

func (c *Client) GetDomainEnum(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetDomainEnum(ctx, id) })
}

func (c *Client) GetDomainEnumUsages(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetDomainEnumUsages(ctx, id) })
}

// ---- Workflow roles (bounded picker) ----

func (c *Client) GetWorkflowRoles(ctx context.Context, include string) ([]byte, *APIError) {
	params := &api.GetWorkflowRolesParams{}
	if include != "" {
		inc := api.GetWorkflowRolesParamsInclude(include)
		params.Include = &inc
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetWorkflowRoles(ctx, params) })
}

// ---- Workflow queues ----

func (c *Client) ListReviewQueue(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetWorkflowReviewItems(ctx, &api.GetWorkflowReviewItemsParams{}, pageableEditor(page, size, sort))
	})
}

func (c *Client) ListPublishQueue(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetWorkflowPublishItems(ctx, &api.GetWorkflowPublishItemsParams{}, pageableEditor(page, size, sort))
	})
}

func (c *Client) ListDraftsQueue(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetWorkflowDraftItems(ctx, &api.GetWorkflowDraftItemsParams{}, pageableEditor(page, size, sort))
	})
}

// ---- Landing settings / trash / archive / content macros ----

func (c *Client) GetLandingSettings(ctx context.Context) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetLandingSettings(ctx) })
}

func (c *Client) ListTrash(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListTrash(ctx, &api.ListTrashParams{}, pageableEditor(page, size, sort))
	})
}

// ListArchive — GET /public/v1/admin/archive: the archived-pages overview
// (admin). Paged exactly like ListTrash; every row carries the archive reason.
func (c *Client) ListArchive(ctx context.Context, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListArchive(ctx, &api.ListArchiveParams{}, pageableEditor(page, size, sort))
	})
}

// GetArchivedPage — GET /public/v1/pages/admin/archive/{pageId}: admin read view
// of a single archived page (ArchivedPageViewResult with nested page detail).
func (c *Client) GetArchivedPage(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetArchivedPage(ctx, pageID) })
}

// GetTrashedPage — GET /public/v1/pages/admin/trash/{pageId}: admin read view
// of a single trashed page (TrashedPageViewResult with nested page detail).
func (c *Client) GetTrashedPage(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetTrashedPage(ctx, pageID) })
}

// ---- Cascade impact previews ----

func (c *Client) PreviewPageCascadeImpact(ctx context.Context, id int64, operation string) ([]byte, *APIError) {
	params := &api.PreviewPageCascadeImpactParams{
		Operation: api.PreviewPageCascadeImpactParamsOperation(operation),
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.PreviewPageCascadeImpact(ctx, id, params)
	})
}

func (c *Client) PreviewAdminArchiveCascadeImpact(ctx context.Context, pageID int64, operation string, includeSubtree *bool) ([]byte, *APIError) {
	params := &api.PreviewAdminArchiveCascadeImpactParams{IncludeSubtree: includeSubtree}
	if operation != "" {
		op := api.PreviewAdminArchiveCascadeImpactParamsOperation(operation)
		params.Operation = &op
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.PreviewAdminArchiveCascadeImpact(ctx, pageID, params)
	})
}

func (c *Client) PreviewAdminTrashCascadeImpact(ctx context.Context, pageID int64, operation string, includeSubtree *bool) ([]byte, *APIError) {
	params := &api.PreviewAdminTrashCascadeImpactParams{
		Operation:      api.PreviewAdminTrashCascadeImpactParamsOperation(operation),
		IncludeSubtree: includeSubtree,
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.PreviewAdminTrashCascadeImpact(ctx, pageID, params)
	})
}

func (c *Client) ListContentMacros(ctx context.Context, context string) ([]byte, *APIError) {
	params := &api.ListContentMacrosParams{}
	if context != "" {
		ctxv := api.ListContentMacrosParamsContext(context)
		params.Context = &ctxv
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListContentMacros(ctx, params) })
}

// GetContentMacroDocs — GET /public/v1/content-macros/docs: the full macro
// knowledge base (preamble + shared filter syntax + per-macro documentation).
func (c *Client) GetContentMacroDocs(ctx context.Context) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetContentMacroDocs(ctx) })
}

// GetContentMacroDocsByName — GET /public/v1/content-macros/{name}/docs: one
// macro's documentation (directiveName as key; 404 CONTENT_MACRO_NOT_FOUND).
func (c *Client) GetContentMacroDocsByName(ctx context.Context, name string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetContentMacroDocsByName(ctx, name) })
}

// ScanMacroUsage — GET /public/v1/content-macros/{macroName}/scan: pages that use
// a macro (access-aware; requires an admin API key). Returns a MacroScanResult.
func (c *Client) ScanMacroUsage(ctx context.Context, macroName string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ScanContentMacroUsage(ctx, macroName) })
}

// ---- Release notes ----

// ListReleaseNotes — GET /public/v1/release-notes: paged release-note summaries
// (version + date). The server enforces a fixed semver-desc order and rejects a
// sort param (400 INVALID_SORT), so this exposes only page/size — never sort.
func (c *Client) ListReleaseNotes(ctx context.Context, page, size int) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListReleaseNotes(ctx, &api.ListReleaseNotesParams{}, pageableEditor(page, size, nil))
	})
}

// GetReleaseNoteByVersion — GET /public/v1/release-notes/{version}: one release
// note's detail (version + date + markdown body; 404 RELEASE_NOTE_NOT_FOUND).
func (c *Client) GetReleaseNoteByVersion(ctx context.Context, version string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetReleaseNoteByVersion(ctx, version) })
}

// ---- Audit log (flat query form via editor) ----

// AuditFilters holds the audit-search filters; empty fields are omitted.
type AuditFilters struct {
	Search, ActionType, Actor, EntityType, From, To string
	EntityID                                        int64
	Include                                         []string
}

func auditEditor(f AuditFilters) api.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		q := req.URL.Query()
		set := func(k, v string) {
			if v != "" {
				q.Set(k, v)
			}
		}
		set("search", f.Search)
		set("actionType", f.ActionType)
		set("actor", f.Actor)
		set("entityType", f.EntityType)
		set("from", f.From)
		set("to", f.To)
		if f.EntityID > 0 {
			q.Set("entityId", strconv.FormatInt(f.EntityID, 10))
		}
		for _, inc := range f.Include {
			if inc != "" {
				q.Add("include", inc)
			}
		}
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

func (c *Client) SearchAuditLog(ctx context.Context, f AuditFilters, page, size int, sort []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.ListAuditLog(ctx, &api.ListAuditLogParams{}, auditEditor(f), pageableEditor(page, size, sort))
	})
}

// ---- Pages: tree + revisions ----

func (c *Client) GetPagesTree(ctx context.Context, ifNoneMatch string) (body []byte, notModified bool, err *APIError) {
	params := &api.GetPageTreeParams{}
	if ifNoneMatch != "" {
		params.IfNoneMatch = &ifNoneMatch
	}
	return c.DoConditional(func() (*http.Response, error) { return c.api.GetPageTree(ctx, params) })
}

func (c *Client) ListRevisions(ctx context.Context, id int64, compare string) ([]byte, *APIError) {
	params := &api.GetPageRevisionsParams{}
	if compare != "" {
		params.Compare = &compare
	}
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetPageRevisions(ctx, id, params) })
}

func (c *Client) GetRevisionSnapshot(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.GetPageRevisionSnapshot(ctx, id) })
}

// ---- Images / attachments (downloads are binary + ETag/304) ----

func (c *Client) ListPageImages(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ListPageImages(ctx, pageID) })
}

func (c *Client) DownloadImage(ctx context.Context, id int64, ifNoneMatch string) (io.ReadCloser, bool, *APIError) {
	params := &api.DownloadImageParams{}
	if ifNoneMatch != "" {
		params.IfNoneMatch = &ifNoneMatch
	}
	return c.DoDownload(func() (*http.Response, error) { return c.api.DownloadImage(ctx, id, params) })
}

func (c *Client) DownloadAttachment(ctx context.Context, id int64, ifNoneMatch string) (io.ReadCloser, bool, *APIError) {
	params := &api.DownloadFileAttachmentParams{}
	if ifNoneMatch != "" {
		params.IfNoneMatch = &ifNoneMatch
	}
	return c.DoDownload(func() (*http.Response, error) { return c.api.DownloadFileAttachment(ctx, id, params) })
}
