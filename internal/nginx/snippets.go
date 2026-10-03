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
