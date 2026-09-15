package hosts

import "testing"

func TestValidateDomain(t *testing.T) {
	valid := []string{"example.com", "www.example.com", "api.example.com", "my-app.example.co.uk"}
	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", d, err)
		}
	}

	invalid := []string{"", "example", "-example.com", "example-.com", "exa mple.com", "example.com/path", "a..com"}
	for _, d := range invalid {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("expected %q to be invalid", d)
		}
	}
}

func TestValidatePath(t *testing.T) {
	valid := []string{"/", "/api/", "/api/v1", "=exact", "~regex"}
	for _, p := range valid {
		if err := ValidatePath(p); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", p, err)
		}
	}
	invalid := []string{"", "api/", "/bad{path}", "/bad;path"}
	for _, p := range invalid {
		if err := ValidatePath(p); err == nil {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}
