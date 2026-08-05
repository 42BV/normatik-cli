package render

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

var jiraAssetHTMLTagRe = regexp.MustCompile(`(?i)</?[a-z][^>]*>`)

type jiraColumn struct {
	name  string
	label string
	width int
}

func (idx *macroIndex) jiraAssetByKey(objectKey string) (map[string]any, bool) {
	return idx.jiraEntryFor("jiraAssetsByKey", strings.ToUpper(strings.TrimSpace(objectKey)))
}

func jiraAssetAttribute(asset map[string]any, fieldID string) map[string]any {
	fieldID = strings.TrimSpace(fieldID)
	for _, value := range garr(asset, "attributes") {
		attr, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if gnumStr(attr, "id") == fieldID || strings.EqualFold(gstr(attr, "name"), fieldID) {
			return attr
		}
	}
	return nil
}

func jiraRelationReferenceField(fieldID string) string {
	parts := strings.Split(fieldID, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}
	return parts[0]
}

func jiraAssetAttributeValue(attr map[string]any) string {
	for _, value := range garr(attr, "referencedAssets") {
		ref, ok := value.(map[string]any)
		if ok {
			if label := gstr(ref, "label"); label != "" {
				return label
			}
		}
	}
	return strings.Join(stringValues(garr(attr, "values")), ", ")
}

func stringValues(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text = jiraAssetText(text); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func jiraAssetText(value string) string {
	value = html.UnescapeString(value)
	value = jiraAssetHTMLTagRe.ReplaceAllString(value, " ")
	value = layoutLine(terminalText(value))
	return strings.Join(strings.Fields(value), " ")
}

func (idx *macroIndex) jiraAssetInline(a map[string]string) string {
	if !idx.jiraEnabled {
		return "[jira-asset]"
	}
	objectKey := strings.TrimSpace(a["objectKey"])
	if objectKey == "" {
		return "Missing asset key"
	}
	asset, ok := idx.jiraAssetByKey(objectKey)
	if !ok {
		return objectKey
	}

	parts := []string{objectKey}
	if label := gstr(asset, "label"); label != "" {
		parts[0] += " - " + label
	}
	for _, fieldID := range strings.Split(a["attributes"], ",") {
		fieldID = strings.TrimSpace(fieldID)
		if fieldID == "" {
			continue
		}
		lookupID := fieldID
		if refID := jiraRelationReferenceField(fieldID); refID != "" {
			lookupID = refID
		}
		attr := jiraAssetAttribute(asset, lookupID)
		if attr == nil {
			continue
		}
		value := jiraAssetAttributeValue(attr)
		if value == "" {
			continue
		}
		name := gstr(attr, "name")
		if name == "" {
			name = fieldID
		}
		parts = append(parts, name+": "+value)
	}
	return strings.Join(parts, "  ·  ")
}

func parseJiraColumns(raw string) []jiraColumn {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	columns := make([]jiraColumn, 0, len(parts))
	for _, part := range parts {
		namePart := strings.TrimSpace(part)
		width := 20
		if lastColon := strings.LastIndexByte(part, ':'); lastColon >= 0 {
			namePart = strings.TrimSpace(part[:lastColon])
			if parsed, err := strconv.Atoi(strings.TrimSpace(part[lastColon+1:])); err == nil {
				width = parsed
			}
		}
		column := jiraColumn{name: namePart, width: width}
		if pipe := strings.IndexByte(namePart, '|'); pipe >= 0 {
			column.name = namePart[:pipe]
			column.label = namePart[pipe+1:]
		}
		columns = append(columns, column)
	}
	return columns
}

func jiraAssetsKey(a map[string]string) string {
	maxResults := 20
	if parsed, err := strconv.Atoi(a["maxResults"]); err == nil {
		maxResults = parsed
	}
	return "object=" + a["object"] +
		"&aql=" + a["aql"] +
		"&columns=" + a["columns"] +
		"&maxResults=" + strconv.Itoa(maxResults)
}

func renderJiraAssets(n *dnode, idx *macroIndex, width int) string {
	if !idx.jiraEnabled {
		return "[macro: jira-assets]"
	}
	hasObject := strings.TrimSpace(n.attrs["object"]) != ""
	hasAQL := strings.TrimSpace(n.attrs["aql"]) != ""
	if !hasObject && !hasAQL {
		return jiraAssetsState(n.attrs, "Not configured.")
	}

	data, ok := idx.jiraEntryFor("jiraAssets", jiraAssetsKey(n.attrs))
	if !ok {
		return jiraAssetsState(n.attrs, "No data available.")
	}
	if message := gstr(data, "errorMessage"); message != "" {
		return jiraAssetsState(n.attrs, message)
	}
	assets := garr(data, "assets")
	if len(assets) == 0 {
		return jiraAssetsState(n.attrs, "No assets found.")
	}

	columns := parseJiraColumns(n.attrs["columns"])
	if len(columns) == 0 {
		return jiraAssetsState(n.attrs, "No columns configured.")
	}
	columnNames := jiraAssetColumnNames(garr(data, "columnNames"))
	headers := make([]string, len(columns))
	widthParts := make([]string, len(columns))
	for i, column := range columns {
		headers[i] = jiraAssetColumnHeader(column, columnNames, i)
		widthParts[i] = strconv.Itoa(column.width)
	}

	rows := make([][]string, 0, len(assets))
	for _, value := range assets {
		asset, ok := value.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(columns))
		for i, column := range columns {
			row[i] = jiraAssetColumnValue(asset, column.name)
		}
		rows = append(rows, row)
	}
	colW := columnWidths(strings.Join(widthParts, ","), len(columns), width-4*len(columns))
	table := asciiTable(headers, rows, colW)
	// NORM-xpiqijsn: only show truncation indicator when totalCount exceeds rendered rows.
	return appendJiraShowingResults(table, len(rows), int(gnum(data, "totalCount")))
}

// jiraShowingResultsLine returns the non-interactive truncation indicator for
// jira list macros. Empty when the set is complete (shown >= total) or totals
// are unavailable — complete sets must not show a "Showing X of Y" line.
func jiraShowingResultsLine(shown, total int) string {
	if total <= 0 || shown <= 0 || shown >= total {
		return ""
	}
	return fmt.Sprintf("Showing %d of %d results", shown, total)
}

func appendJiraShowingResults(table string, shown, total int) string {
	if line := jiraShowingResultsLine(shown, total); line != "" {
		return table + "\n" + line
	}
	return table
}

func jiraAssetColumnNames(values []any) []string {
	result := make([]string, len(values))
	for i, value := range values {
		if text, ok := value.(string); ok {
			result[i] = layoutLine(terminalText(text))
		}
	}
	return result
}

func jiraAssetsState(a map[string]string, state string) string {
	header := "Jira Assets"
	if object := strings.TrimSpace(a["object"]); object != "" {
		header += " - " + object
	}
	return header + "\n" + state
}

func jiraAssetColumnHeader(column jiraColumn, resolved []string, index int) string {
	if column.label != "" {
		return column.label
	}
	switch strings.ToLower(column.name) {
	case "key":
		return "Key"
	case "label":
		return "Name"
	}
	if index < len(resolved) && resolved[index] != "" {
		return resolved[index]
	}
	return column.name
}

func jiraAssetColumnValue(asset map[string]any, fieldID string) string {
	switch strings.ToLower(fieldID) {
	case "key":
		return gstr(asset, "objectKey")
	case "label":
		return gstr(asset, "label")
	case "name":
		if attr := jiraAssetAttribute(asset, fieldID); attr != nil {
			if value := strings.Join(stringValues(garr(attr, "values")), ", "); value != "" {
				return value
			}
		}
		return gstr(asset, "label")
	}
	if refID := jiraRelationReferenceField(fieldID); refID != "" {
		return jiraReferencedAssetLabels(jiraAssetAttribute(asset, refID))
	}
	attr := jiraAssetAttribute(asset, fieldID)
	if attr == nil {
		return ""
	}
	return strings.Join(stringValues(garr(attr, "values")), ", ")
}

func jiraReferencedAssetLabels(attr map[string]any) string {
	if attr == nil {
		return ""
	}
	labels := make([]string, 0)
	for _, value := range garr(attr, "referencedAssets") {
		ref, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if label := gstr(ref, "label"); label != "" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ", ")
}
