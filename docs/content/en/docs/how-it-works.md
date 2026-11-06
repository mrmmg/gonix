---
title: How it works
weight: 30
---

> **If GoNix edits my Nginx files, what happens to the changes I make by hand?**
> They are respected. That one question explains the three ideas the whole tool is built on.

## Idea 1: your files are the source of truth

Tools like Nginx Proxy Manager keep your sites in a database and *generate* configuration from it.
That is convenient until the database and the files disagree, or until you want to do something the UI
does not offer, and then you are stuck.

GoNix has no database. A host is a normal file:

```text
/etc/nginx/sites-available/example.com   # always present, the source of truth
/etc/nginx/sites-enabled/example.com     # symlink, present only while enabled
```

- **Disabling** a host removes only the symlink. The file is never deleted.
- Files are named after the **domain**, not an opaque ID, so `ls` and `grep` still work.

{{< callout type="info" >}}
**Why files and not a database?** Because Nginx's configuration language is already the best
description of an Nginx setup. A layer that hides it ends up as a second, worse copy to keep in sync.
{{< /callout >}}

## Idea 2: managed vs. unmanaged

GoNix marks every file it generates with a first line:

```nginx
# Managed by GoNix.
```

On every scan, files with that marker are **managed**: GoNix can edit them. Files without it, the ones
you wrote yourself, are listed as **`[unmanaged]`** and shown read-only. You can still view their raw
configuration, but GoNix will not rewrite something it did not create.

A hand-written file can be **adopted** (`gonix adopt`, or "Adopt Host" in the TUI). That adds a different
marker, `# Adopted by GoNix.`, and nothing else. An adopted file is never re-rendered: GoNix edits only the
directives a change involves, reads the result back to verify it, and leaves the rest of the file as you
wrote it. See [Migration]({{< relref "migration#adopting-hand-written-hosts-and-certbot" >}}).

{{< callout type="warning" >}}
A managed file is read back into GoNix's model and re-rendered from its template whenever you change
that host through GoNix. A directive GoNix has no setting for may therefore not survive such a change.
For custom directives, use a **custom location block** (see [Hosts]({{< relref "hosts" >}})), or take
the file out of GoNix's hands by removing the marker line, which makes it `[unmanaged]`. Adopted files do
not have this limit.
{{< /callout >}}

## Idea 3: every change takes the same safe path

Whether you click a menu, run a command or an AI agent calls a tool, a change takes exactly one road:

```text
validate  →  back up  →  write  →  nginx -t  ─┬─ pass → reload → audit log → history commit
                                              └─ fail → restore backup → audit log (with nginx's error)
```

- **Nginx is never reloaded with broken configuration.** A failed test restores the previous, working
  file automatically.
- **Every action is recorded** in the audit log, with timestamp, Linux user, action, target and result.
- **Every success is a Git commit** (see [Backups and history]({{< relref "backups-history" >}})), so you
  can look at what changed and go back.

{{< callout type="info" >}}
**Why a single path?** A safety feature that only some screens use is a trap. Because the TUI, the CLI
and the MCP server all go through the same workflow layer, the guarantees hold everywhere.
{{< /callout >}}

## The layers, for the curious

The TUI never writes to `/etc/nginx` directly. It calls the workflow layer (`internal/hosts`), which
calls the rendering and filesystem layer (`internal/nginx`), plus backup and audit. The CLI and the MCP
server sit on the same services. If you want to contribute, see [Contributing]({{< relref "contributing" >}}).
