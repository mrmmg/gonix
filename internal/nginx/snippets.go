package nginx

import "regexp"

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
