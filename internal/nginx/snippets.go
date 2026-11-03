package nginx

import (
	"regexp"
	"strings"
)

// SnippetFilePrefix is prepended to every error pages snippet file name, so
// GoNix-generated snippets are easy to tell apart from anything else in the
// snippets directory.
const SnippetFilePrefix = "gonix-error-pages-"

var reSnippetName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidSnippetName reports whether name is usable as an error pages snippet
// name: letters, digits, '-' and '_' only, so it is always a safe file name.
func ValidSnippetName(name string) bool {
	return reSnippetName.MatchString(name)
}

// SnippetFileName returns the file name (without directory) of the error
// pages snippet called name, e.g. "gonix-error-pages-default.conf".
func SnippetFileName(name string) string {
	return SnippetFilePrefix + name + ".conf"
}

// IPListFilePrefix is prepended to every IP access list snippet file name.
const IPListFilePrefix = "gonix-ip-"

// IPListFileName returns the file name (without directory) of the IP
// access list called name, e.g. "gonix-ip-office.conf". Names follow the
// same rules as ValidSnippetName.
func IPListFileName(name string) string {
	return IPListFilePrefix + name + ".conf"
}

// PresetFilePrefix is prepended to every preset snippet file name.
const PresetFilePrefix = "gonix-preset-"

// PresetFileName returns the file name (without directory) of the preset
// called name, e.g. "gonix-preset-security-headers.conf".
func PresetFileName(name string) string {
	return PresetFilePrefix + name + ".conf"
}

// RateLimitFilePrefix is prepended to both files of a rate limit: the
// http-level zone definition and the server/location-level snippet.
const RateLimitFilePrefix = "gonix-ratelimit-"

// RateLimitFileName returns the file name (without directory) of the rate
// limit called name, e.g. "gonix-ratelimit-api.conf".
func RateLimitFileName(name string) string {
	return RateLimitFilePrefix + name + ".conf"
}

// MaintenanceFilePrefix is prepended to every host's http-level maintenance
// file, e.g. "gonix-maintenance-example.com.conf".
const MaintenanceFilePrefix = "gonix-maintenance-"

// MaintenanceFileName returns the http-level maintenance file name of the
// host serverName.
func MaintenanceFileName(serverName string) string {
	return MaintenanceFilePrefix + serverName + ".conf"
}

// MaintenanceVar returns the variable (without $) that is non-zero while
// serverName is in maintenance mode for the current client, e.g.
// "gonix_maint_example_com". Dots become "_" and hyphens "__", so distinct
// valid domain names never share a variable.
func MaintenanceVar(serverName string) string {
	r := strings.NewReplacer(".", "_", "-", "__")
	return "gonix_maint_" + r.Replace(strings.ToLower(serverName))
}

// ErrorPageURIPrefix is the common prefix of the internal URIs error pages
// snippets serve pages under (errorpages.URIPrefix and TemplateURI).
// Maintenance mode exempts it, so a custom 503 page can still be served.
const ErrorPageURIPrefix = "/__gonix_error_page"

// UpstreamFilePrefix is prepended to every upstream group's http-level file.
const UpstreamFilePrefix = "gonix-upstream-"

// UpstreamFileName returns the file name (without directory) of the upstream
// group called name, e.g. "gonix-upstream-app.conf".
func UpstreamFileName(name string) string {
	return UpstreamFilePrefix + name + ".conf"
}

// CountryListFilePrefix is prepended to both files of a country list: the
// http-level map and the server-level check hosts include.
const CountryListFilePrefix = "gonix-geo-"

// CountryListFileName returns the file name of the country list called
// name, e.g. "gonix-geo-eu_only.conf".
func CountryListFileName(name string) string {
	return CountryListFilePrefix + name + ".conf"
}

var reCountryListName = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// ValidCountryListName reports whether name is usable for a country list.
// Hyphens are not allowed because the name becomes part of a variable.
func ValidCountryListName(name string) bool { return reCountryListName.MatchString(name) }
