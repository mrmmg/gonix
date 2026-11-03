---
title: Error pages
weight: 80
---

> **Your backend restarts and visitors see a bare, grey "502 Bad Gateway". Is that the best you can do?**
> Not at all.

An **error pages snippet** is a named, reusable piece of Nginx configuration that answers selected HTTP
error codes with your own static HTML. Create and edit them in **Main Menu → Error Pages**; the directory
offered by default is `error_pages.pages_directory` (`/var/www/html/error_pages`). Each snippet stores
**one** pages directory and one of two modes.

## Mode 1: a page per code

One file per code, named `<code>.html` (`502.html`). A snippet called `default` handling 502 and 504 is
written to `/etc/nginx/snippets/gonix-error-pages-default.conf`:

```nginx
error_page 502 /__gonix_error_pages/502.html;
error_page 504 /__gonix_error_pages/504.html;

location ^~ /__gonix_error_pages/ {
    internal;
    auth_basic off;
    alias /var/www/html/error_pages/;
}
```

## Mode 2: a single template

One shared file (for example `error.html`) where every occurrence of a placeholder (default
`ERROR_CODE_PLACEHOLDER`) is replaced with the status code. A snippet named `branded` is written to `/etc/nginx/snippets/gonix-error-pages-branded.conf`:

```nginx
error_page 400 401 403 404 413 429 500 502 503 504 /__gonix_error_page;

location = /__gonix_error_page {
    internal;
    auth_basic off;
    root /var/www/html/error_pages;
    try_files /error.html =500;
    types { }
    default_type text/html;
    sub_filter 'ERROR_CODE_PLACEHOLDER' $status;
    sub_filter_once off;
}
```

`$status` holds the original error code, so one location serves every code and the response keeps its
status (still a real 502). Template mode needs Nginx built with `ngx_http_sub_module`:

```bash
nginx -V 2>&1 | grep -o http_sub_module
```

Without it `nginx -t` fails and GoNix rolls back. GoNix also warns if the template lacks the placeholder.

## Using a snippet

Choose it in **Add New Host**, or from a host's **Error Pages** menu (CLI: `--error-pages NAME`). The host
gets a single line:

```nginx
include /etc/nginx/snippets/gonix-error-pages-default.conf;
```

## Things worth knowing

- Changing a snippet's codes or directory updates every host that includes it, tested with `nginx -t`
  and rolled back on failure.
- A snippet in use cannot be deleted.
- `auth_basic off` keeps error pages viewable on hosts protected by an access list.
- Pages are served from an internal location, so nobody can request them directly. Inline your assets:
  CSS in `<style>`, images as `data:` URIs.
- Only errors **Nginx itself** returns are replaced, such as 502/504 when the upstream is down or slow. An
  error produced by your application passes through unchanged.
- A missing page file does not break the configuration. GoNix warns and shows each page's status.
- A **disabled** host has no server block at all, so its requests go to Nginx's default server, not to its error pages.
