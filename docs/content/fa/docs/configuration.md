---
title: مرجع پیکربندی
weight: 170
---

> **گونیکس تنظیمات و مسیرهایش را کجا نگه می‌دارد، و می‌توانم عوضشان کنم؟**
> همه‌چیز در یک فایل YAML است: `/etc/gonix/gonix.yaml`. هیچ مسیری داخل خود برنامه ثابت نوشته نشده است.

نصب‌کننده این فایل را یک بار می‌سازد و **هرگز بازنویسی‌اش نمی‌کند**. با `--config PATH` یا متغیر محیطی `GONIX_CONFIG` به فایل دیگری
اشاره کنید. پیش‌فرض همراه برنامه
[`configs/default.yaml`](https://github.com/mrmmg/gonix/blob/main/configs/default.yaml) است.

## `nginx`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `config_dir` | `/etc/nginx` | ریشه‌ی پیکربندی Nginx |
| `sites_available` | `/etc/nginx/sites-available` | محل فایل‌های هاست |
| `sites_enabled` | `/etc/nginx/sites-enabled` | symlinkهای هاست‌های فعال |
| `binary_path` | `nginx` | باینری Nginx |
| `config_file` | `""` | به `nginx -t -c` داده می‌شود. خالی یعنی پیش‌فرض باینری. |
| `service` | `nginx` | واحد systemd که بعد از هر تغییر reload می‌شود |
| `http_snippets_dir` | `/etc/nginx/conf.d` | فایل‌های سطح http (IP واقعی، zoneهای محدودیت نرخ، bypass تعمیر). `nginx.conf` باید آن را داخل `http` include کند. |
| `cache_dir` | `/var/cache/nginx/gonix` | محل ذخیره‌ی پاسخ‌ها در preset مربوط به `proxy-cache` |
| `streams_dir` | `/etc/nginx/gonix-streams` | پروکسی‌های TCP/UDP |
| `main_include_dir` | `/etc/nginx/modules-enabled` | پوشه‌ی include سطح بالا که گونیکس می‌تواند بلاک `stream` را در آن بسازد |
| `http_port` | `80` | پورتی که هاست‌های جدید برای HTTP روی آن گوش می‌دهند |
| `https_port` | `443` | پورتی که هاست‌های جدید برای HTTPS روی آن گوش می‌دهند؛ اگر 443 نباشد، در ریدایرکت HTTP به HTTPS آورده می‌شود |

## `certificates`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `directory` | `/etc/nginx/certs` | یک زیرپوشه برای هر دامنه، با `fullchain.pem` و `privkey.pem` |

## `logs`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `nginx_directory` | `/var/log/nginx` | لاگ دسترسی و خطای هر هاست |
| `audit_file` | `/var/log/gonix/audit.log` | audit log |

## `backup`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `directory` | `/etc/gonix/backups` | نسخه‌های تاریخ‌دار، قبل از هر نوشتن |
| `keep_count` | `20` | تعداد نسخه‌های پشتیبانی که برای هر فایل نگه داشته می‌شود؛ قدیمی‌ترها پاک می‌شوند |

## `access_lists`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `directory` | `/etc/gonix/accesslists` | فایل‌های `htpasswd` مربوط به Basic Auth |
| `group` | `""` | گروهی که مالک این فایل‌هاست تا workerها بتوانند بخوانندشان. خالی یعنی تشخیص خودکار. |

## `error_pages`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `snippets_directory` | `/etc/nginx/snippets` | فایل‌های تولیدشده‌ی `gonix-error-pages-<name>.conf` |
| `pages_directory` | `/var/www/html/error_pages` | پوشه‌ی پیشنهادی پیش‌فرض برای صفحه‌های HTML |

## `geoip`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `database` | `/usr/share/GeoIP/GeoLite2-Country.mmdb` | دیتابیس کشورهای MaxMind برای بلاک کردن کشورها |

## `history`

| کلید | پیش‌فرض | معنی |
|---|---|---|
| `enabled` | `true` | هر تغییر موفق را در Git کامیت می‌کند |
| `git_dir` | `/etc/gonix/history.git` | مخزن، بیرون از `/etc/nginx` |

## متغیرهای محیطی

| متغیر | معنی |
|---|---|
| `GONIX_CONFIG` | فایل پیکربندی مورد استفاده |
| `GONIX_INSTANCE` | نمونه‌ای که مدیریت می‌شود (مثل `--instance`) |
| `GONIX_ACTOR` | نامی که به‌عنوان actor در audit log ثبت می‌شود |
