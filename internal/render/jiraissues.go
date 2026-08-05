package render

import (
	"strconv"
	"strings"
)

var jiraIssueStatusColors = map[string]string{
	"new":           "#60A5FA",
	"indeterminate": "#FACC15",
	"done":          "#4ADE80",
}

func (idx *macroIndex) jiraIssueInline(a map[string]string) string {
	if !idx.jiraEnabled {
		return "[jira-issue]"
	}
	issueKey := strings.TrimSpace(a["key"])
	if issueKey == "" {
		return "Missing issue key"
	}
	issue, ok := idx.jiraEntryFor("jiraIssuesByKey", strings.ToUpper(issueKey))
	if !ok {
		return jiraAssetText(issueKey)
	}

	parts := []string{jiraAssetText(issueKey)}
	if summary := jiraAssetText(gtext(issue, "summary")); summary != "" {
		parts[0] += " - " + summary
	}
	if status := jiraAssetText(gtext(gmap(issue, "status"), "name")); status != "" {
		parts = append(parts, "Status: "+status)
	}
	return strings.Join(parts, "  ·  ")
}

func jiraIssuesKey(a map[string]string) string {
	maxResults := 20
	if parsed, err := strconv.Atoi(a["maxResults"]); err == nil {
		maxResults = parsed
	}
	return "jql=" + a["jql"] +
		"&columns=" + a["columns"] +
		"&maxResults=" + strconv.Itoa(maxResults)
}

func renderJiraIssues(n *dnode, idx *macroIndex, width int) string {
	if !idx.jiraEnabled {
		return "[macro: jira-issues]"
	}
	if strings.TrimSpace(n.attrs["jql"]) == "" {
		return jiraIssuesState("Not configured.")
	}
	data, ok := idx.jiraEntryFor("jiraIssues", jiraIssuesKey(n.attrs))
	if !ok {
		return jiraIssuesState("No data available.")
	}
	issues := garr(data, "issues")
	if len(issues) == 0 {
		return jiraIssuesState("No issues found.")
	}
	columns := parseJiraColumns(n.attrs["columns"])
	if len(columns) == 0 {
		return jiraIssuesState("No columns configured.")
	}

	firstIssue, _ := issues[0].(map[string]any)
	fieldNames := gmap(data, "fieldNames")
	headers := make([]string, len(columns))
	widthParts := make([]string, len(columns))
	for i, column := range columns {
		headers[i] = jiraIssueColumnHeader(column, fieldNames, firstIssue)
		widthParts[i] = strconv.Itoa(column.width)
	}
	rows := make([][]string, 0, len(issues))
	for _, value := range issues {
		issue, ok := value.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(columns))
		for i, column := range columns {
			row[i] = jiraIssueColumnValue(issue, column.name)
		}
		rows = append(rows, row)
	}
	colW := columnWidths(strings.Join(widthParts, ","), len(columns), width-4*len(columns))
	table := asciiTable(headers, rows, colW)
	// NORM-xpiqijsn: only show truncation indicator when totalCount exceeds rendered rows.
	return appendJiraShowingResults(table, len(rows), int(gnum(data, "totalCount")))
}

func jiraIssuesState(state string) string {
	return "Jira Issues\n" + state
}

func jiraIssueColumnHeader(column jiraColumn, fieldNames, firstIssue map[string]any) string {
	if resolved := jiraAssetText(gtext(fieldNames, column.name)); resolved != "" {
		return resolved
	}
	if column.label != "" {
		return jiraAssetText(column.label)
	}
	if field := gmap(gmap(firstIssue, "fields"), column.name); field != nil {
		if displayName := jiraAssetText(gtext(field, "displayName")); displayName != "" {
			return displayName
		}
	}
	switch column.name {
	case "key":
		return "Key"
	case "summary":
		return "Summary"
	case "status":
		return "Status"
	case "assignee":
		return "Assignee"
	case "reporter":
		return "Reporter"
	case "priority":
		return "Priority"
	case "issuetype":
		return "Type"
	case "created":
		return "Created"
	case "updated":
		return "Updated"
	}
	return jiraAssetText(column.name)
}

func jiraIssueColumnValue(issue map[string]any, fieldID string) string {
	switch fieldID {
	case "key":
		return jiraAssetText(gtext(issue, "key"))
	case "summary":
		return jiraAssetText(gtext(issue, "summary"))
	case "status":
		return jiraIssueStatusPill(gmap(issue, "status"))
	}
	field := gmap(gmap(issue, "fields"), fieldID)
	return jiraAssetText(gtext(field, "value"))
}

func jiraIssueStatusPill(status map[string]any) string {
	label := jiraAssetText(gtext(status, "name"))
	if label == "" {
		return ""
	}
	color := jiraIssueStatusColors[strings.ToLower(gtext(status, "category"))]
	if color == "" {
		color = "#9CA3AF"
	}
	return pill(label, color)
}
