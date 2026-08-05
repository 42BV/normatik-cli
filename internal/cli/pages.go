package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/render"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

func newPagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "Work with pages (list, get, render, search, create, update, delete, move, sort-children, tree, describe-properties, revisions, images, attachments, restriction)",
		Long:  "Commands for pages in the Normatik wiki. Every error carries an errorCode + hint.",
		RunE:  command.UnknownSub,
	}
	cmd.AddCommand(newPagesListCmd())
	cmd.AddCommand(newPagesGetCmd())
	cmd.AddCommand(newPagesRenderCmd())
	cmd.AddCommand(newPagesSearchCmd())
	addWriteCommands(cmd, newPagesCreateCmd())
	cmd.AddCommand(newPagesDescribePropertiesCmd())
	cmd.AddCommand(newPagesTreeCmd())
	cmd.AddCommand(newPagesRevisionsCmd())
	cmd.AddCommand(newPagesImagesCmd())
	cmd.AddCommand(newPagesAttachmentsCmd())
	addWriteCommands(cmd, newPagesRestrictionCmd())
	addPagesWrites(cmd) // update/delete/move/sort-children
	return cmd
}

func newPagesTreeCmd() *cobra.Command {
	var inm string
	cmd := &cobra.Command{
		Use:     "tree",
		Short:   "Fetch the page tree (supports ETag/If-None-Match → 304)",
		Example: "  normatik pages tree\n  normatik pages tree --if-none-match '\"abc123\"'",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, notModified, apiErr := d.Client.GetPagesTree(cmd.Context(), inm)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages tree")
			}
			if command.PrintURL(d, cmd, weburl.Pages()) {
				return nil
			}
			if notModified {
				d.Printer.Message("304 Not Modified — the tree has not changed.")
				return nil
			}
			d.Printer.List(body) // tree-node velden afgeleid; volledige boom via --output json
			return nil
		},
	}
	cmd.Flags().StringVar(&inm, "if-none-match", "", "ETag for a conditional GET (304 if unchanged)")
	command.URLFlag(cmd)
	return cmd
}

func newPagesRevisionsCmd() *cobra.Command {
	rc := &cobra.Command{Use: "revisions", Short: "Revisions of a page (list, snapshot, start, transition, restore)", RunE: command.UnknownSub}
	var compare string
	list := &cobra.Command{
		Use: "list <id>", Short: "Version list (--compare a,b for diff)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.ListRevisions(cmd.Context(), id, compare)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages revisions list")
			}
			if command.PrintURL(d, cmd, weburl.PageVersions(id)) {
				return nil
			}
			d.Printer.Raw(body)
			return nil
		},
	}
	list.Flags().StringVar(&compare, "compare", "", "compare two revision numbers, e.g. --compare 2,3")
	command.URLFlag(list)
	snap := &cobra.Command{
		Use: "snapshot <id>", Short: "Current working-revision snapshot", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.GetRevisionSnapshot(cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages revisions snapshot")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			d.Printer.Raw(body)
			return nil
		},
	}
	command.URLFlag(snap)
	rc.AddCommand(list, snap)
	addRevisionsWrites(rc) // start/transition/restore
	return rc
}

func newPagesImagesCmd() *cobra.Command {
	ic := &cobra.Command{Use: "images", Short: "Images of a page (list, upload, download, delete)", RunE: command.UnknownSub}
	list := &cobra.Command{
		Use: "list <pageId>", Short: "List images of a page", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.ListPageImages(cmd.Context(), id)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages images list")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			d.Printer.List(body)
			return nil
		},
	}
	command.URLFlag(list)
	var out, inm string
	dl := &cobra.Command{
		Use: "download <imageId>", Short: "Download an image (binary)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return writeBinary(cmd, "normatik pages images download", out, func(d *command.Deps) (io.ReadCloser, bool, *client.APIError) {
				return d.Client.DownloadImage(cmd.Context(), id, inm)
			})
		},
	}
	dl.Flags().StringVar(&out, "output-file", "", "write to file (otherwise stdout)")
	dl.Flags().StringVar(&inm, "if-none-match", "", "ETag for a conditional download")
	del := idWrite("delete <imageId>", "Delete an image", "normatik pages images delete", "Image deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeleteImage(ctx, id)
		})
	up := uploadCmd("upload <pageId> <file>", "Upload an image to a page", "normatik pages images upload", weburl.Page,
		func(d *command.Deps, ctx context.Context, pageID int64, file string) ([]byte, *client.APIError) {
			return d.Client.UploadPageImage(ctx, pageID, file)
		})
	ic.AddCommand(list, dl)
	addWriteCommands(ic, del, up)
	return ic
}

func newPagesAttachmentsCmd() *cobra.Command {
	ac := &cobra.Command{Use: "attachments", Short: "Attachments of a page (upload, download, delete)", RunE: command.UnknownSub}
	var out, inm string
	dl := &cobra.Command{
		Use: "download <attachmentId>", Short: "Download an attachment (binary)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := idArg(cmd, args[0])
			if err != nil {
				return command.Handled(2)
			}
			return writeBinary(cmd, "normatik pages attachments download", out, func(d *command.Deps) (io.ReadCloser, bool, *client.APIError) {
				return d.Client.DownloadAttachment(cmd.Context(), id, inm)
			})
		},
	}
	dl.Flags().StringVar(&out, "output-file", "", "write to file (otherwise stdout)")
	dl.Flags().StringVar(&inm, "if-none-match", "", "ETag for a conditional download")
	del := idWrite("delete <attachmentId>", "Delete an attachment", "normatik pages attachments delete", "Attachment deleted.", "soft",
		func(d *command.Deps, ctx context.Context, id int64) ([]byte, *client.APIError) {
			return d.Client.DeleteFileAttachment(ctx, id)
		})
	up := uploadCmd("upload <pageId> <file>", "Upload an attachment to a page", "normatik pages attachments upload", weburl.PageAttachments,
		func(d *command.Deps, ctx context.Context, pageID int64, file string) ([]byte, *client.APIError) {
			return d.Client.UploadAttachment(ctx, pageID, file)
		})
	ac.AddCommand(dl)
	addWriteCommands(ac, del, up)
	return ac
}

// uploadCmd builds a "upload <pageId> <file>" subcommand that POSTs a file via
// multipart and renders the resulting attachment/image metadata. urlPath builds
// the --url output from the pageId — never the response's own "url" field,
// which is a backend file-attachment URL, not a frontend route (CR edge-semantics).
func uploadCmd(use, short, invocation string, urlPath func(int64) string, fn func(*command.Deps, context.Context, int64, string) ([]byte, *client.APIError)) *cobra.Command {
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			id, perr := command.ParseID(args[0])
			if perr != nil {
				d.Printer.Message("Error [USAGE]: <pageId> must be a number, got %q", args[0])
				return command.Handled(2)
			}
			// NORMATIK-21: lstat (volgt de laatste symlink niet) en weiger niet-reguliere
			// bestanden vroeg — symlink/FIFO/device/dir komen niet tot netwerkverkeer.
			if fi, statErr := os.Lstat(args[1]); statErr != nil || !fi.Mode().IsRegular() {
				d.Printer.Message("Error [USAGE]: file %q not found, or not a regular file (symlinks and special files are refused)", args[1])
				return command.Handled(2)
			}
			body, apiErr := fn(d, cmd.Context(), id, args[1])
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, invocation)
			}
			if command.PrintURL(d, cmd, urlPath(id)) {
				return nil
			}
			d.Printer.Raw(body, "id", "filename", "size", "contentType", "url")
			return nil
		},
	}
	command.URLFlag(cmd)
	return cmd
}

func newPagesCreateCmd() *cobra.Command {
	var name, content, file, timezone string
	var pageTypeID, parentID int64
	var props, unsets []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new page (flags, --property, or -f form.json with propertyValues[])",
		Long: "Create a new page (flags, --property, or -f form.json with propertyValues[]).\n" +
			"--url performs the write first, then prints only the new page's frontend URL (built\n" +
			"from the response id) instead of the normal output — not to be confused with\n" +
			"`login --url` (the environment site URL used to authenticate; a different flag,\n" +
			"different meaning).",
		Example: "  normatik pages create --name \"My page\" --page-type-id 3\n" +
			"  normatik pages create --name X --page-type-id 3 --property \"Status=Approved\" --property \"Owner=email:a@b.nl\"\n" +
			"      # Owner is a USER_LIST property: pass email:<addr> for an account or ext:<name> for an external user (there is no separate 'user' type)\n" +
			"  normatik pages create -f page.json              # full PageCreateForm incl. propertyValues\n" +
			"  normatik pages create -f page.json --name X     # flags supplement/override the form\n" +
			"  open $(normatik pages create --name X --page-type-id 3 --url)  # create, then open the new page (write executes first; URL uses the response id)\n" +
			"  normatik pages describe-properties --page-type 3  # discover settable properties",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			if err := command.CheckURLDryRun(d, cmd); err != nil {
				return err
			}
			// Base: het -f form.json (behoudt propertyValues[]), anders een leeg formulier.
			form := api.PageCreateForm{}
			if file != "" {
				loaded, lerr := loadForm[api.PageCreateForm](file)
				if lerr != nil {
					d.Printer.Message("Error [USAGE]: could not read form: %v", lerr)
					return command.Handled(2)
				}
				form = loaded
			}
			// Top-level flags vullen aan/overschrijven; propertyValues blijven onaangeroerd.
			if cmd.Flags().Changed("name") {
				form.Name = name
			}
			if cmd.Flags().Changed("page-type-id") {
				form.PageTypeId = pageTypeID
			}
			if cmd.Flags().Changed("parent") {
				form.ParentId = &parentID
			}
			if cmd.Flags().Changed("content") {
				form.Content = &content
			}
			if form.Name == "" || form.PageTypeId == 0 {
				d.Printer.Message("Error [USAGE]: name and page-type-id are required (via flags or -f form.json)")
				return command.Handled(2)
			}
			hasProps := len(props) > 0 || len(unsets) > 0
			hasFormProps := len(derefPropertyValues(form.PropertyValues)) > 0
			if dr, _ := cmd.Flags().GetBool("dry-run"); dr {
				// Combination rule: with --content we validate the markdown first
				// (unchanged /content/validate behaviour); when there are property
				// values — from --property flags OR from a -f form — we resolve the
				// payload client-side and print it. Never a write.
				if form.Content == nil && !hasProps && !hasFormProps {
					d.Printer.Message("Error [USAGE]: --dry-run needs --content/-f content and/or --property/-f propertyValues to preview")
					return command.Handled(2)
				}
				if form.Content != nil {
					if err := validateContent(cmd, d, *form.Content, nil, i64Ptr(form.PageTypeId), false); err != nil {
						return err
					}
				}
				if hasProps || hasFormProps {
					// Flags need name->id dispatch; a form without flags already carries
					// resolved propertyValues, so it is printed as-is (payload-faithful).
					resolved := form
					if hasProps {
						r, rerr := resolveCreateProperties(cmd, d, form, props, unsets, timezone)
						if rerr != nil {
							return rerr
						}
						resolved = r
					}
					d.Printer.DryRun(resolved)
				}
				return nil
			}
			// --property / --unset-property merge onto the form's propertyValues
			// using the page type's effective metadata for name->id dispatch.
			if hasProps {
				resolved, rerr := resolveCreateProperties(cmd, d, form, props, unsets, timezone)
				if rerr != nil {
					return rerr
				}
				form = resolved
			}
			res, apiErr := d.Client.CreatePageForm(cmd.Context(), form)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages create")
			}
			// --url wins over the normal response render, using the newly created
			// page's own id from the response (never a caller-supplied id: create
			// has none).
			var created struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(res, &created)
			if command.PrintURL(d, cmd, weburl.Page(created.ID)) {
				return nil
			}
			d.Printer.Raw(res, "id", "name", "pageTypeName", "parentId")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "page name (required unless via -f)")
	cmd.Flags().Int64Var(&pageTypeID, "page-type-id", 0, "page type id (required unless via -f)")
	cmd.Flags().Int64Var(&parentID, "parent", 0, "parent page id (optional)")
	cmd.Flags().StringVar(&content, "content", "", "markdown content (optional)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "PageCreateForm JSON file (incl. propertyValues[])")
	cmd.Flags().StringArrayVar(&props, "property", nil, "set a property: \"name=value\" (repeatable; see `pages describe-properties`)")
	cmd.Flags().StringArrayVar(&unsets, "unset-property", nil, "remove a property by name (repeatable)")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA zone for DATE_TIME properties (default: host local zone)")
	cmd.Flags().Bool("dry-run", false, "preview without saving: validate --content via /content/validate and/or print the resolved --property payload")
	command.URLFlag(cmd)
	return cmd
}

// derefPropertyValues unwraps a form's optional propertyValues slice.
func derefPropertyValues(pv *[]api.PropertyValueForm) []api.PropertyValueForm {
	if pv == nil {
		return nil
	}
	return *pv
}

// resolveCreateProperties merges --property / --unset-property onto the form's
// propertyValues using the page type's effective metadata for name->id dispatch.
// It only reads (GET page-type metadata + name lookups) — never a write — so it is
// shared by the real create path and the --dry-run preview. On a client-side
// validation failure it renders the error and returns the carried exit code.
func resolveCreateProperties(cmd *cobra.Command, d *command.Deps, form api.PageCreateForm, props, unsets []string, timezone string) (api.PageCreateForm, error) {
	descriptors, apiErr := fetchDescriptors(cmd.Context(), d.Client, form.PageTypeId)
	if apiErr != nil {
		return form, command.RenderError(d.Printer, apiErr, "normatik pages create")
	}
	tz := timezone
	if tz == "" {
		tz = localZoneName()
	}
	pd := propertyDispatcher{lookup: newCachingLookup(newClientLookup(d.Client)), timezone: tz}
	merged, perr := applyPropertyFlags(cmd.Context(), derefPropertyValues(form.PropertyValues), props, unsets, descriptors, pd)
	if perr != nil {
		return form, renderPropError(d, perr)
	}
	form.PropertyValues = &merged
	return form, nil
}

// renderPropError prints a client-side property-flag validation error (code +
// detail + indented extra lines) and returns the carried exit code.
func renderPropError(d *command.Deps, e *propFlagError) error {
	d.Printer.Message("Error [%s]: %s", e.code, e.detail)
	for _, l := range e.extra {
		d.Printer.Message("  %s", l)
	}
	return command.Handled(e.exit)
}

// fetchDescriptors loads a page type's effective property descriptors (the
// create metadata source for --property dispatch and describe-properties).
func fetchDescriptors(ctx context.Context, c *client.Client, pageTypeID int64) ([]api.PropertyDescriptorResult, *client.APIError) {
	body, apiErr := c.GetPageType(ctx, pageTypeID, nil)
	if apiErr != nil {
		return nil, apiErr
	}
	var pt api.PageTypeResult
	if err := json.Unmarshal(body, &pt); err != nil {
		return nil, &client.APIError{Status: 0, Body: body, Malformed: true}
	}
	if pt.PropertyDescriptors == nil {
		return nil, nil
	}
	return *pt.PropertyDescriptors, nil
}

// newPagesDescribePropertiesCmd discovers the settable properties of a page type
// (or a page's page type): name | dataType | writable | lookup-hint.
func newPagesDescribePropertiesCmd() *cobra.Command {
	var pageTypeID, pageID int64
	cmd := &cobra.Command{
		Use:   "describe-properties",
		Short: "Describe a page type's properties (name, dataType, writable, hint, enum values, numeric config, example)",
		Long: "Self-contained discovery for `--property`: each property lists its dataType, " +
			"whether it is writable, a lookup hint, an ENUM's allowed values, a NUMBER's " +
			"min/max/decimals/format, and a ready-to-paste name=value example. Use --output " +
			"json for the structured enumValues/minValue/maxValue/decimals/numberFormat/example fields.",
		Example: "  normatik pages describe-properties --page-type 3\n" +
			"  normatik pages describe-properties --page 42 --output json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			var descriptors []api.PropertyDescriptorResult
			if cmd.Flags().Changed("page") {
				// --page describes the page's effective property set straight from the
				// page response (availablePropertyDescriptors) — the same source the
				// update --property dispatch uses, in a single call.
				page, apiErr := fetchPage(cmd.Context(), d.Client, pageID)
				if apiErr != nil {
					return command.RenderError(d.Printer, apiErr, "normatik pages describe-properties")
				}
				descriptors = derefDescriptors(page.AvailablePropertyDescriptors)
			} else {
				fetched, apiErr := fetchDescriptors(cmd.Context(), d.Client, pageTypeID)
				if apiErr != nil {
					return command.RenderError(d.Printer, apiErr, "normatik pages describe-properties")
				}
				descriptors = fetched
			}
			if cmd.Flags().Changed("page") {
				if command.PrintURL(d, cmd, weburl.Page(pageID)) {
					return nil
				}
			} else if command.PrintURL(d, cmd, weburl.PageType(pageTypeID)) {
				return nil
			}
			// Resolve enum values inline (cached: two props sharing a domain enum
			// trigger one lookup) so the discovery output is self-contained.
			lookup := newCachingLookup(newClientLookup(d.Client))
			rows := describePropertyRows(cmd.Context(), descriptors, lookup)
			body, _ := json.Marshal(rows)
			d.Printer.List(body, "name", "dataType", "writable", "lookupHint", "example")
			return nil
		},
	}
	cmd.Flags().Int64Var(&pageTypeID, "page-type", 0, "page type id")
	cmd.Flags().Int64Var(&pageID, "page", 0, "page id (its page type is described)")
	cmd.MarkFlagsMutuallyExclusive("page-type", "page")
	cmd.MarkFlagsOneRequired("page-type", "page")
	command.URLFlag(cmd)
	return cmd
}

// describePropertyRow is one row of describe-properties output. It is a
// self-contained discovery record: an agent can build a `--property` call from a
// single row without consulting domain-enums get or page-types get. enumValues
// holds an ENUM's allowed display values; minValue/maxValue/decimals/numberFormat
// carry a NUMBER's full config; example is a ready-to-paste "name=value" string.
type describePropertyRow struct {
	Name         string   `json:"name"`
	DataType     string   `json:"dataType"`
	Writable     bool     `json:"writable"`
	LookupHint   string   `json:"lookupHint"`
	EnumValues   []string `json:"enumValues,omitempty"`
	MinValue     *float64 `json:"minValue,omitempty"`
	MaxValue     *float64 `json:"maxValue,omitempty"`
	Decimals     *int32   `json:"decimals,omitempty"`
	NumberFormat string   `json:"numberFormat,omitempty"`
	Example      string   `json:"example,omitempty"`
}

// describePropertyRows builds the discovery rows, resolving each ENUM descriptor's
// allowed values through lookup (cached by the caller). Enum resolution degrades
// gracefully: a failed lookup leaves enumValues empty and keeps the enum name in
// the hint. Numeric config is surfaced verbatim from the descriptor.
func describePropertyRows(ctx context.Context, descriptors []api.PropertyDescriptorResult, lookup propertyLookup) []describePropertyRow {
	rows := make([]describePropertyRow, 0, len(descriptors))
	for _, desc := range descriptors {
		dt := derefDataType(desc.DataType)
		enumValues := describeEnumValues(ctx, lookup, desc)
		row := describePropertyRow{
			Name:       derefStr(desc.Name),
			DataType:   string(dt),
			Writable:   writableDataType(dt),
			LookupHint: describeHint(desc, enumValues),
			EnumValues: enumValues,
			Example:    describeExample(desc, enumValues),
		}
		if dt == api.PropertyDescriptorResultDataTypeNUMBER {
			row.MinValue = f32ToF64Ptr(desc.MinValue)
			row.MaxValue = f32ToF64Ptr(desc.MaxValue)
			if desc.Decimals != nil {
				d := *desc.Decimals
				row.Decimals = &d
			}
			if desc.NumberFormat != nil {
				row.NumberFormat = string(*desc.NumberFormat)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func newPagesListCmd() *cobra.Command {
	var page, size int
	var sort []string
	var pageTypeID int64

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List root pages, or all pages of a page type (--page-type-id)",
		Long: "List pages (paginated). Without --page-type-id this lists the root pages; " +
			"with --page-type-id it lists ALL pages of that page type (not just root pages).",
		Example: "  normatik pages list\n" +
			"  normatik pages list --size 5 --sort name,asc\n" +
			"  normatik pages list --page-type-id 3          # all pages of page type 3\n" +
			"  normatik pages list --page-type-id 3 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			var ptID *int64
			if cmd.Flags().Changed("page-type-id") {
				ptID = &pageTypeID
			}
			list, apiErr := d.Client.ListPages(cmd.Context(), page, size, sort, ptID)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages list")
			}
			if command.PrintURL(d, cmd, weburl.Pages()) {
				return nil
			}
			d.Printer.PageList(list)
			return nil
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (one-based)")
	cmd.Flags().IntVar(&size, "size", 10, "items per page")
	cmd.Flags().StringArrayVar(&sort, "sort", nil, "sort expression, e.g. name,asc (repeatable; whitelisted server-side)")
	cmd.Flags().Int64Var(&pageTypeID, "page-type-id", 0, "filter by page type (lists ALL pages of that type, not just root)")
	command.URLFlag(cmd)
	return cmd
}

func newPagesGetCmd() *cobra.Command {
	var expand []string
	var working bool
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Fetch page detail (property values; --working for the unpublished working revision)",
		Long: "Fetch page detail (property values; --working for the unpublished working revision).\n" +
			"--url prints only this page's frontend URL instead of the normal output — not to be\n" +
			"confused with `login --url` (the environment site URL used to authenticate; a different\n" +
			"flag, different meaning).",
		Args: cobra.ExactArgs(1),
		Example: "  normatik pages get 1\n" +
			"  normatik pages get 1 --working   # the working-revision values (after `pages update` on a workflow page)\n" +
			"  normatik pages get 1 --expand workflow,attachments --output json\n" +
			"  open $(normatik pages get 1 --url)   # print only the frontend URL, piped straight into `open`",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			id, perr := command.ParseID(args[0])
			if perr != nil {
				d.Printer.Message("Error [USAGE]: <id> must be a number, got %q", args[0])
				return command.Handled(2)
			}
			body, apiErr := d.Client.GetPage(cmd.Context(), id, expand)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages get")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			// Human mode shows the identifying summary AND the property values
			// (same formatter as `pages render`); --output json stays raw.
			d.Printer.PageGet(body, working, "id", "name", "pageTypeName", "parentId")
			// Table mode never inlines the page content (that's `pages render` /
			// -o json); point the caller at those routes so content discovery does
			// not require guessing (NORM-odompfug). Suppressed by --quiet like other
			// meta-footer output; stays on stderr so stdout is pipe-clean.
			if d.Printer.Mode != render.JSON && !d.Printer.Quiet {
				d.Printer.Message("Content: normatik pages render %d   (or --output json)", id)
			}
			// --working reshapes only the table read-back; the JSON stays the raw
			// composite (working values live under .workingRevision). Point that out
			// on stderr so the JSON on stdout stays byte-identical and pipe-clean.
			if working && d.Printer.Mode == render.JSON {
				d.Printer.Message("Note: --working affects table output only; in JSON the working values are under .workingRevision")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&expand, "expand", nil, "sections: jira-macros,workflow,attachments,images,work-items,restriction")
	cmd.Flags().BoolVar(&working, "working", false, "show the working-revision property values instead of the published ones (table mode only; JSON always includes .workingRevision)")
	command.URLFlag(cmd)
	return cmd
}

func newPagesRenderCmd() *cobra.Command {
	var plain bool
	cmd := &cobra.Command{
		Use:   "render <id>",
		Short: "Render a page as ASCII with macros resolved (--plain for placeholders)",
		Long: "Render a page as an ASCII document: title, properties and the content with macros\n" +
			"resolved from the composite's macroData (tables, enum pills, pagelinks, children/toc\n" +
			"lists, progress rings). Colours are only emitted on a real terminal — piped or\n" +
			"redirected output stays plain text, and NO_COLOR is honoured.\n" +
			"Use --plain for the placeholder-only rendering ([macro: name], no macro resolution,\n" +
			"no styling). --output json returns the raw composite in both cases.",
		Args: cobra.ExactArgs(1),
		Example: "  normatik pages render 1\n" +
			"  normatik pages render 1 --plain          # placeholder-only ([macro: name])\n" +
			"  normatik pages render 1 --output json    # the full composite",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			id, perr := command.ParseID(args[0])
			if perr != nil {
				d.Printer.Message("Error [USAGE]: <id> must be a number, got %q", args[0])
				return command.Handled(2)
			}
			body, apiErr := d.Client.GetPage(cmd.Context(), id, expandSections)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages render")
			}
			if command.PrintURL(d, cmd, weburl.Page(id)) {
				return nil
			}
			if plain {
				d.Printer.Page(body) // placeholder-only ([macro: name])
			} else {
				d.Printer.PageRich(body) // default: macros resolved from macroData
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "render macro placeholders ([macro: name]) instead of the resolved rich layout; never styled")
	command.URLFlag(cmd)
	return cmd
}

func newPagesSearchCmd() *cobra.Command {
	var page, size int
	cmd := &cobra.Command{
		Use:     "search <query>",
		Short:   "Full-text search across pages",
		Args:    cobra.ExactArgs(1),
		Example: "  normatik pages search norm\n  normatik pages search \"annex a\" --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := command.Build(cmd)
			if err != nil {
				return err
			}
			body, apiErr := d.Client.SearchPages(cmd.Context(), args[0], page, size, nil)
			if apiErr != nil {
				return command.RenderError(d.Printer, apiErr, "normatik pages search")
			}
			d.Printer.List(body, "id", "name", "pageTypeName") // toon de zoekresultaat-rijen, niet alleen paging-meta
			return nil
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (one-based)")
	cmd.Flags().IntVar(&size, "size", 10, "items per page")
	return cmd
}
