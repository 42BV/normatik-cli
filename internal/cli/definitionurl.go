package cli

import (
	"encoding/json"

	"github.com/42BV/normatik-cli/internal/command"
	"github.com/42BV/normatik-cli/internal/weburl"
	"github.com/spf13/cobra"
)

// The --url of a page type, enumeration or work item type follows the origin of the definition (NORM-jxgyeifw):
// a definition local to a space opens in the settings of that space, a shared one in the global administration.
// The origin comes from originSpaceKey in the definition response, never from --space or the default space.

// originSpaceKey reads originSpaceKey from a definition response; "" for a shared definition.
func originSpaceKey(body []byte) string {
	var v struct {
		OriginSpaceKey string `json:"originSpaceKey"`
	}
	_ = json.Unmarshal(body, &v)
	return v.OriginSpaceKey
}

// pageTypeURL is the route of page type id, given its definition response.
func pageTypeURL(id int64, body []byte) string {
	if key := originSpaceKey(body); key != "" {
		return weburl.SpacePageType(key, id)
	}
	return weburl.PageType(id)
}

// domainEnumURL is the route of enumeration id, given its definition response.
func domainEnumURL(id int64, body []byte) string {
	if key := originSpaceKey(body); key != "" {
		return weburl.SpaceDomainEnum(key, id)
	}
	return weburl.AdminDomainEnum(id)
}

// workItemTypeURL is the route of work item type slug, given its definition response.
func workItemTypeURL(slug string, body []byte) string {
	if key := originSpaceKey(body); key != "" {
		return weburl.SpaceWorkItemType(key, slug)
	}
	return weburl.AdminWorkItemType(slug)
}

// urlRequested reports whether --url is set, so a command only reads a definition for its URL when it prints one.
func urlRequested(cmd *cobra.Command) bool {
	url, _ := cmd.Flags().GetBool("url")
	return url
}

// lookupPageTypeURL is pageTypeURL for a command whose response is not the page type (descriptors, sort). With --url
// it reads the page type for its origin; a failed read falls back to the global route, as before.
func lookupPageTypeURL(cmd *cobra.Command, d *command.Deps, id int64) string {
	if !urlRequested(cmd) {
		return weburl.PageType(id)
	}
	body, apiErr := d.Client.GetPageType(cmd.Context(), id, nil)
	if apiErr != nil {
		return weburl.PageType(id)
	}
	return pageTypeURL(id, body)
}

// lookupWorkItemTypeURL is workItemTypeURL for a command whose response is not the type (transitions). With --url it
// reads the type in --space for its origin; a failed read falls back to the global route, as before.
func lookupWorkItemTypeURL(cmd *cobra.Command, d *command.Deps, slug, space string) string {
	if !urlRequested(cmd) {
		return weburl.AdminWorkItemType(slug)
	}
	body, apiErr := d.Client.GetWorkItemType(cmd.Context(), slug, space)
	if apiErr != nil {
		return weburl.AdminWorkItemType(slug)
	}
	return workItemTypeURL(slug, body)
}

// lookupDomainEnumURL is domainEnumURL for a command whose response is not the enumeration (usages). With --url it
// reads the enumeration for its origin; a failed read falls back to the global route, as before.
func lookupDomainEnumURL(cmd *cobra.Command, d *command.Deps, id int64) string {
	if !urlRequested(cmd) {
		return weburl.AdminDomainEnum(id)
	}
	body, apiErr := d.Client.GetDomainEnum(cmd.Context(), id)
	if apiErr != nil {
		return weburl.AdminDomainEnum(id)
	}
	return domainEnumURL(id, body)
}
