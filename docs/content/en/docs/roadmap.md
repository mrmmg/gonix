---
title: Roadmap
weight: 200
---

> **What is GoNix missing, and is that on purpose?**
> Mostly yes. These are designed for, and deliberately not implemented yet.

- **More DNS providers for wildcard certificates.** Only Cloudflare is supported today.
- **WAF / ModSecurity integration.**
- **Active health checks.** Open-source Nginx has only passive ones.
- **Importing arbitrary hand-written Nginx configuration** into managed hosts. Today those hosts stay
  read-only `[unmanaged]`.

## Left out on purpose

**OCSP stapling.** Let's Encrypt stopped running OCSP responders in 2025, so there is nothing to staple.

## Have an idea?

Open an issue at [github.com/mrmmg/gonix/issues](https://github.com/mrmmg/gonix/issues). See also the
[changelog](https://github.com/mrmmg/gonix/blob/main/CHANGELOG.md).
