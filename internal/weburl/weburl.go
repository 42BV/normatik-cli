// Package weburl builds normatik-ui frontend routes for the CLI's --url flag.
//
// Every function is a pure string substitution against the mapping table in
// normatik-plans/checkpoints/cli-url-flag.md (CR-besluit 3): a bare numeric
// ID, or for work-item-types the slug. No query-params, no free user-text,
// no response-dependent conditional logic — the URL form is deliberately
// simplified to keep the injection/escaping surface structurally absent.
//
// Functions return a PATH, not a full URL. Base-URL resolution (--profile /
// NORMATIK_BASE_URL / active-profile precedence, exit 78 on a missing base)
// is a separate concern (U2). httpx.CanonicalSiteURL already guarantees the
// resolved base has no trailing slash, and every path here starts with "/",
// so the caller's join is plain string concatenation (base + path) with no
// edge case left to reproduce or test in this package. Returning paths also
// keeps weburl free of any dependency on config/httpx/auth: every builder
// below is a zero-setup pure function.
//
// No slug generation: AdminWorkItemType takes a slug the caller already has
// (from a command arg or a response field). A Go-side slug reimplementation
// was considered and rejected — see the CR's "Verworpen alternatieven"
// (three-way drift risk against SlugUtils.java / createPageSlug).
package weburl

import "fmt"

// ---- id-only paths ----

// Page returns the page detail route. Reused by every mapping-table entry
// that resolves to a page: pages get/render/create/update/move, pages
// describe-properties --page, pages revisions snapshot/start/transition/
// restore, pages images list/upload, pages restriction *, macros usage,
// trash restore, archive restore, and work-items * — all of these print this same
// /pages/{id} URL, so they call Page directly instead of getting their own
// builder.
func Page(id int64) string { return fmt.Sprintf("/pages/%d", id) }

// PageVersions returns the page revision-list route (pages revisions list).
func PageVersions(id int64) string { return fmt.Sprintf("/pages/%d/versions", id) }

// PageAttachments returns the page attachments route (pages attachments upload).
func PageAttachments(id int64) string { return fmt.Sprintf("/pages/%d/attachments", id) }

// PageSortChildren returns the sort-children route (pages sort-children).
func PageSortChildren(id int64) string { return fmt.Sprintf("/pages/%d/sort-children", id) }

// PageType returns the page-type detail route: page-types
// get/create/update/move/available-descriptors/chain-link-options,
// property-descriptors create/sort, and pages describe-properties
// --page-type all resolve here.
func PageType(id int64) string { return fmt.Sprintf("/page-types/%d", id) }

// AdminUser returns the admin user detail route (users
// get/create/update/reactivate). Never the /edit variant — see CR besluit 3.
func AdminUser(id int64) string { return fmt.Sprintf("/admin/users/%d", id) }

// AdminGroup returns the admin group detail route (groups
// get/create/update/activate/deactivate/members *).
func AdminGroup(id int64) string { return fmt.Sprintf("/admin/groups/%d", id) }

// AdminDomainEnum returns the admin domain-enum detail route (domain-enums
// get/create/update/usages).
func AdminDomainEnum(id int64) string { return fmt.Sprintf("/admin/domain-enums/%d", id) }

// ---- slug path ----

// AdminWorkItemType returns the admin work-item-type detail route
// (work-item-types get/create/update/transitions *) — the only slug-keyed
// route in the mapping table, never numeric ID. The caller supplies the
// slug; this function does not validate or encode it.
func AdminWorkItemType(slug string) string { return "/admin/work-item-types/" + slug }

// ---- static list URLs ----

// Pages returns the page list/tree route (pages list, pages tree).
func Pages() string { return "/pages" }

// PageTypes returns the page-type list route (page-types list/delete/find).
func PageTypes() string { return "/page-types" }

// AdminUsers returns the admin user list route (users list/search/delete).
func AdminUsers() string { return "/admin/users" }

// AdminGroups returns the admin group list route (groups list/search).
func AdminGroups() string { return "/admin/groups" }

// AdminDomainEnums returns the admin domain-enum list route (domain-enums
// list/delete).
func AdminDomainEnums() string { return "/admin/domain-enums" }

// AdminWorkItemTypes returns the admin work-item-type list route
// (work-item-types list/delete).
func AdminWorkItemTypes() string { return "/admin/work-item-types" }

// AdminTrash returns the admin trash route (trash list/purge).
func AdminTrash() string { return "/admin/trash" }

// AdminArchive returns the admin archive route (archive list).
func AdminArchive() string { return "/admin/archive" }

// AdminArchivePage returns the archived-page read view route
// (archive show / future UI ArchivedPageView).
func AdminArchivePage(id int64) string { return fmt.Sprintf("/admin/archive/%d", id) }

// AdminAudit returns the admin audit route (audit search). Never
// /admin/audit/{id} — that route does not exist (documented bug,
// Admin/links.ts:160-167).
func AdminAudit() string { return "/admin/audit" }

// AdminMacroScan returns the admin macro-scan route (macros scan <name>);
// the macro name is not part of the URL.
func AdminMacroScan() string { return "/admin/macro-scan" }

// AdminLanding returns the admin landing-settings route (landing-settings
// get/update).
func AdminLanding() string { return "/admin/landing" }

// AdminWorkflowRoles returns the admin workflow-roles route (workflow-roles *).
func AdminWorkflowRoles() string { return "/admin/workflow-roles" }

// WorkflowReview returns the reviewer queue route (workflow review;
// role-gated REVIEWER).
func WorkflowReview() string { return "/workflow/review" }

// WorkflowPublish returns the publisher queue route (workflow publish;
// role-gated PUBLISHER).
func WorkflowPublish() string { return "/workflow/publish" }

// WorkflowDrafts returns the drafts queue route (workflow drafts;
// role-gated CONTRIBUTOR+).
func WorkflowDrafts() string { return "/workflow/drafts" }
