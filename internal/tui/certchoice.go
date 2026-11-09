package tui

import (
	"fmt"
	"path/filepath"

	"github.com/mrmmg/gonix/internal/certificates"
)

// certChoice is one certificate a host can be given: its two files, and
// what GoNix knows about it (nil for a custom path it has not inspected).
type certChoice struct {
	CertFile, KeyFile string
	Cert              *certificates.Certificate
}

// certChoices are the certificates offered by a wizard, as labelled options
// that always say whose certificate it is: GoNix's own, Let's Encrypt's
// (certbot), or a custom path that a host already uses.
type certChoices struct {
	Labels  []string
	By      map[string]certChoice
	Current string // the label of the certificate the host uses now, if any
}

// allCerts lists GoNix's certificates and certbot's.
func allCerts(deps Deps) []certificates.Certificate {
	return certificates.All(deps.Config.Certificates.Directory, deps.Config.Certificates.LetsEncryptDir)
}

func certLabel(c certificates.Certificate) string {
	label := fmt.Sprintf("%s  [%s] — %s", c.Domain, c.SourceLabel(), c.Short())
	if c.NeedsAttention() {
		label += "  ⚠"
	}
	return label
}

// newCertChoices lists every certificate. currentCert/currentKey are the
// files the host uses now: if they are not among the listed certificates
// they are offered too, as a custom certificate that is kept as it is.
func newCertChoices(deps Deps, currentCert, currentKey string) certChoices {
	ch := certChoices{By: map[string]certChoice{}}
	cur := ""
	if currentCert != "" {
		cur = filepath.Clean(currentCert)
	}
	add := func(label string, c certChoice) {
		if _, taken := ch.By[label]; taken {
			label += " " + c.CertFile // two certificates that read the same
		}
		ch.Labels = append(ch.Labels, label)
		ch.By[label] = c
		if cur != "" && filepath.Clean(c.CertFile) == cur {
			ch.Current = label
		}
	}
	for _, c := range allCerts(deps) {
		c := c
		add(certLabel(c), certChoice{CertFile: c.CertPath, KeyFile: c.KeyPath, Cert: &c})
	}
	if cur != "" && ch.Current == "" {
		label := currentCert + "  [Custom] — kept as it is"
		ch.Labels = append([]string{label}, ch.Labels...)
		ch.By[label] = certChoice{CertFile: currentCert, KeyFile: currentKey}
		ch.Current = label
	}
	return ch
}

// Describe says, in words, which certificate a choice is.
func (c certChoice) Describe() string {
	if c.Cert == nil {
		return "the custom certificate " + c.CertFile
	}
	return fmt.Sprintf("the %s certificate for %s (%s)", c.Cert.SourceLabel(), c.Cert.Domain, c.Cert.Short())
}
