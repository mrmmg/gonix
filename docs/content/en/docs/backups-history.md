---
title: Backups and history
weight: 110
---

> **What if a change goes through, passes `nginx -t`, and is still wrong?**
> The test only proves Nginx can *start*, not that the change was a good idea. For that you need to be
> able to go back, and GoNix keeps two safety nets for it.

## Net 1: backups, taken before every write

Before GoNix writes a host's file, it saves a timestamped copy under `/etc/gonix/backups/`. Then it writes
and runs `nginx -t`:

- **Test passes**: Nginx is reloaded and the success goes in the audit log.
- **Test fails**: the previous configuration is restored from the backup just taken, Nginx is **never**
  reloaded with broken config, and the failure (with Nginx's error output) goes in the audit log.

Old backups beyond `backup.keep_count` (default 20 per file) are pruned. The **Backup / Restore** screen on
each host lists the retained snapshots and restores any of them, along the same test-before-reload path.

## Net 2: history, a Git repository of everything

When `history.enabled` is on (the default) and `git` is installed, every successful change is committed to
`/etc/gonix/history.git`, with `/etc/nginx` as its work tree. Nothing is added **inside** `/etc/nginx`.

```bash
sudo gonix history list --lines 10
sudo gonix history show 3f9c2ab           # the diff of that change
sudo gonix history restore 3f9c2ab --yes  # tested with nginx -t first, like everything else
```

The TUI has the same thing under **Configuration History**.

{{< callout type="info" >}}
**Why keep the Git directory outside `/etc/nginx`?** So the work tree stays clean: no hidden `.git` for
backup tools or other software to trip over, and nothing that can end up in an `include` by accident.
{{< /callout >}}

## Which one do I use?

| Situation | Use |
|---|---|
| "I just saved this host and it's wrong" | Host **Backup / Restore** (fast, per file) |
| "Something changed last week, what?" | `history list` and `history show` |
| "Roll everything back to Tuesday" | `history restore` |
| "Move everything to a new machine" | [Export / import]({{< relref "migration" >}}) |
