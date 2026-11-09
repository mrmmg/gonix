---
title: مرجع دستورها
weight: 1
---

`gonix help` همیشه فهرست مرجع و به‌روز است. این صفحه همان دستورها را بر اساس کار دسته‌بندی می‌کند.
دستورهای دارای **✎** پیکربندی را تغییر می‌دهند و `--dry-run` می‌پذیرند.

## هاست‌ها

| دستور | توضیح |
|---|---|
| `host list` | فهرست هاست‌ها |
| `host show <domain> [--raw]` | نمایش تنظیمات، یا پیکربندی خام |
| ✎ `host add <domain> (--upstream URL \| --group NAME \| --root DIR \| --redirect URL) [flags]` | ساخت و فعال‌کردن هاست |
| ✎ `host set <domain> [flags]` | تغییر تنظیمات هاست |
| ✎ `host enable <domain>` / `host disable <domain>` | فعال‌کردن، یا غیرفعال‌کردن با نگه‌داشتن فایل |
| ✎ `host delete <domain> --yes` | حذف هاست (یک پشتیبان می‌ماند) |
| ✎ `host path add <domain> <path> (--upstream URL \| --group NAME) [--websocket]` | افزودن location ریورس‌پروکسی |
| ✎ `host path remove <domain> <path>` | حذف یک location |

پرچم‌های رایج `host add` / `host set`:

| پرچم | معنی |
|---|---|
| `--ssl NAME` | نام پوشه‌ی گواهی (`none` برای جداکردن) |
| `--http2`، `--http3`، `--force-https` | نسخه‌های HTTP و ریدایرکت HTTP به HTTPS |
| `--presets a,b` | [Presetها]({{< relref "../traffic" >}}) |
| `--ip-list`، `--country-list`، `--access-list`، `--rate-limit`، `--error-pages` | وصل‌کردن یک سیاست نام‌دار (`none` برای جداکردن) |
| `--satisfy any\|all` | لیست IP *یا* / *و* Basic Auth |
| `--alias NAME` (add) / `--aliases "a b"` (set) | نام‌های دامنه‌ی اضافه |
| `--websocket on\|off` | پشتیبانی از WebSocket |
| `--maintenance on\|off`، `--bypass "IP ..."` | حالت تعمیر |
| `--access-log`، `--error-log` | `on` یا `off` |
| `--connect-timeout`، `--read-timeout`، `--send-timeout`، `--max-body` | تنظیم پروکسی |
| `--code`، `--no-preserve-path` | کد وضعیت ریدایرکت و رفتار مسیر |
| `--no-forward-headers` | هدرهای `Host`/`X-Forwarded-*` را نفرست |
| `--disabled` | هاست را بدون فعال‌کردن بساز |

## کنترل دسترسی

| دستور | توضیح |
|---|---|
| `accesslist list` | فهرست لیست‌های Basic Auth |
| ✎ `iplist create <name> --mode allow\|block --entries "IP/CIDR ..."` | ساخت لیست IP |
| ✎ `iplist add <name> <ip/cidr>` / `iplist remove <name> <ip/cidr>` | ویرایش لیست |
| ✎ `iplist delete <name> --yes` | حذف لیستِ بی‌استفاده |
| `iplist list` | فهرست لیست‌های IP |
| ✎ `country setup` | بارگذاری دیتابیس GeoIP |
| ✎ `country create <name> --mode allow\|block --countries "DE FR ..."` | ساخت لیست کشور |
| ✎ `country delete <name> --yes` | حذف لیستِ بی‌استفاده |
| `country list` | فهرست لیست‌های کشور |
| `realip show` | نمایش تنظیمات IP واقعی کاربر |
| ✎ `realip cloudflare [--refresh]` | اعتماد به کلودفلر |
| ✎ `realip custom --proxies "IP/CIDR ..." --header X-Forwarded-For\|X-Real-IP` | اعتماد به پروکسی‌های خودتان |
| ✎ `realip disable` | توقف اعتماد به هدرهای پروکسی |

## ترافیک، توزیع بار و stream

| دستور | توضیح |
|---|---|
| `preset list` | فهرست presetهای هاست |
| ✎ `ratelimit create <name> [--preset general\|api\|login] [--rate N --per s\|m --burst N --nodelay --conn N]` | ساخت محدودیت نرخ |
| ✎ `ratelimit delete <name> --yes` / `ratelimit list` | حذف / فهرست |
| ✎ `upstream create <name> --servers "host:port[ weight=N][ backup], ..." [--method round_robin\|least_conn\|ip_hash]` | ساخت گروه upstream |
| ✎ `upstream delete <name> --yes` / `upstream list` | حذف / فهرست |
| ✎ `stream setup` | ساخت بلاک `stream { }` |
| ✎ `stream create <name> --port N --backend host:port [--udp] [--ip-list NAME]` | ساخت پروکسی stream |
| ✎ `stream enable\|disable\|delete <name>` / `stream list` | مدیریت / فهرست |
| `errorpages list` | فهرست snippetهای صفحه‌ی خطا |

## مشاهده

| دستور | توضیح |
|---|---|
| `logs <domain> [--lines N] [--filter "status:5xx ip:... path:..."] [--summary] [--error]` | درخواست‌ها یا خطاهای اخیر |
| `audit [--lines N]` | آخرین تغییرهای ثبت‌شده در audit log |
| `check [--no-upstreams]` | Config Check |
| `cert list [--letsencrypt DIR\|none]` | گواهی‌ها و انقضایشان، به‌همراه گواهی‌های certbot |
| `nginx status\|test\|reload` | کنترل سرویس |

## تاریخچه و جابه‌جایی

| دستور | توضیح |
|---|---|
| `history list [--lines N]` / `history show <commit>` | مرور تاریخچه |
| `history restore <commit> --yes` | برگرداندن یک کامیت |
| `export <file.tar.gz \| ->` | خروجی از همه‌چیزی که گونیکس مدیریت می‌کند |
| ✎ `import <file.tar.gz> [--overwrite --yes]` | ورود یک خروجی |
| ✎ `npm-import --data DIR [--letsencrypt DIR] [--disabled]` | ورود از Nginx Proxy Manager |
| ✎ `adopt (<domain\|file>... \| --all) [--verify-renewal]` | مدیریت فایل‌های هاست دست‌نویس (فقط یک کامنت علامت اضافه می‌شود) |
| ✎ `adopt release <domain>` | کنار گذاشتن مدیریت یک فایل adopt‌شده (محتوایش حفظ می‌شود) |

## ابزارها

| دستور | توضیح |
|---|---|
| `update [--check] [--rollback] [--yes]` | به‌روزرسانی GoNix از آخرین Release در GitHub: بررسی checksum، نگه‌داشتن نسخه‌ی قبلی با نام `gonix.old` و راه‌اندازی دوباره‌ی exporter مانیتورینگ |
| `instances` | فهرست نمونه‌های پیکربندی‌شده‌ی Nginx |
| `skill install [--project \| --dir DIR]` / `skill print` | [Skill عامل هوش مصنوعی]({{< relref "../ai-agents" >}}) |
| `mcp` | سرور MCP روی stdio |
| `--version`، `-v` | چاپ نسخه |
