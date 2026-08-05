package cli

// Property-flag parsing + per-dataType dispatch for `pages create`'s repeatable
// `--property "name=value"` / `--unset-property name` flags. The logic is kept
// here (not inlined in pages.go) so the later `pages update` ticket can reuse the
// exact same grammar, dispatch table and lookups.
//
// The dispatch resolves a property NAME (case-insensitive) to its descriptor in
// the page type's effective metadata, then sets EXACTLY ONE field on the
// generated PropertyValueForm by dataType. Six dataTypes are read-only and are
// rejected client-side. Name->id lookups (enum value, page, page-type, user)
// go through the propertyLookup interface so they can be faked in unit tests.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ---- error type ----------------------------------------------------------

// propFlagError is a client-side validation failure for property flags. It
// carries the errorCode label, the primary detail line, optional indented extra
// lines (valid-values / did-you-mean / candidates) and the process exit code.
type propFlagError struct {
	code   string
	detail string
	extra  []string
	exit   int
}

func (e *propFlagError) Error() string { return e.code + ": " + e.detail }

func usageError(detail string) *propFlagError {
	return &propFlagError{code: "USAGE", detail: detail, exit: 2}
}

// invalidRequest mirrors the server's INVALID_REQUEST class (exit 3).
func invalidRequest(detail string, extra ...string) *propFlagError {
	return &propFlagError{code: "INVALID_REQUEST", detail: detail, extra: extra, exit: 3}
}

// ---- parsing -------------------------------------------------------------

// propertyAssignment is one parsed `--property name=value` pair. value is the
// verbatim right-hand side (an empty string is a legitimate "set empty value").
type propertyAssignment struct {
	name  string
	value string
}

// parseAssignments splits each `--property` string on the FIRST `=` so that `=`
// is allowed inside the value. A string without any `=` is a usage error.
func parseAssignments(props []string) ([]propertyAssignment, *propFlagError) {
	out := make([]propertyAssignment, 0, len(props))
	for _, raw := range props {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, usageError(fmt.Sprintf("--property %q must be in name=value form", raw))
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			return nil, usageError(fmt.Sprintf("--property %q has an empty name", raw))
		}
		out = append(out, propertyAssignment{name: name, value: parts[1]})
	}
	return out, nil
}

// knownPrefixes are the disambiguation prefixes; an explicit prefix overrides the
// default (name-lookup) heuristic.
var knownPrefixes = []string{"id:", "name:", "email:", "ext:"}

// splitPrefix returns the recognised prefix (without the colon) and the rest of
// the token. ("", token) when no known prefix is present.
func splitPrefix(token string) (prefix, rest string) {
	for _, p := range knownPrefixes {
		if strings.HasPrefix(token, p) {
			return strings.TrimSuffix(p, ":"), token[len(p):]
		}
	}
	return "", token
}

// ---- lookups (faked in tests) -------------------------------------------

type enumValue struct {
	id    int64
	value string
}

// namedRef is an id+name candidate; note carries a secondary label (e.g. a
// page's pageTypeName) used only in candidate/ambiguity messages.
type namedRef struct {
	id   int64
	name string
	note string
}

// propertyLookup abstracts the network name->id resolutions so the dispatch is
// unit-testable with a stub.
type propertyLookup interface {
	EnumValues(ctx context.Context, domainEnumID int64) ([]enumValue, error)
	PagesByName(ctx context.Context, name string, allowedPageTypeID *int64) ([]namedRef, error)
	PageTypesByName(ctx context.Context, name string) ([]namedRef, error)
	UsersByEmail(ctx context.Context, email string) ([]namedRef, error)
}

// clientLookup is the production propertyLookup backed by the API client.
type clientLookup struct {
	c        *client.Client
	ptCache  []namedRef
	ptLoaded bool
}

func newClientLookup(c *client.Client) *clientLookup { return &clientLookup{c: c} }

// cachingLookup memoizes name->id resolutions for the lifetime of a single
// command so that two properties needing the same enum / page-search / user-search
// trigger at most one client call per key (enum by domainEnumID, page by
// name+allowedPageTypeID, user by email). PageTypesByName is already cached by the
// underlying clientLookup, so it is delegated unchanged.
type cachingLookup struct {
	inner propertyLookup
	enums map[int64][]enumValue
	pages map[string][]namedRef
	users map[string][]namedRef
}

func newCachingLookup(inner propertyLookup) *cachingLookup {
	return &cachingLookup{
		inner: inner,
		enums: map[int64][]enumValue{},
		pages: map[string][]namedRef{},
		users: map[string][]namedRef{},
	}
}

func (l *cachingLookup) EnumValues(ctx context.Context, domainEnumID int64) ([]enumValue, error) {
	if v, ok := l.enums[domainEnumID]; ok {
		return v, nil
	}
	v, err := l.inner.EnumValues(ctx, domainEnumID)
	if err != nil {
		return nil, err
	}
	l.enums[domainEnumID] = v
	return v, nil
}

func (l *cachingLookup) PagesByName(ctx context.Context, name string, allowedPageTypeID *int64) ([]namedRef, error) {
	key := name + "|"
	if allowedPageTypeID != nil {
		key += strconv.FormatInt(*allowedPageTypeID, 10)
	}
	if v, ok := l.pages[key]; ok {
		return v, nil
	}
	v, err := l.inner.PagesByName(ctx, name, allowedPageTypeID)
	if err != nil {
		return nil, err
	}
	l.pages[key] = v
	return v, nil
}

func (l *cachingLookup) PageTypesByName(ctx context.Context, name string) ([]namedRef, error) {
	return l.inner.PageTypesByName(ctx, name)
}

func (l *cachingLookup) UsersByEmail(ctx context.Context, email string) ([]namedRef, error) {
	if v, ok := l.users[email]; ok {
		return v, nil
	}
	v, err := l.inner.UsersByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	l.users[email] = v
	return v, nil
}

func (l *clientLookup) EnumValues(ctx context.Context, domainEnumID int64) ([]enumValue, error) {
	body, apiErr := l.c.GetDomainEnum(ctx, domainEnumID)
	if apiErr != nil {
		return nil, errors.New(apiErr.Error())
	}
	var de api.DomainEnumResult
	if err := json.Unmarshal(body, &de); err != nil {
		return nil, err
	}
	var out []enumValue
	if de.Values != nil {
		for _, v := range *de.Values {
			out = append(out, enumValue{id: deref64(v.Id), value: derefStr(v.Value)})
		}
	}
	return out, nil
}

func (l *clientLookup) PagesByName(ctx context.Context, name string, allowedPageTypeID *int64) ([]namedRef, error) {
	// size 200 == backend maxPageSize: fetch the candidate set in one call so an exact
	// name match is never silently dropped beyond a small first page.
	body, apiErr := l.c.SearchPages(ctx, name, 1, 200, allowedPageTypeID)
	if apiErr != nil {
		return nil, errors.New(apiErr.Error())
	}
	var res api.PagePageSearchResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	var out []namedRef
	if res.Content != nil {
		for _, p := range *res.Content {
			out = append(out, namedRef{id: deref64(p.Id), name: derefStr(p.Name), note: derefStr(p.PageTypeName)})
		}
	}
	return out, nil
}

func (l *clientLookup) PageTypesByName(ctx context.Context, _ string) ([]namedRef, error) {
	if l.ptLoaded {
		return l.ptCache, nil
	}
	body, apiErr := l.c.ListPageTypes(ctx)
	if apiErr != nil {
		return nil, errors.New(apiErr.Error())
	}
	var pts []api.PageTypeResult
	if err := unmarshalList(body, &pts); err != nil {
		return nil, err
	}
	out := make([]namedRef, 0, len(pts))
	for _, p := range pts {
		out = append(out, namedRef{id: deref64(p.Id), name: derefStr(p.Name)})
	}
	l.ptCache, l.ptLoaded = out, true
	return out, nil
}

func (l *clientLookup) UsersByEmail(ctx context.Context, email string) ([]namedRef, error) {
	body, apiErr := l.c.SearchUsers(ctx, email, 1, 200, nil)
	if apiErr != nil {
		return nil, errors.New(apiErr.Error())
	}
	var users []api.UserResult
	if err := unmarshalList(body, &users); err != nil {
		return nil, err
	}
	var out []namedRef
	for _, u := range users {
		out = append(out, namedRef{id: deref64(u.Id), name: derefStr(u.Email)})
	}
	return out, nil
}

// unmarshalList tolerates either a bare JSON array or a Page<T> envelope
// ({"content":[...]}) so list endpoints with differing shapes both decode.
func unmarshalList[T any](body []byte, dst *[]T) error {
	if json.Unmarshal(body, dst) == nil && *dst != nil {
		return nil
	}
	var env struct {
		Content []T `json:"content"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return err
	}
	*dst = env.Content
	return nil
}

// ---- dataType classification --------------------------------------------

// writableDataType reports whether a dataType can be set via --property. The six
// read-only types are derived/computed server-side.
func writableDataType(dt api.PropertyDescriptorResultDataType) bool {
	switch dt {
	case api.PropertyDescriptorResultDataTypeTEXT,
		api.PropertyDescriptorResultDataTypeMARKDOWN,
		api.PropertyDescriptorResultDataTypeENUM,
		api.PropertyDescriptorResultDataTypeDATE,
		api.PropertyDescriptorResultDataTypeDATETIME,
		api.PropertyDescriptorResultDataTypeNUMBER,
		api.PropertyDescriptorResultDataTypePAGEOUTGOING,
		api.PropertyDescriptorResultDataTypePAGETYPE,
		api.PropertyDescriptorResultDataTypeUSERLIST:
		return true
	case api.PropertyDescriptorResultDataTypeCALCULATED,
		api.PropertyDescriptorResultDataTypeCONDITIONALENUM,
		api.PropertyDescriptorResultDataTypePAGEINCOMING,
		api.PropertyDescriptorResultDataTypePAGESELECTOR,
		api.PropertyDescriptorResultDataTypePROPERTYCHAIN,
		api.PropertyDescriptorResultDataTypePARENTPAGE:
		return false
	}
	return false
}

// lookupHint describes how a writable property's value is interpreted, for the
// `describe-properties` discovery output.
func lookupHint(desc api.PropertyDescriptorResult) string {
	dt := derefDataType(desc.DataType)
	switch dt {
	case api.PropertyDescriptorResultDataTypeTEXT:
		return "free text"
	case api.PropertyDescriptorResultDataTypeMARKDOWN:
		return "markdown text"
	case api.PropertyDescriptorResultDataTypeENUM:
		if desc.DomainEnumName != nil {
			return "enum display value (enum: " + *desc.DomainEnumName + ")"
		}
		return "enum display value"
	case api.PropertyDescriptorResultDataTypeDATE:
		return "YYYY-MM-DD"
	case api.PropertyDescriptorResultDataTypeDATETIME:
		return "YYYY-MM-DDTHH:MM:SS (zone via --timezone)"
	case api.PropertyDescriptorResultDataTypeNUMBER:
		if desc.NumberFormat != nil && *desc.NumberFormat == api.PropertyDescriptorResultNumberFormatPERCENTAGE {
			return "number as percent, e.g. 12.5" + rangeSuffix(desc) + decimalsSuffix(desc)
		}
		return "number" + rangeSuffix(desc) + decimalsSuffix(desc)
	case api.PropertyDescriptorResultDataTypePAGEOUTGOING:
		if desc.AllowedPageTypeName != nil {
			return "page name or id:<n>, comma-separated (replaces the whole list; allowed type: " + *desc.AllowedPageTypeName + ")"
		}
		return "page name or id:<n>, comma-separated (replaces the whole list)"
	case api.PropertyDescriptorResultDataTypePAGETYPE:
		return "page-type name or id:<n>"
	case api.PropertyDescriptorResultDataTypeUSERLIST:
		return "user email or ext:<name>, comma-separated (replaces the whole list)"
	case api.PropertyDescriptorResultDataTypeCALCULATED,
		api.PropertyDescriptorResultDataTypeCONDITIONALENUM,
		api.PropertyDescriptorResultDataTypePAGEINCOMING,
		api.PropertyDescriptorResultDataTypePAGESELECTOR,
		api.PropertyDescriptorResultDataTypePROPERTYCHAIN,
		api.PropertyDescriptorResultDataTypePARENTPAGE:
		return "read-only (computed/derived)"
	}
	return ""
}

func rangeSuffix(desc api.PropertyDescriptorResult) string {
	if desc.MinValue == nil && desc.MaxValue == nil {
		return ""
	}
	lo, hi := "", ""
	if desc.MinValue != nil {
		lo = trimFloat(float64(*desc.MinValue))
	}
	if desc.MaxValue != nil {
		hi = trimFloat(float64(*desc.MaxValue))
	}
	return fmt.Sprintf(" [%s, %s]", dashEmpty(lo), dashEmpty(hi))
}

// decimalsSuffix appends the descriptor's display precision to a NUMBER hint. This
// is a presentation hint only: it says how many fractional digits the value is
// SHOWN with, not how many the caller may type. The backend does not enforce it,
// so a value like 12.5 is accepted even when the display precision is 0.
func decimalsSuffix(desc api.PropertyDescriptorResult) string {
	if desc.Decimals == nil {
		return ""
	}
	return fmt.Sprintf(" (display: %d decimals)", *desc.Decimals)
}

// describeEnumValues resolves an ENUM descriptor's allowed display values for the
// self-contained describe-properties output. It degrades to nil (no values) when
// the descriptor is not an enum, carries no domain enum id, or the lookup fails
// (e.g. a permission error) — the caller still shows the enum name from the hint
// and never crashes on a failed lookup.
func describeEnumValues(ctx context.Context, lookup propertyLookup, desc api.PropertyDescriptorResult) []string {
	if derefDataType(desc.DataType) != api.PropertyDescriptorResultDataTypeENUM || desc.DomainEnumId == nil {
		return nil
	}
	values, err := lookup.EnumValues(ctx, *desc.DomainEnumId)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.value)
	}
	return out
}

// describeHint builds the human-readable lookup hint, folding the resolved enum
// values inline so the table output stays self-contained (arrays render as "…" in
// table cells, so the values must live in the hint string).
func describeHint(desc api.PropertyDescriptorResult, enumValues []string) string {
	hint := lookupHint(desc)
	if len(enumValues) > 0 && derefDataType(desc.DataType) == api.PropertyDescriptorResultDataTypeENUM {
		hint += "; values: " + strings.Join(enumValues, ", ")
	}
	return hint
}

// describeExample returns a ready-to-paste `--property "name=value"` string for a
// writable descriptor so an agent can construct a call without a second command.
// Read-only descriptors return "" (no example).
func describeExample(desc api.PropertyDescriptorResult, enumValues []string) string {
	name := derefStr(desc.Name)
	switch derefDataType(desc.DataType) {
	case api.PropertyDescriptorResultDataTypeTEXT:
		return name + "=some text"
	case api.PropertyDescriptorResultDataTypeMARKDOWN:
		return name + "=**markdown** text"
	case api.PropertyDescriptorResultDataTypeENUM:
		if len(enumValues) > 0 {
			return name + "=" + enumValues[0]
		}
		return name + "=<enum value>"
	case api.PropertyDescriptorResultDataTypeDATE:
		return name + "=2026-07-01"
	case api.PropertyDescriptorResultDataTypeDATETIME:
		return name + "=2026-07-01T09:00:00"
	case api.PropertyDescriptorResultDataTypeNUMBER:
		return name + "=" + numberExample(desc)
	case api.PropertyDescriptorResultDataTypePAGEOUTGOING:
		return name + "=id:123"
	case api.PropertyDescriptorResultDataTypePAGETYPE:
		return name + "=id:3"
	case api.PropertyDescriptorResultDataTypeUSERLIST:
		return name + "=email:user@example.com"
	case api.PropertyDescriptorResultDataTypeCALCULATED,
		api.PropertyDescriptorResultDataTypeCONDITIONALENUM,
		api.PropertyDescriptorResultDataTypePAGEINCOMING,
		api.PropertyDescriptorResultDataTypePAGESELECTOR,
		api.PropertyDescriptorResultDataTypePROPERTYCHAIN,
		api.PropertyDescriptorResultDataTypePARENTPAGE:
		return ""
	}
	return ""
}

// numberExample picks a concrete value that satisfies the descriptor's min/max
// bounds (both expressed in display units, percent for a PERCENTAGE descriptor).
func numberExample(desc api.PropertyDescriptorResult) string {
	switch {
	case desc.MinValue != nil:
		return trimFloat(float64(*desc.MinValue))
	case desc.MaxValue != nil:
		return trimFloat(float64(*desc.MaxValue))
	default:
		return "1"
	}
}

// ---- dispatch ------------------------------------------------------------

// propertyDispatcher turns a (descriptor, rawValue) pair into a PropertyValueForm
// by setting exactly one field according to the dataType.
type propertyDispatcher struct {
	lookup   propertyLookup
	timezone string // IANA zone for DATE_TIME; resolved by the caller
}

func (pd propertyDispatcher) build(ctx context.Context, desc api.PropertyDescriptorResult, name, rawValue string) (api.PropertyValueForm, *propFlagError) {
	form := api.PropertyValueForm{PropertyDescriptorId: deref64(desc.Id)}
	if desc.DataType == nil {
		return form, invalidRequest(fmt.Sprintf("property '%s' has no dataType in the metadata", name))
	}
	dt := *desc.DataType
	switch dt {
	case api.PropertyDescriptorResultDataTypeTEXT, api.PropertyDescriptorResultDataTypeMARKDOWN:
		v := rawValue
		form.TextValue = &v
	case api.PropertyDescriptorResultDataTypeENUM:
		return pd.buildEnum(ctx, desc, name, rawValue, form)
	case api.PropertyDescriptorResultDataTypeDATE:
		return pd.buildDate(name, rawValue, form)
	case api.PropertyDescriptorResultDataTypeDATETIME:
		return pd.buildDateTime(name, rawValue, form)
	case api.PropertyDescriptorResultDataTypeNUMBER:
		return pd.buildNumber(desc, name, rawValue, form)
	case api.PropertyDescriptorResultDataTypePAGEOUTGOING:
		return pd.buildPageOutgoing(ctx, desc, name, rawValue, form)
	case api.PropertyDescriptorResultDataTypePAGETYPE:
		return pd.buildPageType(ctx, name, rawValue, form)
	case api.PropertyDescriptorResultDataTypeUSERLIST:
		return pd.buildUserList(ctx, name, rawValue, form)
	case api.PropertyDescriptorResultDataTypeCALCULATED,
		api.PropertyDescriptorResultDataTypeCONDITIONALENUM,
		api.PropertyDescriptorResultDataTypePAGEINCOMING,
		api.PropertyDescriptorResultDataTypePAGESELECTOR,
		api.PropertyDescriptorResultDataTypePROPERTYCHAIN,
		api.PropertyDescriptorResultDataTypePARENTPAGE:
		return form, readOnlyError(name, dt)
	}
	return form, nil
}

func readOnlyError(name string, dt api.PropertyDescriptorResultDataType) *propFlagError {
	return invalidRequest(fmt.Sprintf("property '%s' is read-only (%s) and cannot be set via --property", name, dt))
}

func (pd propertyDispatcher) buildEnum(ctx context.Context, desc api.PropertyDescriptorResult, name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	prefix, rest := splitPrefix(strings.TrimSpace(rawValue))
	if prefix == "id" {
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return form, invalidRequest(fmt.Sprintf("property '%s' id:%s is not a numeric enum value id", name, rest))
		}
		form.EnumValueId = &id
		return form, nil
	}
	if desc.DomainEnumId == nil {
		return form, invalidRequest(fmt.Sprintf("property '%s' has no domain enum in the metadata", name))
	}
	values, err := pd.lookup.EnumValues(ctx, *desc.DomainEnumId)
	if err != nil {
		return form, lookupFailed(name, err)
	}
	for _, v := range values {
		if strings.EqualFold(v.value, rest) {
			id := v.id
			form.EnumValueId = &id
			return form, nil
		}
	}
	allowed := make([]string, 0, len(values))
	for _, v := range values {
		allowed = append(allowed, v.value)
	}
	extra := []string{fmt.Sprintf("valid values for '%s': %s", name, strings.Join(allowed, ", "))}
	if dym := closestName(rest, allowed); dym != "" {
		extra = append([]string{fmt.Sprintf("did you mean '%s'?", dym)}, extra...)
	}
	return form, invalidRequest(fmt.Sprintf("property '%s' has no enum value %q", name, rest), extra...)
}

func (pd propertyDispatcher) buildDate(name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	v := strings.TrimSpace(rawValue)
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return form, &propFlagError{code: "DATE_TIME_FORMAT_INVALID", exit: 3,
			detail: fmt.Sprintf("property '%s' expects a date YYYY-MM-DD, got %q", name, v)}
	}
	form.DateValue = &openapi_types.Date{Time: t}
	return form, nil
}

func (pd propertyDispatcher) buildDateTime(name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	v := strings.TrimSpace(rawValue)
	if _, err := time.Parse("2006-01-02T15:04:05", v); err != nil {
		return form, &propFlagError{code: "DATE_TIME_FORMAT_INVALID", exit: 3,
			detail: fmt.Sprintf("property '%s' expects YYYY-MM-DDTHH:MM:SS, got %q", name, v)}
	}
	zone := pd.timezone
	if zone == "" {
		zone = "UTC"
	}
	out := v + "[" + zone + "]"
	form.DateTimeValue = &out
	return form, nil
}

func (pd propertyDispatcher) buildNumber(desc api.PropertyDescriptorResult, name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	v := strings.TrimSpace(rawValue)
	display, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return form, invalidRequest(fmt.Sprintf("property '%s' expects a number, got %q", name, v))
	}
	// Validate against the descriptor min/max in DISPLAY units (percent for a
	// PERCENTAGE descriptor — the same units the user types and the metadata holds).
	if (desc.MinValue != nil && display < float64(*desc.MinValue)) ||
		(desc.MaxValue != nil && display > float64(*desc.MaxValue)) {
		return form, &propFlagError{code: "NUMBER_OUT_OF_RANGE", exit: 3,
			detail: fmt.Sprintf("property '%s' value %s out of range %s", name, trimFloat(display), rangeBracket(desc))}
	}
	send := display
	if desc.NumberFormat != nil && *desc.NumberFormat == api.PropertyDescriptorResultNumberFormatPERCENTAGE {
		send = display / 100
	}
	f := float32(send)
	form.NumericValue = &f
	return form, nil
}

func (pd propertyDispatcher) buildPageOutgoing(ctx context.Context, desc api.PropertyDescriptorResult, name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	tokens := splitMulti(rawValue)
	ids := make([]int64, 0, len(tokens))
	for _, tok := range tokens {
		prefix, rest := splitPrefix(tok)
		if prefix == "id" {
			id, err := strconv.ParseInt(rest, 10, 64)
			if err != nil {
				return form, invalidRequest(fmt.Sprintf("property '%s' id:%s is not a numeric page id", name, rest))
			}
			ids = append(ids, id)
			continue
		}
		candidates, err := pd.lookup.PagesByName(ctx, rest, desc.AllowedPageTypeId)
		if err != nil {
			return form, lookupFailed(name, err)
		}
		exact := filterExact(candidates, rest)
		switch len(exact) {
		case 1:
			ids = append(ids, exact[0].id)
		case 0:
			return form, pageNotFound(name, rest, desc, candidates)
		default:
			return form, ambiguous(name, rest, exact)
		}
	}
	form.PageReferenceIds = &ids
	return form, nil
}

func (pd propertyDispatcher) buildPageType(ctx context.Context, name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	prefix, rest := splitPrefix(strings.TrimSpace(rawValue))
	if prefix == "id" {
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return form, invalidRequest(fmt.Sprintf("property '%s' id:%s is not a numeric page-type id", name, rest))
		}
		form.SelectedPageTypeId = &id
		return form, nil
	}
	candidates, err := pd.lookup.PageTypesByName(ctx, rest)
	if err != nil {
		return form, lookupFailed(name, err)
	}
	exact := filterExact(candidates, rest)
	switch len(exact) {
	case 1:
		id := exact[0].id
		form.SelectedPageTypeId = &id
		return form, nil
	case 0:
		extra := []string{"use id:<n> to reference a page type by id"}
		if dym := closestName(rest, refNames(candidates)); dym != "" {
			extra = append([]string{fmt.Sprintf("did you mean '%s'?", dym)}, extra...)
		}
		return form, invalidRequest(fmt.Sprintf("property '%s': no page type named %q", name, rest), extra...)
	default:
		return form, ambiguous(name, rest, exact)
	}
}

func (pd propertyDispatcher) buildUserList(ctx context.Context, name, rawValue string, form api.PropertyValueForm) (api.PropertyValueForm, *propFlagError) {
	tokens := splitMulti(rawValue)
	links := make([]api.UserLinkForm, 0, len(tokens))
	for _, tok := range tokens {
		prefix, rest := splitPrefix(tok)
		switch prefix {
		case "ext":
			ext := rest
			links = append(links, api.UserLinkForm{ExternalDisplayName: &ext})
		case "id":
			id, err := strconv.ParseInt(rest, 10, 64)
			if err != nil {
				return form, invalidRequest(fmt.Sprintf("property '%s' id:%s is not a numeric user id", name, rest))
			}
			links = append(links, api.UserLinkForm{UserId: &id})
		default: // "" or "email"
			candidates, err := pd.lookup.UsersByEmail(ctx, rest)
			if err != nil {
				return form, lookupFailed(name, err)
			}
			exact := filterExact(candidates, rest)
			switch len(exact) {
			case 1:
				id := exact[0].id
				links = append(links, api.UserLinkForm{UserId: &id})
			case 0:
				return form, invalidRequest(
					fmt.Sprintf("property '%s': no user with email %q", name, rest),
					"use ext:<name> for an external (non-account) user")
			default:
				return form, ambiguous(name, rest, exact)
			}
		}
	}
	form.UserLinks = &links
	return form, nil
}

// ---- merge orchestrator --------------------------------------------------

// dispatchAssignments parses --property and dispatches each assignment against the
// descriptor metadata, returning the resulting values in input order. It is the
// shared core for both the create and update property-flag paths; the dispatcher
// (and its lookups) live here and are never duplicated per command.
func dispatchAssignments(ctx context.Context, props []string, descriptors []api.PropertyDescriptorResult, pd propertyDispatcher) ([]api.PropertyValueForm, *propFlagError) {
	assignments, perr := parseAssignments(props)
	if perr != nil {
		return nil, perr
	}
	out := make([]api.PropertyValueForm, 0, len(assignments))
	for _, a := range assignments {
		desc, ok := findDescriptor(descriptors, a.name)
		if !ok {
			return nil, unknownProperty(a.name, descriptors)
		}
		pv, derr := pd.build(ctx, *desc, a.name, a.value)
		if derr != nil {
			return nil, derr
		}
		out = append(out, pv)
	}
	return out, nil
}

// resolveUnsetIDs maps each --unset-property name to its descriptor id (an unknown
// name yields the same did-you-mean error as --property). Duplicates are dropped.
func resolveUnsetIDs(unsets []string, descriptors []api.PropertyDescriptorResult) ([]int64, *propFlagError) {
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(unsets))
	for _, raw := range unsets {
		nm := strings.TrimSpace(raw)
		if nm == "" {
			return nil, usageError("--unset-property requires a property name")
		}
		desc, ok := findDescriptor(descriptors, nm)
		if !ok {
			return nil, unknownProperty(nm, descriptors)
		}
		id := deref64(desc.Id)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// applyPropertyFlags merges --property / --unset-property onto a base
// propertyValues slice (from -f form.json) for the CREATE path. It returns the
// merged slice sorted by descriptorId for deterministic output. base values are
// kept unless overridden or unset; unknown names and read-only types are rejected
// client-side. On create there is no stored value, so an unset simply drops the
// descriptor from the outgoing set.
func applyPropertyFlags(ctx context.Context, base []api.PropertyValueForm, props, unsets []string, descriptors []api.PropertyDescriptorResult, pd propertyDispatcher) ([]api.PropertyValueForm, *propFlagError) {
	assigned, perr := dispatchAssignments(ctx, props, descriptors, pd)
	if perr != nil {
		return nil, perr
	}
	unsetIDs, perr := resolveUnsetIDs(unsets, descriptors)
	if perr != nil {
		return nil, perr
	}

	byID := make(map[int64]api.PropertyValueForm, len(base))
	order := make([]int64, 0, len(base))
	put := func(pv api.PropertyValueForm) {
		if _, seen := byID[pv.PropertyDescriptorId]; !seen {
			order = append(order, pv.PropertyDescriptorId)
		}
		byID[pv.PropertyDescriptorId] = pv
	}
	for _, pv := range base {
		put(pv)
	}
	for _, pv := range assigned {
		put(pv)
	}
	for _, id := range unsetIDs {
		if _, seen := byID[id]; seen {
			delete(byID, id)
			order = removeID(order, id)
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	out := make([]api.PropertyValueForm, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// applyPropertyFlagsPatch merges --property / --unset-property onto a base
// propertyValues slice (from -f PageEditForm) for the UPDATE/PATCH path. Unlike
// create, removal is expressed through the returned unsetPropertyDescriptorIds
// (PATCH semantics) rather than by dropping the value; a descriptor that is unset
// is also stripped from the value delta so the same PATCH never both sets and
// unsets it.
func applyPropertyFlagsPatch(ctx context.Context, base []api.PropertyValueEditForm, props, unsets []string, descriptors []api.PropertyDescriptorResult, pd propertyDispatcher) (values []api.PropertyValueEditForm, unsetIDs []int64, err *propFlagError) {
	assigned, perr := dispatchAssignments(ctx, props, descriptors, pd)
	if perr != nil {
		return nil, nil, perr
	}
	unsetIDs, perr = resolveUnsetIDs(unsets, descriptors)
	if perr != nil {
		return nil, nil, perr
	}

	byID := make(map[int64]api.PropertyValueEditForm, len(base))
	order := make([]int64, 0, len(base))
	put := func(pv api.PropertyValueEditForm) {
		if _, seen := byID[pv.PropertyDescriptorId]; !seen {
			order = append(order, pv.PropertyDescriptorId)
		}
		byID[pv.PropertyDescriptorId] = pv
	}
	for _, pv := range base {
		put(pv)
	}
	for _, pv := range assigned {
		put(toEditForm(pv))
	}
	for _, id := range unsetIDs {
		if _, seen := byID[id]; seen {
			delete(byID, id)
			order = removeID(order, id)
		}
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	values = make([]api.PropertyValueEditForm, 0, len(order))
	for _, id := range order {
		values = append(values, byID[id])
	}
	return values, unsetIDs, nil
}

// toEditForm maps a dispatched create-style PropertyValueForm to the PATCH edit
// form. The two shapes are identical except for EditForm's optional Id (which the
// dispatcher never sets), so every value field is carried over 1:1.
func toEditForm(pv api.PropertyValueForm) api.PropertyValueEditForm {
	return api.PropertyValueEditForm{
		PropertyDescriptorId:      pv.PropertyDescriptorId,
		TextValue:                 pv.TextValue,
		EnumValueId:               pv.EnumValueId,
		DateValue:                 pv.DateValue,
		DateTimeValue:             pv.DateTimeValue,
		NumericValue:              pv.NumericValue,
		PageReferenceIds:          pv.PageReferenceIds,
		SelectedPageTypeId:        pv.SelectedPageTypeId,
		UserLinks:                 pv.UserLinks,
		DisplayColumns:            pv.DisplayColumns,
		NameColumnWidthPercentage: pv.NameColumnWidthPercentage,
		Version:                   pv.Version,
	}
}

// findDescriptor matches a descriptor by name, case-insensitively.
func findDescriptor(descriptors []api.PropertyDescriptorResult, name string) (*api.PropertyDescriptorResult, bool) {
	for i := range descriptors {
		if descriptors[i].Name != nil && strings.EqualFold(*descriptors[i].Name, name) {
			return &descriptors[i], true
		}
	}
	return nil, false
}

// unknownProperty reports an unknown property name with the writable candidates
// and a did-you-mean suggestion.
func unknownProperty(name string, descriptors []api.PropertyDescriptorResult) *propFlagError {
	var writable []string
	for _, d := range descriptors {
		if d.Name != nil && writableDataType(derefDataType(d.DataType)) {
			writable = append(writable, *d.Name)
		}
	}
	extra := []string{fmt.Sprintf("valid values for 'property': %s", strings.Join(writable, ", "))}
	if dym := closestName(name, writable); dym != "" {
		extra = append([]string{fmt.Sprintf("did you mean '%s'?", dym)}, extra...)
	}
	return invalidRequest(fmt.Sprintf("unknown property %q", name), extra...)
}

// ---- shared formatting / utility helpers --------------------------------

func lookupFailed(name string, err error) *propFlagError {
	return &propFlagError{code: "LOOKUP_FAILED", exit: 1,
		detail: fmt.Sprintf("could not resolve property '%s': %v", name, err)}
}

func pageNotFound(name, value string, desc api.PropertyDescriptorResult, candidates []namedRef) *propFlagError {
	var extra []string
	if desc.AllowedPageTypeName != nil {
		extra = append(extra, "allowed page type: "+*desc.AllowedPageTypeName)
	}
	if len(candidates) > 0 {
		extra = append(extra, "near matches: "+joinCandidates(candidates))
	}
	extra = append(extra, "use id:<n> to reference a page by id")
	return invalidRequest(fmt.Sprintf("property '%s': no page named %q", name, value), extra...)
}

func ambiguous(name, value string, matches []namedRef) *propFlagError {
	return invalidRequest(
		fmt.Sprintf("property '%s': %q is ambiguous (%d matches)", name, value, len(matches)),
		"candidates: "+joinCandidates(matches),
		fmt.Sprintf("disambiguate with id:<n>, e.g. id:%d", matches[0].id))
}

func joinCandidates(refs []namedRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		s := fmt.Sprintf("%d %s", r.id, r.name)
		if r.note != "" {
			s += " (" + r.note + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// filterExact keeps refs whose name matches the target case-insensitively.
func filterExact(refs []namedRef, target string) []namedRef {
	var out []namedRef
	for _, r := range refs {
		if strings.EqualFold(r.name, target) {
			out = append(out, r)
		}
	}
	return out
}

func refNames(refs []namedRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.name)
	}
	return out
}

// splitMulti splits a multi-value (array dataType) value on commas, trimming and
// dropping empties.
func splitMulti(value string) []string {
	var out []string
	for _, p := range strings.Split(value, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func removeID(ids []int64, id int64) []int64 {
	out := ids[:0]
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

// closestName returns the candidate with the smallest edit distance (<=3) to
// target, case-insensitively; "" when nothing is close.
func closestName(target string, candidates []string) string {
	target = strings.ToLower(strings.TrimSpace(target))
	best, bestD := "", 4
	for _, c := range candidates {
		if d := levenshtein(target, strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = minInt(cur[j-1]+1, minInt(prev[j]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[lb]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rangeBracket(desc api.PropertyDescriptorResult) string {
	lo, hi := "", ""
	if desc.MinValue != nil {
		lo = trimFloat(float64(*desc.MinValue))
	}
	if desc.MaxValue != nil {
		hi = trimFloat(float64(*desc.MaxValue))
	}
	return fmt.Sprintf("[%s, %s]", dashEmpty(lo), dashEmpty(hi))
}

func trimFloat(f float64) string { return strconv.FormatFloat(f, 'g', -1, 64) }

func dashEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// f32ToF64Ptr widens an optional float32 (the descriptor's numeric bounds) to a
// float64 pointer for stable JSON output, preserving nil.
func f32ToF64Ptr(p *float32) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

func derefDataType(p *api.PropertyDescriptorResultDataType) api.PropertyDescriptorResultDataType {
	if p == nil {
		return ""
	}
	return *p
}

func derefDescriptors(p *[]api.PropertyDescriptorResult) []api.PropertyDescriptorResult {
	if p == nil {
		return nil
	}
	return *p
}

func derefEditValues(p *[]api.PropertyValueEditForm) []api.PropertyValueEditForm {
	if p == nil {
		return nil
	}
	return *p
}

// fetchPage GETs a page and unmarshals the composite result. It is the update
// path's metadata source: the response carries availablePropertyDescriptors,
// workflowEnabled and version (no separate GetPageType round-trip needed).
func fetchPage(ctx context.Context, c *client.Client, id int64) (*api.PublicPageCompositeResult, *client.APIError) {
	body, apiErr := c.GetPage(ctx, id, nil)
	if apiErr != nil {
		return nil, apiErr
	}
	var page api.PublicPageCompositeResult
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, &client.APIError{Status: 0, Body: body, Malformed: true}
	}
	return &page, nil
}

// localZoneName resolves the host IANA zone for DATE_TIME values when the user
// did not pass --timezone. Falls back through TZ, the /etc/localtime symlink and
// finally UTC.
func localZoneName() string {
	if tz := os.Getenv("TZ"); tz != "" && tz != "Local" {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):]
		}
	}
	if n := time.Local.String(); n != "" && n != "Local" {
		return n
	}
	return "UTC"
}
