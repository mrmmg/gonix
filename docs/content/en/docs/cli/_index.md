---
title: Command line
weight: 140
---

> **Can I use GoNix in a script, in CI, or from cron, or is it only a pretty menu?**
> Everything the TUI can do is also a command.

Run `gonix` with no arguments for the TUI, or with a command:

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 --ssl app.example.com --presets security-headers,hsts
sudo gonix host add example.net --redirect https://example.com --code 301
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24 198.51.100.7"
sudo gonix host set app.example.com --ip-list office --dry-run   # show the diff, change nothing
sudo gonix logs app.example.com --filter "status:5xx" --summary
sudo gonix check --json
```

## Three flags you will use constantly

| Flag | What it gives you |
|---|---|
| `--dry-run` | The exact files that would change, as a diff, without touching anything |
| `--json` | Machine-readable results. Errors are `{"ok":false,"error":"..."}` |
| `--yes`, `-y` | Confirms destructive commands (delete, restore, overwrite) |

{{< callout type="info" >}}
**Why does `--yes` exist?** A script that deletes a host because of a typo should not be one keystroke from
disaster. Destructive commands need you to say so out loud.
{{< /callout >}}

Other global flags: `--config PATH` (or `GONIX_CONFIG`) and `--instance NAME` (or `GONIX_INSTANCE`).

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | The command failed (with `--json`: `{"ok":false,"error":"..."}`) |
| `2` | Usage error: wrong flags or arguments |

## Same road as the TUI

Every change goes through the same backup, `nginx -t`, reload-or-rollback path as the TUI, is written to the
audit log (actor `cli`, or the value of `GONIX_ACTOR`) and committed to the history.

## Learning the commands

```bash
gonix help               # every command
gonix help host          # only commands matching "host"
```

Commands marked `*` in the help change the configuration and accept `--dry-run`.

{{< cards >}}
  {{< card link="reference" title="Command reference" subtitle="Every command, grouped by topic." icon="book-open" >}}
  {{< card link="recipes" title="Recipes" subtitle="Real tasks, start to finish." icon="beaker" >}}
{{< /cards >}}
