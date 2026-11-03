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

func TestParseAliases(t *testing.T) {
	got, err := ParseAliases(" www.example.com, example.org  www.example.com EXAMPLE.com", "example.com")
	if err != nil {
		t.Fatalf("ParseAliases: %v", err)
	}
	want := []string{"www.example.com", "example.org"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ParseAliases = %v, want %v", got, want)
	}
	if got, err := ParseAliases("", "example.com"); err != nil || len(got) != 0 {
		t.Errorf("ParseAliases(\"\") = %v, %v; want no aliases", got, err)
	}
	if _, err := ParseAliases("www.example.com bad_name", "example.com"); err == nil {
		t.Error("expected an error for an invalid alias")
	}
}
