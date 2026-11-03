---
title: AI agents
weight: 150
---

> **Would you let an AI edit `/etc/nginx` directly?**
> Probably not. So GoNix gives agents a way in that is safer than your own hands.

An agent that edits Nginx files directly can break production with one stray character. An agent using
GoNix gets the same guarantees you do: backup, `nginx -t`, automatic rollback, audit log and Git history.
Two integrations are included.

## The skill

A skill that teaches an agent how to use the CLI safely: inspect first, `--dry-run` before every change,
never touch `/etc/nginx` by hand.

```bash
gonix skill install            # Claude Code, for your user (~/.claude/skills/gonix)
gonix skill install --project  # only for the current project (./.claude/skills/gonix)
gonix skill print              # print SKILL.md, for any other agent
```

The skill is embedded in the binary, so it always matches the version you run.

## The MCP server

An [MCP](https://modelcontextprotocol.io) server on stdio, with typed tools that map to the CLI commands:

```bash
claude mcp add gonix -- sudo -n gonix mcp
```

`sudo -n` means *never prompt*, so you need a sudoers rule that lets your user run `gonix` without a
password:

```text
# /etc/sudoers.d/gonix  (edit with visudo)
youruser ALL=(root) NOPASSWD: /usr/local/bin/gonix
```

{{< callout type="warning" >}}
That rule gives that user root-level Nginx control without a password. Grant it only to accounts you trust
with that, and rely on the audit log: changes made by agents are recorded with `actor=mcp`.
{{< /callout >}}

## How an agent should work

The skill encodes this loop, and it is a good habit for humans too:

```bash
sudo gonix host list --json                       # 1. look
sudo gonix host add api.example.com \
     --upstream http://127.0.0.1:3000 --dry-run   # 2. preview the diff
sudo GONIX_ACTOR=claude gonix host add api.example.com \
     --upstream http://127.0.0.1:3000             # 3. apply after approval
sudo gonix host show api.example.com --json       # 4. verify
sudo gonix check --json                           # 5. look for problems
```

- **Destructive commands need `--yes`.** Agents add it only when you explicitly asked for that.
- **`GONIX_ACTOR=<name>`** labels changes in the audit log, so you can tell who did what.
- If a change fails, GoNix has **already restored** the previous configuration. The agent reports the
  `nginx -t` output instead of retrying with manual edits.
