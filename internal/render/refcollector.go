package render

import "strings"

// CollectReferencePageIDs returns unique page ids from reference tables that
// the rich renderer actually paints: top-level childrenInstances,
// propertyChains and pageSelector, plus the same three maps one include-page
// or excerpt-include level down. Ids are first-seen in a canonical walk
// (family order, then sorted map keys, then pageReferences array order).
//
// An entry counts only when both pageReferences and displayColumns are
// non-empty. Children CONTENT tables, Details-section PAGE properties, content
// parsing and depth-2 nested includes are out of scope.
func CollectReferencePageIDs(composite map[string]any) []int64 {
	seen := make(map[int64]struct{})
	var out []int64
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	collectFromMacroData(gmap(composite, "macroData"), add, true)
	return out
}

func collectFromMacroData(md map[string]any, add func(int64), includeNested bool) {
	if md == nil {
		return
	}
	collectTableMap(md, "childrenInstances", add, true)
	collectTableMap(md, "propertyChains", add, false)
	collectTableMap(md, "pageSelector", add, false)
	if !includeNested {
		return
	}
	for _, nestKey := range []string{"includePages", "excerptIncludes"} {
		nested := gmap(md, nestKey)
		for _, k := range sortedKeys(nested) {
			entry, _ := nested[k].(map[string]any)
			collectFromMacroData(gmap(entry, "macroData"), add, false)
		}
	}
}

func collectTableMap(md map[string]any, key string, add func(int64), skipContent bool) {
	mm := gmap(md, key)
	for _, k := range sortedKeys(mm) {
		entry, _ := mm[k].(map[string]any)
		if entry == nil {
			continue
		}
		if skipContent && strings.EqualFold(gstr(entry, "mode"), "content") {
			continue
		}
		refs := garr(entry, "pageReferences")
		cols := garr(entry, "displayColumns")
		if len(refs) == 0 || len(cols) == 0 {
			continue
		}
		for _, it := range refs {
			rm, ok := it.(map[string]any)
			if !ok {
				continue
			}
			add(pageRefID(rm))
		}
	}
}

func pageRefID(m map[string]any) int64 {
	if m == nil {
		return 0
	}
	return jsonInt64(m["id"])
}

func jsonInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	}
	return 0
}
