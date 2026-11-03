---
title: GoNix
layout: hextra-home
---

{{< logo >}}

{{< hextra/hero-badge link="https://github.com/mrmmg/gonix/releases/latest" >}}
  <span>Free, open source, MIT licensed</span>
  {{< icon name="arrow-circle-right" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Manage Nginx from the terminal.&nbsp;<br class="hx:sm:block hx:hidden" />Without hiding it.
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  GoNix is a safe, interactive layer on top of your real Nginx files.&nbsp;<br class="hx:sm:block hx:hidden" />Open any config in an editor and it is plain Nginx, because it always was.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="Get started" link="docs/quickstart" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Nothing is hidden"
    subtitle="No database, no proprietary format. Hosts are ordinary files in sites-available, named after their domain."
  >}}
  {{< hextra/feature-card
    title="Nginx is never left broken"
    subtitle="Every change is backed up, tested with nginx -t and rolled back automatically when the test fails."
  >}}
  {{< hextra/feature-card
    title="TUI, CLI and AI agents"
    subtitle="The same actions are available as a terminal UI, as scriptable commands with --json and --dry-run, and as an MCP server."
  >}}
  {{< hextra/feature-card
    title="Everything you reach for"
    subtitle="Reverse proxy, redirects, SSL and wildcard certificates, access lists, rate limits, load balancing, TCP/UDP streams, live logs."
  >}}
  {{< hextra/feature-card
    title="Time machine included"
    subtitle="Every successful change is a Git commit outside /etc/nginx. Browse the diffs, restore any earlier state."
  >}}
  {{< hextra/feature-card
    title="One static binary"
    subtitle="Install with one command on linux/amd64 or linux/arm64. No runtime, no daemon, nothing to keep running."
  >}}
{{< /hextra/feature-grid >}}
