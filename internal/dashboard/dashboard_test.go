package dashboard

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	m "github.com/mrmmg/gonix/internal/metrics"
)

type found struct{ where, expr, legend string }

// load returns the generated dashboard and every found in it: the panels'
// targets and the variables' definitions.
func load(t *testing.T) (map[string]any, []found) {
	t.Helper()
	raw, err := JSON()
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	var qs []found
	for _, p := range d["panels"].([]any) {
		panel := p.(map[string]any)
		title, _ := panel["title"].(string)
		targets, _ := panel["targets"].([]any)
		for _, tg := range targets {
			tt := tg.(map[string]any)
			expr, _ := tt["expr"].(string)
			legend, _ := tt["legendFormat"].(string)
			qs = append(qs, found{"panel " + title, expr, legend})
		}
	}
	for _, v := range d["templating"].(map[string]any)["list"].([]any) {
		vv := v.(map[string]any)
		if vv["type"] == "found" {
			qs = append(qs, found{"variable " + vv["name"].(string), vv["found"].(string), ""})
		}
	}
	return d, qs
}

var (
	reMetric   = regexp.MustCompile(`\bgonix_[a-z0-9_]+`)
	reMatcher  = regexp.MustCompile(`([a-z_]+)\s*(=~|!~|!=|=)\s*"`)
	reBy       = regexp.MustCompile(`\b(?:by|without)\s*\(([^)]*)\)`)
	reLegend   = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)
	reLabelVal = regexp.MustCompile(`label_values\(.*,\s*([a-z_]+)\)`)
)

func TestEveryQueryMatchesTheMetricDefinitions(t *testing.T) {
	_, qs := load(t)
	if len(qs) < 30 {
		t.Fatalf("only %d queries found; the dashboard should have more", len(qs))
	}
	for _, q := range qs {
		metrics := reMetric.FindAllStringIndex(q.expr, -1)
		allowed := map[string]bool{m.LabelInstance: true}
		found := 0
		for _, loc := range metrics {
			name := q.expr[loc[0]:loc[1]]
			if name == m.LabelInstance {
				continue // the label name, not a metric
			}
			found++
			def, ok := m.Lookup(name)
			if !ok {
				t.Errorf("%s: %s is not a metric GoNix exports (see internal/metrics/defs.go)", q.where, name)
				continue
			}
			for _, l := range def.Labels {
				allowed[l] = true
			}

			before := strings.TrimSpace(q.expr[:loc[0]])
			inRate := strings.HasSuffix(before, "rate(") || strings.HasSuffix(before, "increase(")
			switch {
			case def.Type == m.Counter && !inRate && !strings.HasPrefix(q.where, "variable"):
				t.Errorf("%s: counter %s must be used with rate() or increase()", q.where, name)
			case def.Type == m.Gauge && inRate:
				t.Errorf("%s: gauge %s must not be used with rate() or increase()", q.where, name)
			}
			if !strings.HasPrefix(q.where, "variable") && !strings.HasPrefix(q.expr[loc[1]:], `{`+m.LabelInstance+`=~"$instance"`) {
				t.Errorf("%s: %s is not filtered by the instance variable", q.where, name)
			}
		}

		if found == 0 {
			t.Errorf("%s: no gonix metric in %q", q.where, q.expr)
			continue
		}
		var used []string
		for _, mm := range reMatcher.FindAllStringSubmatch(q.expr, -1) {
			used = append(used, mm[1])
		}
		for _, mm := range reBy.FindAllStringSubmatch(q.expr, -1) {
			for _, l := range strings.Split(mm[1], ",") {
				used = append(used, strings.TrimSpace(l))
			}
		}
		for _, mm := range reLegend.FindAllStringSubmatch(q.legend, -1) {
			used = append(used, mm[1])
		}
		if mm := reLabelVal.FindStringSubmatch(q.expr); mm != nil {
			used = append(used, mm[1])
		}
		for _, l := range used {
			// "→" and spaces in legends are not labels; only names reach here.
			if !allowed[l] {
				t.Errorf("%s: label %q is not on the metrics used in %q", q.where, l, q.expr)
			}
		}
	}
}

func TestQueriesAreWellFormed(t *testing.T) {
	d, qs := load(t)
	defined := map[string]bool{"__rate_interval": true, "__range": true}
	for _, v := range d["templating"].(map[string]any)["list"].([]any) {
		defined[v.(map[string]any)["name"].(string)] = true
	}
	reVar := regexp.MustCompile(`\$\{?([A-Za-z_]+)`)
	for _, q := range qs {
		for _, pair := range []string{"()", "[]", "{}"} {
			if strings.Count(q.expr, pair[:1]) != strings.Count(q.expr, pair[1:]) {
				t.Errorf("%s: unbalanced %s in %q", q.where, pair, q.expr)
			}
		}
		if strings.Count(q.expr, `"`)%2 != 0 {
			t.Errorf("%s: unbalanced quotes in %q", q.where, q.expr)
		}
		for _, mm := range reVar.FindAllStringSubmatch(q.expr, -1) {
			if !defined[mm[1]] {
				t.Errorf("%s: uses undefined variable $%s", q.where, mm[1])
			}
		}
	}
}

func TestLayoutAndStructure(t *testing.T) {
	d, _ := load(t)
	if d["uid"] != UID || d["title"] != "GoNix" {
		t.Errorf("uid/title = %v/%v", d["uid"], d["title"])
	}
	var titles []string
	rows := 0
	for _, p := range d["panels"].([]any) {
		panel := p.(map[string]any)
		title, _ := panel["title"].(string)
		if title == "" {
			t.Errorf("a %v panel has no title", panel["type"])
		}
		if slices.Contains(titles, title) {
			t.Errorf("two panels are titled %q", title)
		}
		titles = append(titles, title)
		g := panel["gridPos"].(map[string]any)
		if g["x"].(float64)+g["w"].(float64) > 24 {
			t.Errorf("panel %q runs past the grid", title)
		}
		if panel["type"] == "row" {
			rows++
		} else if panel["datasource"] == nil {
			t.Errorf("panel %q has no data source", title)
		}
	}
	if rows != 7 {
		t.Errorf("%d rows, want 7", rows)
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	a, err := JSON()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := JSON()
	if !bytes.Equal(a, b) {
		t.Error("two runs differ; the file would change on every generation")
	}
}

func TestSaveReplacesOldFileAndAcceptsADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "mine.json")
	got, err := Save(file)
	if err != nil || got != file {
		t.Fatalf("Save = %q, %v", got, err)
	}
	os.WriteFile(file, []byte("old"), 0o644)
	if _, err := Save(file); err != nil {
		t.Fatal(err)
	}
	want, _ := JSON()
	if data, _ := os.ReadFile(file); !bytes.Equal(data, want) {
		t.Error("the old version was not replaced")
	}
	got, err = Save(dir)
	if err != nil || got != filepath.Join(dir, FileName) {
		t.Errorf("Save(dir) = %q, %v", got, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".gonix-dashboard-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}
