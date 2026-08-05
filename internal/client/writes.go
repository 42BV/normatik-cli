package client

import (
	"context"
	"net/http"

	"github.com/42BV/normatik-cli/internal/api"
)

// post/put wrappers return the raw response body (often the updated resource or
// diagnostics); delete/bodyless ops usually return an empty 204 body.

// ---- Users ----

func (c *Client) CreateExternalUser(ctx context.Context, f api.ExternalUserForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateUser(ctx, f) })
}
func (c *Client) UpdateUser(ctx context.Context, id int64, f api.UserForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateUser(ctx, id, f) })
}
func (c *Client) DeleteUser(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteUser(ctx, id) })
}
func (c *Client) ReactivateUser(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ReactivateUser(ctx, id) })
}
func (c *Client) PermanentDeleteUser(ctx context.Context, id int64, f api.PermanentDeleteForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PermanentlyDeleteUser(ctx, id, f) })
}

// ---- Groups ----

func (c *Client) CreateGroup(ctx context.Context, f api.UserGroupForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateGroup(ctx, f) })
}
func (c *Client) UpdateGroup(ctx context.Context, id int64, f api.UserGroupForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateGroup(ctx, id, f) })
}
func (c *Client) ActivateGroup(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ActivateGroup(ctx, id) })
}
func (c *Client) DeactivateGroup(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeactivateGroup(ctx, id) })
}
func (c *Client) AddGroupMember(ctx context.Context, id int64, f api.AddMemberForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.AddGroupMember(ctx, id, f) })
}
func (c *Client) RemoveGroupMember(ctx context.Context, id, userID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RemoveGroupMember(ctx, id, userID) })
}

// ---- Page types ----

func (c *Client) CreatePageType(ctx context.Context, f api.PageTypeForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreatePageType(ctx, f) })
}
func (c *Client) UpdatePageType(ctx context.Context, id int64, f api.PageTypeForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdatePageType(ctx, id, f) })
}
func (c *Client) DeletePageType(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeletePageType(ctx, id) })
}
func (c *Client) MovePageType(ctx context.Context, id int64, f api.PageTypeMoveForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.MovePageType(ctx, id, f) })
}

// ---- Property descriptors ----

func (c *Client) CreatePropertyDescriptor(ctx context.Context, pageTypeID int64, f api.PropertyDescriptorForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreatePropertyDescriptor(ctx, pageTypeID, f) })
}
func (c *Client) UpdatePropertyDescriptor(ctx context.Context, id int64, f api.PropertyDescriptorForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdatePropertyDescriptor(ctx, id, f) })
}
func (c *Client) DeletePropertyDescriptor(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeletePropertyDescriptor(ctx, id) })
}
func (c *Client) SwapPropertyDescriptors(ctx context.Context, fromID, toID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.SwapPropertyDescriptors(ctx, fromID, toID) })
}
func (c *Client) SortPropertyDescriptors(ctx context.Context, pageTypeID int64, ids []int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.SortPageTypePropertyDescriptors(ctx, pageTypeID, ids) })
}
func (c *Client) SortDisplayColumns(ctx context.Context, descriptorID int64, ids []int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SortPropertyDescriptorDisplayColumns(ctx, descriptorID, ids)
	})
}

// ---- Domain enums ----

func (c *Client) CreateDomainEnum(ctx context.Context, f api.DomainEnumForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateDomainEnum(ctx, f) })
}
func (c *Client) UpdateDomainEnum(ctx context.Context, id int64, f api.DomainEnumForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateDomainEnum(ctx, id, f) })
}
func (c *Client) DeleteDomainEnum(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteDomainEnum(ctx, id) })
}

// ---- Work items ----

func (c *Client) CreateWorkItem(ctx context.Context, pageID int64, f api.WorkItemCreateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateWorkItem(ctx, pageID, f) })
}
func (c *Client) EditWorkItem(ctx context.Context, pageID, workItemID int64, f api.WorkItemEditForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateWorkItem(ctx, pageID, workItemID, f) })
}
func (c *Client) DeleteWorkItem(ctx context.Context, pageID, workItemID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteWorkItem(ctx, pageID, workItemID) })
}
func (c *Client) CommentWorkItem(ctx context.Context, pageID, workItemID int64, f api.WorkItemCommentForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.AddWorkItemComment(ctx, pageID, workItemID, f) })
}
func (c *Client) TransitionWorkItem(ctx context.Context, pageID, workItemID int64, f api.WorkItemTransitionForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PerformWorkItemTransition(ctx, pageID, workItemID, f) })
}

// ---- Work item types ----

func (c *Client) CreateWorkItemType(ctx context.Context, f api.WorkItemTypeCreateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateWorkItemType(ctx, f) })
}
func (c *Client) UpdateWorkItemType(ctx context.Context, slug string, f api.WorkItemTypeUpdateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateWorkItemType(ctx, slug, f) })
}
func (c *Client) DeleteWorkItemType(ctx context.Context, slug string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteWorkItemType(ctx, slug) })
}
func (c *Client) AddWorkItemTransition(ctx context.Context, typeSlug string, f api.WorkItemTransitionCreateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreateWorkItemTypeTransition(ctx, typeSlug, f) })
}
func (c *Client) UpdateWorkItemTransition(ctx context.Context, typeSlug string, transitionID int64, f api.WorkItemTransitionUpdateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UpdateWorkItemTypeTransition(ctx, typeSlug, transitionID, f)
	})
}
func (c *Client) RemoveWorkItemTransition(ctx context.Context, typeSlug string, transitionID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteWorkItemTypeTransition(ctx, typeSlug, transitionID) })
}

// ---- Page restrictions ----

func (c *Client) CreateRestriction(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.CreatePageRestriction(ctx, pageID) })
}
func (c *Client) RemoveRestriction(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RemovePageRestriction(ctx, pageID) })
}
func (c *Client) AddAccess(ctx context.Context, pageID int64, f api.PageRestrictionAccessForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.AddPageRestrictionAccess(ctx, pageID, f) })
}
func (c *Client) UpdateAccess(ctx context.Context, pageID, accessID int64, f api.PageRestrictionAccessForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdatePageRestrictionAccess(ctx, pageID, accessID, f) })
}
func (c *Client) RemoveAccess(ctx context.Context, pageID, accessID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RemovePageRestrictionAccess(ctx, pageID, accessID) })
}
func (c *Client) AddGroupAccess(ctx context.Context, pageID int64, f api.PageRestrictionGroupAccessForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.AddPageRestrictionGroupAccess(ctx, pageID, f) })
}
func (c *Client) UpdateGroupAccess(ctx context.Context, pageID, accessID int64, f api.PageRestrictionGroupAccessForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UpdatePageRestrictionGroupAccess(ctx, pageID, accessID, f)
	})
}
func (c *Client) RemoveGroupAccess(ctx context.Context, pageID, accessID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RemovePageRestrictionGroupAccess(ctx, pageID, accessID) })
}

// ---- Workflow roles / landing settings ----

func (c *Client) SetUserWorkflowRole(ctx context.Context, userID int64, f api.WorkflowRoleForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateUserWorkflowRole(ctx, userID, f) })
}
func (c *Client) SetGroupWorkflowRole(ctx context.Context, groupID int64, f api.WorkflowRoleForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateGroupWorkflowRole(ctx, groupID, f) })
}
func (c *Client) UpdateLandingSettings(ctx context.Context, f api.LandingSettingsForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdateLandingSettings(ctx, f) })
}

// ---- Pages: update/delete/move/sort-children ----

func (c *Client) UpdatePage(ctx context.Context, id int64, f api.PageEditForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UpdatePage(ctx, id, f) })
}
func (c *Client) PatchPage(ctx context.Context, id int64, f api.PagePatchForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PatchPage(ctx, id, f) })
}
func (c *Client) DeletePage(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeletePage(ctx, id) })
}
func (c *Client) ArchivePage(ctx context.Context, id int64, f api.ArchivePageForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ArchivePage(ctx, id, f) })
}
func (c *Client) MovePage(ctx context.Context, id int64, f api.PageMoveForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.MovePage(ctx, id, f) })
}
func (c *Client) SortChildren(ctx context.Context, parentID int64, childIDs []int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SortPageChildren(ctx, parentID, api.SortChildrenForm{ChildIds: childIDs})
	})
}

// ---- Revisions / workflow transitions / trash / archive ----

func (c *Client) StartRevision(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.StartPageRevision(ctx, id) })
}
func (c *Client) PerformTransition(ctx context.Context, id int64, f api.TransitionForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.TransitionPageRevision(ctx, id, f) })
}
func (c *Client) RestoreVersion(ctx context.Context, id int64, revisionNumber int32) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RestorePageVersion(ctx, id, revisionNumber) })
}
func (c *Client) RestoreFromTrash(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.RestorePageFromTrash(ctx, pageID) })
}
func (c *Client) PurgeFromTrash(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.PermanentlyDeletePageFromTrash(ctx, pageID) })
}

// UnarchivePage — POST /public/v1/pages/admin/archive/{pageId}/unarchive: the
// server-side name of what the CLI exposes as `archive restore`.
func (c *Client) UnarchivePage(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.UnarchivePage(ctx, pageID) })
}

// DeleteArchivedPage — DELETE /public/v1/pages/admin/archive/{pageId}: moves an
// archived page to trash (admin). CLI command: `archive delete`.
func (c *Client) DeleteArchivedPage(ctx context.Context, pageID int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteArchivedPage(ctx, pageID) })
}
func (c *Client) DiscardWorkingRevision(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DiscardPageWorkingRevision(ctx, id) })
}

// ---- Image / attachment delete ----

func (c *Client) DeleteImage(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.DeleteImage(ctx, id, &api.DeleteImageParams{}) })
}
func (c *Client) DeleteFileAttachment(ctx context.Context, id int64) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.DeleteFileAttachment(ctx, id, &api.DeleteFileAttachmentParams{})
	})
}

// ---- Content validation (dry-run, READ_ONLY-safe) ----

func (c *Client) ValidateContent(ctx context.Context, f api.PublicContentValidationForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) { return c.api.ValidateContent(ctx, f) })
}
