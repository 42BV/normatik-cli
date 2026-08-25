package render

// Data-macro renderers for the rich page renderer: macros whose payload is
// page-external data resolved by the backend into macroData — progress-ring,
// page-tasks, attachments and the file/pdf/image one-liners.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// progressBarWidth is the cell width of the ::progress-ring ASCII bar.
const progressBarWidth = 24

// progressRingNoDataNote is the config/empty payload copy. Reserved for missing
// data or total=0 with excludedUnpublished=0 — not for a valid draft-only count.
const progressRingNoDataNote = "(no data available for this configuration)"

// renderProgressRing renders ::progress-ring from its ProgressRingMacroData
// (total/successCount/percentage/breakdown/excludedUnpublished): an ASCII bar
// with the percentage and success/total counts, plus one breakdown line per
// enum value. Configuration errors stay the no-data note. A valid selection
// with only pages that lack an active revision (total=0, excludedUnpublished>0)
// shows that exclusion instead of the config error. Mixed totals keep the bar
// and append the same exclusion line.
func renderProgressRing(n *dnode, idx *macroIndex, width int) string {
	entry, ok := idx.entryFor("progressRings", n)
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	total := gnum(entry, "total")
	excluded := int(gnum(entry, "excludedUnpublished"))
	if total == 0 && excluded == 0 {
		return ruleHeader("Progress Ring") + "\n" + progressRingNoDataNote
	}
	var b strings.Builder
	if total == 0 {
		b.WriteString(ruleHeader("Progress Ring"))
	} else {
		pct := int(gnum(entry, "percentage"))
		filled := progressBarWidth * pct / 100
		filled = minInt(maxInt(filled, 0), progressBarWidth)
		fmt.Fprintf(&b, "%s%s %d%% (%d/%d)",
			strings.Repeat("█", filled), strings.Repeat("░", progressBarWidth-filled),
			pct, int(gnum(entry, "successCount")), int(total))
		b.WriteString(breakdownLines(garr(entry, "breakdown")))
	}
	if excluded > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(excludedUnpublishedNote(excluded))
	}
	return b.String()
}

func excludedUnpublishedNote(n int) string {
	if n == 1 {
		return "1 page has no active revision"
	}
	return fmt.Sprintf("%d pages have no active revision", n)
}

// breakdownLines renders the per-enum-value legend below the progress bar:
// a colored bullet (colorClass carries the raw DomainEnumValue.color hex, may
// be null — neutral bullet then, like the web's FALLBACK_COLOR), the value
// label, the count and a ✓ on values in the configured success-set.
func breakdownLines(items []any) string {
	labelW := 0
	for _, it := range items {
		if bm, ok := it.(map[string]any); ok {
			if w := displayWidth(gstr(bm, "value")); w > labelW {
				labelW = w
			}
		}
	}
	var b strings.Builder
	for _, it := range items {
		bm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		value := gstr(bm, "value")
		pad := strings.Repeat(" ", labelW-displayWidth(value))
		fmt.Fprintf(&b, "\n%s %s%s  %d", colorLabel("●", gstr(bm, "colorClass")), value, pad, int(gnum(bm, "count")))
		if gbool(bm, "isSuccess") {
			b.WriteString("  ✓")
		}
	}
	return b.String()
}

// pageTaskFieldLabels mirrors PageTasksRenderer.tsx SUPPORTED_FIELDS +
// FIELD_LABELS; unknown fields-entries are dropped, like the web.
var pageTaskFieldLabels = map[string]string{
	"status":      "Status",
	"description": "Description",
	"assignees":   "Assignees",
	"dueDate":     "Due date",
	"transitions": "Transitions",
	"actions":     "Actions",
	"log":         "Log",
}

// pageTaskFields mirrors PageTasksRenderer.tsx parseFields + getVisibleFields:
// no fields attr → description only; description is always included (appended
// when the attr omits it); unsupported names are dropped.
func pageTaskFields(raw string) []string {
	var parsed []string
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		if _, ok := pageTaskFieldLabels[f]; ok {
			parsed = append(parsed, f)
		}
	}
	if len(parsed) == 0 {
		return []string{"description"}
	}
	for _, f := range parsed {
		if f == "description" {
			return parsed
		}
	}
	return append(parsed, "description")
}

// renderPageTasks renders ::page-tasks{type=..} from macroData.pageTasks
// (keyed by the trimmed type slug; entry = PageTasksMacroData with items of
// PageWorkItemResult). fields/widths attrs pick and size the columns like the
// web renderer. Interactive columns stay visible but are explicitly marked as
// web-only, so the CLI preserves the configured table shape without implying
// that actions can be executed from rendered content.
func renderPageTasks(n *dnode, idx *macroIndex, width int) string {
	entry, ok := idx.entryFor("pageTasks", n)
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	items := garr(entry, "items")
	if len(items) == 0 {
		return "(no tasks of type " + strings.TrimSpace(n.attrs["type"]) + ")"
	}
	fields := pageTaskFields(n.attrs["fields"])
	headers := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = pageTaskFieldLabels[f]
	}
	var rows [][]string
	for _, it := range items {
		im, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = pageTaskCell(f, im, idx)
		}
		rows = append(rows, row)
	}
	var colW []int
	if n.attrs["widths"] != "" {
		colW = columnWidths(n.attrs["widths"], len(fields), width-4*len(fields))
	}
	return asciiTable(headers, rows, colW)
}

// pageTaskCell renders one work-item cell: status as a colored pill
// (currentStatusLabel/currentStatusColor, same style as the enum pill),
// description with inline macros resolved, assignees as a comma-joined
// displayName list, dueDate as the raw ISO date.
func pageTaskCell(field string, im map[string]any, idx *macroIndex) string {
	switch field {
	case "description":
		return renderInline(gstr(im, "description"), idx)
	case "status":
		label := gstr(im, "currentStatusLabel")
		if label == "" {
			return "—"
		}
		return pill(label, gstr(im, "currentStatusColor"))
	case "assignees":
		var names []string
		for _, it := range garr(im, "assignees") {
			if am, ok := it.(map[string]any); ok {
				if name := gstr(am, "displayName"); name != "" {
					names = append(names, name)
				}
			}
		}
		if len(names) == 0 {
			return "—"
		}
		return strings.Join(names, ", ")
	case "dueDate":
		if due := gstr(im, "dueDate"); due != "" {
			return due
		}
		return "—"
	case "actions":
		return "Web only"
	}
	return "—" // transitions / log: interactive or web-only columns
}

// renderAttachments renders ::attachments as a bordered table over ALL
// entries in macroData.fileAttachments — the resolver loads every attachment
// of the page when the directive is present, and file/pdf merge into the same
// map (MacroData.mergeFileAttachments). Rows sort by id ascending, the
// iteration order the web's Object.values inherits from numeric JSON keys.
func renderAttachments(_ *dnode, idx *macroIndex, _ int) string {
	mm := idx.dataMap("fileAttachments")
	if len(mm) == 0 {
		return "(no attachments)"
	}
	keys := make([]string, 0, len(mm))
	for k := range mm {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, errA := strconv.Atoi(keys[i])
		b, errB := strconv.Atoi(keys[j])
		if errA == nil && errB == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	var rows [][]string
	for _, k := range keys {
		am, ok := mm[k].(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, []string{gstr(am, "filename"), gstr(am, "contentType"), formatFileSize(gnum(am, "size"))})
	}
	return asciiTable([]string{"Name", "Type", "Size"}, rows, nil)
}

// formatFileSize mirrors the web's formatFileSize (AttachmentsRenderer.tsx /
// FileRenderer.tsx): bytes < 1 KiB verbatim, then one decimal in KB/MB.
func formatFileSize(bytes float64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", int64(bytes))
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", bytes/1024)
	}
	return fmt.Sprintf("%.1f MB", bytes/(1024*1024))
}

// renderFile renders ::file{id=..} as a compact one-line download label.
func renderFile(n *dnode, idx *macroIndex, width int) string {
	return renderAttachmentLine(n, idx, width, "")
}

// renderPdf renders ::pdf{id=..} like ::file, labelled as PDF — the CLI never
// embeds a viewer (no extra HTTP calls; the web streams a blob iframe).
func renderPdf(n *dnode, idx *macroIndex, width int) string {
	return renderAttachmentLine(n, idx, width, "PDF · ")
}

// renderAttachmentLine is the shared ::file / ::pdf path: an id-keyed lookup
// in fileAttachments → "📎 [PDF · ]filename (size)". A miss (unknown id) keeps
// the existing labelled placeholder.
func renderAttachmentLine(n *dnode, idx *macroIndex, width int, label string) string {
	entry, ok := idx.entryFor("fileAttachments", n)
	if !ok {
		return renderUnknownMacro(n, idx, width)
	}
	name := gstr(entry, "filename")
	if name == "" {
		name = "attachment " + n.attrs["id"]
	}
	line := "📎 " + label + name
	if size := gnum(entry, "size"); size > 0 {
		line += " (" + formatFileSize(size) + ")"
	}
	return line
}

// renderImage renders ::image{id=..} as a labelled placeholder with the
// filename from macroData.images (plus the alt attr when present) — the image
// is never downloaded or pixel-rendered. A miss mirrors the web's
// "Image #{id} not found" placeholder (ImageRenderer.tsx).
func renderImage(n *dnode, idx *macroIndex, width int) string {
	id := n.attrs["id"]
	entry, ok := idx.entryFor("images", n)
	if !ok {
		if id == "" {
			return renderUnknownMacro(n, idx, width)
		}
		return "🖼 (image #" + id + " not found)"
	}
	label := joinNonEmpty(" — ", gstr(entry, "filename"), n.attrs["alt"])
	return "🖼 [image: " + label + "]"
}
