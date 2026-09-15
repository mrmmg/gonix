// Package hosts implements the safe, high-level host management workflow:
// validating user input, writing configuration through internal/nginx,
// testing it, and rolling back automatically on failure while recording
// every action to the audit log.
package hosts

import (
	"fmt"
	"regexp"
	"strings"
)

// domainLabel matches a single DNS label: letters, digits and hyphens, not
// starting or ending with a hyphen, at most 63 characters.
var domainLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// ValidateDomain checks that domain looks like a syntactically valid DNS
// host name suitable for use as an Nginx server_name and configuration file
// name (example.com, www.example.com, api.example.com, ...).
func ValidateDomain(domain string) error {
	d := strings.TrimSpace(domain)
	if d == "" {
		return fmt.Errorf("domain name must not be empty")
	}
	if len(d) > 253 {
		return fmt.Errorf("domain name is too long (max 253 characters)")
	}
	if strings.ContainsAny(d, "/\\ \t") {
		return fmt.Errorf("domain name %q must not contain spaces or path separators", d)
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return fmt.Errorf("domain name %q must have at least two labels (e.g. example.com)", d)
	}
	for _, label := range labels {
		if !domainLabel.MatchString(label) {
			return fmt.Errorf("domain name %q contains an invalid label %q", d, label)
		}
	}
	return nil
}

// ValidatePath validates an Nginx location path, e.g. "/", "/api/".
func ValidatePath(path string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("location path must not be empty")
	}
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "=") && !strings.HasPrefix(p, "~") {
		return fmt.Errorf("location path %q must start with / (or a valid Nginx location modifier)", p)
	}
	if strings.ContainsAny(p, "{};") {
		return fmt.Errorf("location path %q must not contain { } or ;", p)
	}
	return nil
}
