---
title: Migration
weight: 120
---

> **Moving servers is where configuration goes to be forgotten.**
> GoNix gives you two migration paths, and both start with `--dry-run`.

## Moving to a new server

Export everything GoNix manages into one archive, then import it on the other machine.

```bash
sudo gonix export gonix-backup.tar.gz                 # on the old server
sudo gonix import gonix-backup.tar.gz --dry-run       # on the new one: see what would change
sudo gonix import gonix-backup.tar.gz
```

- Export to standard output with `-` instead of a file name, for piping over SSH.
- Import refuses to overwrite existing files unless you add `--overwrite --yes`.

{{< callout type="info" >}}
**Why dry-run an import?** An import is the biggest change you can make in one command. Reading the diff
costs ten seconds and can save the evening.
{{< /callout >}}

## Leaving Nginx Proxy Manager

> **Do you have to recreate thirty hosts by hand?**
> No. Point GoNix at NPM's data directory.

```bash
sudo gonix npm-import --data /opt/npm/data --letsencrypt /opt/npm/letsencrypt --dry-run
```

It brings over proxy hosts, redirects, streams, certificates and access lists. `--disabled` imports the
hosts without enabling them, which is a good first step: look, then enable one by one.

Anything that cannot be mapped (custom advanced configuration, wildcard-only hosts) is **reported instead
of guessed**. A silent wrong guess in a proxy configuration is worse than a visible gap.
