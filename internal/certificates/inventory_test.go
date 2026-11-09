package certificates

import (
	"strings"
	"testing"
	"time"
)

func TestSourceOf(t *testing.T) {
	cases := map[string]string{
		"/etc/nginx/certs/a.com/fullchain.pem":      SourceGoNix,
		"/etc/letsencrypt/live/a.com/fullchain.pem": SourceCertbot,
		"/srv/tls/a.com/fullchain.pem":              SourceCustom,
		"/etc/letsencrypt/archive/a/fullchain1.pem": SourceCustom,
	}
	for path, want := range cases {
		if got := SourceOf(path, "/etc/nginx/certs", "/etc/letsencrypt"); got != want {
			t.Errorf("SourceOf(%s) = %s, want %s", path, got, want)
		}
	}
}

func TestAttentionWindowAndShort(t *testing.T) {
	at := func(d time.Duration) Certificate {
		return Certificate{Domain: "a.com", Source: SourceCertbot, Status: StatusValid, NotAfter: time.Now().Add(d)}
	}
	if at(13*24*time.Hour).NeedsAttention() != true {
		t.Error("13 days left should need attention")
	}
	if at(20 * 24 * time.Hour).NeedsAttention() {
		t.Error("20 days left should not need attention")
	}
	if !at(-time.Hour).NeedsAttention() {
		t.Error("expired should need attention")
	}
	if got := at(42*24*time.Hour + time.Hour).Short(); got != "42d left" {
		t.Errorf("Short = %q", got)
	}
	if got := at(-3*24*time.Hour - time.Hour).Short(); got != "expired 3d ago" {
		t.Errorf("Short = %q", got)
	}
	out := Attention([]Certificate{at(5 * 24 * time.Hour), at(90 * 24 * time.Hour)})
	if len(out) != 1 || !strings.Contains(out[0], "Let's Encrypt") {
		t.Errorf("Attention = %v", out)
	}
}
