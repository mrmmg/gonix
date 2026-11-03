package diff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	if Unified("a\n", "a\n", "x", "y") != "" {
		t.Error("equal inputs must give an empty diff")
	}
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	b := "1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n"
	got := Unified(a, b, "a/f", "b/f")
	want := `--- a/f
+++ b/f
@@ -1,6 +1,6 @@
 1
 2
-3
+three
 4
 5
 6
@@ -10,3 +10,4 @@
 10
 11
 12
+13
`
	if got != want {
		t.Errorf("Unified =\n%s\nwant\n%s", got, want)
	}

	created := Unified("", "x\ny\n", "/dev/null", "b/new")
	if !strings.Contains(created, "@@ -0,0 +1,2 @@\n+x\n+y\n") {
		t.Errorf("new file diff =\n%s", created)
	}
	removed := Unified("x\n", "", "a/old", "/dev/null")
	if !strings.Contains(removed, "@@ -1 +0,0 @@\n-x\n") {
		t.Errorf("removed file diff =\n%s", removed)
	}
}
