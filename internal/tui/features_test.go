package tui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/mrmmg/gonix/internal/certificates"
	"github.com/mrmmg/gonix/internal/config"
	"github.com/mrmmg/gonix/internal/update"
)

// writeCert makes dir/fullchain.pem and dir/privkey.pem for name, valid for
// the given time from now.
func writeCert(t *testing.T, dir, name string, valid time.Duration) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(valid),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(filepath.Join(dir, "privkey.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
}

func TestCertificatesFromBothStoresAreTaggedAndFlagged(t *testing.T) {
	deps := testDeps(t)
	le := t.TempDir()
	deps.Config.Certificates.LetsEncryptDir = le
	writeCert(t, filepath.Join(deps.Config.Certificates.Directory, "mine.com"), "mine.com", 90*24*time.Hour)
	writeCert(t, filepath.Join(le, "live", "theirs.com"), "theirs.com", 5*24*time.Hour)

	choices := newCertChoices(deps, "", "")
	if len(choices.Labels) != 2 {
		t.Fatalf("labels = %v", choices.Labels)
	}
	joined := strings.Join(choices.Labels, "\n")
	for _, want := range []string{"mine.com  [GoNix]", "theirs.com  [Let's Encrypt]", "⚠"} {
		if !strings.Contains(joined, want) {
			t.Errorf("labels lack %q:\n%s", want, joined)
		}
	}

	if w := strings.Join(certAlerts(deps), "\n"); !strings.Contains(w, "theirs.com") || !strings.Contains(w, "[Let's Encrypt]") || strings.Contains(w, "mine.com") {
		t.Errorf("alerts = %q", w)
	}

	// A certificate outside both stores is offered as custom and kept.
	custom := filepath.Join(t.TempDir(), "x", "fullchain.pem")
	choices = newCertChoices(deps, custom, "/k.pem")
	if !strings.Contains(choices.Current, "[Custom]") || choices.By[choices.Current].KeyFile != "/k.pem" {
		t.Errorf("custom choice = %q", choices.Current)
	}

	// The one a host uses is preselected.
	live := filepath.Join(le, "live", "theirs.com", "fullchain.pem")
	if c := newCertChoices(deps, live, ""); !strings.Contains(c.Current, "[Let's Encrypt]") {
		t.Errorf("current = %q", c.Current)
	}
}

func TestCertificatesScreenSearch(t *testing.T) {
	deps := testDeps(t)
	deps.Config.Certificates.LetsEncryptDir = t.TempDir()
	for _, n := range []string{"alpha.com", "beta.org", "gamma.com"} {
		writeCert(t, filepath.Join(deps.Config.Certificates.Directory, n), n, 90*24*time.Hour)
	}
	s := newCertificatesScreen(deps)
	if got := s.menu.Selected(); got != 0 {
		t.Fatalf("selected = %d", got)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if !s.menu.Searching() {
		t.Fatal("Ctrl+F did not open the search")
	}
	for _, r := range "beta" {
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// "q" typed in the box must not quit the screen, and the match maps back
	// to its index in the full list (the pinned row is item 0).
	if got := s.menu.Selected(); got != 2 {
		t.Errorf("selected = %d, want 2 (beta.org)", got)
	}
	if view := ansi.Strip(s.View(100, 30)); !strings.Contains(view, "beta.org") || strings.Contains(view, "alpha.com") {
		t.Errorf("view does not show only the match:\n%s", view)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if s.menu.Searching() || len(s.menu.visible()) != 4 {
		t.Error("Esc should close the search and show everything")
	}
	if !strings.Contains(s.View(100, 30), "Ctrl + F to search") {
		t.Error("the search hint is missing")
	}
}

func TestWizardBackKeepsAnswersAndReviewsBeforeApplying(t *testing.T) {
	applied := false
	fields := []wizardField{
		{Key: "a", Label: "A", Kind: fieldText},
		{Key: "b", Label: "B", Kind: fieldBool},
		{Key: "c", Label: "C", Kind: fieldText, ShowIf: func(v map[string]string) bool { return v["b"] == "yes" }},
	}
	w := newWizard("T", fields, func(v map[string]string) (screen, tea.Cmd) {
		applied = true
		if _, ok := v["c"]; ok {
			t.Errorf("an answer to a question no longer asked leaked: %v", v)
		}
		return newResultScreen(testDeps(t), "T", true, "ok", nil), nil
	}, nil).withReview(func(v map[string]string) []string { return []string{"a=" + v["a"]} })

	key := func(k tea.KeyMsg) { w.Update(k) }
	for _, r := range "one" {
		key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	key(tea.KeyMsg{Type: tea.KeyEnter})
	key(tea.KeyMsg{Type: tea.KeyRight}) // B: yes
	key(tea.KeyMsg{Type: tea.KeyEnter})
	key(tea.KeyMsg{Type: tea.KeyShiftTab}) // back to B, which remembers "yes"
	if w.idx != 1 || w.boolCursor != 1 {
		t.Fatalf("idx=%d boolCursor=%d, want B answered yes", w.idx, w.boolCursor)
	}
	key(tea.KeyMsg{Type: tea.KeyLeft}) // B: no, so C is not asked
	key(tea.KeyMsg{Type: tea.KeyEnter})
	if !w.reviewing || applied {
		t.Fatalf("reviewing=%v applied=%v: the summary must come before applying", w.reviewing, applied)
	}
	if !strings.Contains(w.View(100, 30), "a=one") {
		t.Error("the review does not show the summary")
	}
	key(tea.KeyMsg{Type: tea.KeyShiftTab}) // back from the review to B
	if w.reviewing || w.idx != 1 {
		t.Fatalf("reviewing=%v idx=%d after going back", w.reviewing, w.idx)
	}
	key(tea.KeyMsg{Type: tea.KeyEnter})
	key(tea.KeyMsg{Type: tea.KeyEnter}) // apply
	if !applied {
		t.Error("Enter on the review did not apply")
	}
}

func TestCertbotCertificateOffersRenewal(t *testing.T) {
	c := certificates.Certificate{Domain: "a.com", Source: certificates.SourceCertbot, Status: certificates.StatusValid,
		NotAfter: time.Now().Add(3 * 24 * time.Hour)}
	s := newCertActionScreen(testDeps(t), c)
	if !strings.Contains(s.menu.items[caRenew].title, "certbot") {
		t.Errorf("renew item = %q", s.menu.items[caRenew].title)
	}
	if !strings.Contains(s.View(100, 30), "Let's Encrypt") {
		t.Error("the screen does not say whose certificate it is")
	}
}

func TestHostsListSearchByDomainAliasAndBackend(t *testing.T) {
	deps := testDeps(t)
	createTestHost(t, deps, "shop.example.com")
	createTestHost(t, deps, "blog.example.org")
	s := newHostsListScreen(deps)
	search := func(q string) *hostsListScreen {
		s.menu.stopSearch()
		s.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
		for _, r := range q {
			s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		return s
	}
	for q, want := range map[string]string{"shop": "shop.example.com", "ORG": "blog.example.org", "127.0.0.1:8080 blog": "blog.example.org"} {
		search(q)
		sel := s.menu.Selected()
		if sel < 0 || s.hosts[sel].ServerName != want {
			t.Errorf("search %q selected %d, want %s", q, sel, want)
		}
	}
	search("nothing-like-this")
	if s.menu.Selected() != -1 {
		t.Error("no match should select nothing")
	}
	// Enter on "no match" must not panic or open anything.
	if _, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("Enter with no match did something")
	}
}

func TestFuzzyMatchAndHighlight(t *testing.T) {
	s, pos, ok := fuzzyMatch("shpex", "shop.example.com")
	if !ok || len(pos) != 5 || pos[0] != 0 {
		t.Fatalf("shpex: %v %v %v", s, pos, ok)
	}
	if _, _, ok := fuzzyMatch("xyz", "shop.example.com"); ok {
		t.Error("xyz should not match")
	}
	sub, _, _ := fuzzyMatch("exam", "shop.example.com")
	if sub <= s {
		t.Errorf("a contiguous match (%d) should beat a scattered one (%d)", sub, s)
	}
	// One letter is never fuzzy.
	if _, _, ok := fuzzyMatch("z", "abc"); ok {
		t.Error("single letters match only as substrings")
	}
	// Highlighting leaves the text and existing colours intact.
	styled := dangerStyle.Render("abc") + "def"
	got := highlight(styled, map[int]bool{1: true, 4: true})
	if ansi.Strip(got) != "abcdef" || !strings.Contains(got, "\x1b[4;7m") {
		t.Errorf("highlight = %q", got)
	}
}

func TestSearchRanksBestFirst(t *testing.T) {
	m := newSearchMenu([]menuItem{{title: "+ New", pinned: true}, {title: "a-b-c.test"}, {title: "abc.test"}}, "x")
	m.startSearch()
	m.input.SetValue("abc")
	m.refilter()
	if m.Selected() != 0 || m.origin[1] != 2 {
		t.Errorf("origin = %v: the contiguous match should come first", m.origin)
	}
}

func TestWizardChoiceListsAreSearchable(t *testing.T) {
	opts := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"}
	w := newWizard("T", []wizardField{{Key: "k", Label: "Pick", Kind: fieldChoice, Options: opts}},
		func(v map[string]string) (screen, tea.Cmd) { return nil, nil }, nil)
	w.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	for _, r := range "fxt" {
		w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Esc closes the search, not the wizard.
	if sel := w.menu.Selected(); sel != 5 {
		t.Fatalf("selected %d, want 5 (foxtrot)", sel)
	}
	w.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if w.menu.Searching() {
		t.Error("Esc should close the search")
	}
	// Short lists have no search box.
	short := newWizard("T", []wizardField{{Key: "k", Label: "Pick", Kind: fieldChoice, Options: opts[:3]}},
		func(v map[string]string) (screen, tea.Cmd) { return nil, nil }, nil)
	if short.menu.searchable {
		t.Error("a list of three needs no search")
	}
}

func TestSelectedRowKeepsItsHighlightAfterInnerColours(t *testing.T) {
	inner := dangerStyle.Render("red") + " tail"
	if got := renderSelected(inner, 20); ansi.Strip(got) != "red tail"+strings.Repeat(" ", 12) {
		t.Errorf("renderSelected = %q", got)
	}
}

// fakeRelease serves a release newer than the running version.
func fakeRelease(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"body":"Added things\n- one\n- two"}`, tag)
	}))
	old := newUpdateClient
	newUpdateClient = func() *update.Client { return &update.Client{HTTP: srv.Client(), API: srv.URL, Base: srv.URL} }
	t.Cleanup(func() { srv.Close(); newUpdateClient = old })
}

func TestMainMenuAnnouncesANewReleaseInTheBackground(t *testing.T) {
	deps := testDeps(t)
	deps.Config.Update.Check = true
	fakeRelease(t, "v99.0.0")
	m := newMainMenu(deps)
	if strings.Contains(ansi.Strip(m.View(100, 30)), "is available") {
		t.Fatal("nothing is known before the check has run")
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("a stale cache should start a check")
	}
	m.Update(cmd())
	view := ansi.Strip(m.View(110, 40))
	for _, want := range []string{"GoNix 99.0.0 is available", "sudo gonix update", "turned off in Settings", "version 99.0.0 is available"} {
		if !strings.Contains(view, want) {
			t.Errorf("main menu lacks %q:\n%s", want, view)
		}
	}
	// The next start knows from the cache, without asking again.
	if again := newMainMenu(deps); again.update == nil || again.Init() != nil {
		t.Error("the cached result should be shown at once and not re-checked")
	}
}

func TestNoCheckWhenDisabled(t *testing.T) {
	deps := testDeps(t) // Update.Check is off
	fakeRelease(t, "v99.0.0")
	if newMainMenu(deps).Init() != nil {
		t.Error("a disabled check must not contact GitHub")
	}
	// Check Now still works on request.
	s := newUpdateScreen(deps)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s.Update(cmd())
	if !strings.Contains(ansi.Strip(s.View(110, 40)), "Version 99.0.0 is available") {
		t.Errorf("view:\n%s", ansi.Strip(s.View(110, 40)))
	}
}

func TestUpToDateIsSaidPlainly(t *testing.T) {
	deps := testDeps(t)
	fakeRelease(t, "v"+Version)
	s := newUpdateScreen(deps)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s.Update(cmd())
	if !strings.Contains(ansi.Strip(s.View(110, 40)), "up to date") {
		t.Error("not said to be up to date")
	}
}

func TestSettingsSwitchesUpdateOptionsAndSavesThem(t *testing.T) {
	deps := testDeps(t)
	deps.ConfigPath = filepath.Join(t.TempDir(), "gonix.yaml")
	os.WriteFile(deps.ConfigPath, []byte("# my notes\nnginx:\n  http_port: 8080\n"), 0o644)
	deps.Config.Update.Check = true
	s := newSettingsScreen(deps)
	s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Check for updates: off
	if deps.Config.Update.Check {
		t.Error("the running configuration was not changed")
	}
	saved, _ := config.Load(deps.ConfigPath)
	if saved.Update.Check || !saved.Update.RestartExporter || saved.Nginx.HTTPPort != 8080 {
		t.Errorf("saved = %+v, http_port %d", saved.Update, saved.Nginx.HTTPPort)
	}
	if b, _ := os.ReadFile(deps.ConfigPath); !strings.Contains(string(b), "# my notes") {
		t.Error("the rest of the file was not kept")
	}
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // restart exporter: off
	if saved, _ = config.Load(deps.ConfigPath); saved.Update.RestartExporter {
		t.Error("restart_exporter should now be off")
	}
}
