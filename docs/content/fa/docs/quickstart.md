---
title: شروع سریع
weight: 20
---

> **چقدر طول می‌کشد تا یک سایت واقعی با HTTPS پشت Nginx داشته باشم؟**
> حدود پنج دقیقه. و در تمام این مدت می‌بینید گونیکس دقیقاً چه خط‌هایی می‌نویسد.

فرض کنید برنامه‌ای روی `127.0.0.1:3000` اجرا می‌شود و می‌خواهید `app.example.com` با HTTPS به آن برسد.

{{% steps %}}

### گواهی را در جایش بگذارید

گونیکس انتظار دارد گواهی‌ها در یک پوشه‌ی مرکزی باشند و هر دامنه زیرپوشه‌ی خودش را داشته باشد:

```text
/etc/nginx/certs/app.example.com/fullchain.pem
/etc/nginx/certs/app.example.com/privkey.pem
```

اگر هنوز گواهی ندارید، گونیکس می‌تواند یک گواهی Wildcard برایتان بگیرد. در صفحه‌ی
[گواهی SSL]({{< relref "ssl" >}}) توضیح داده‌ایم.

### اول پیش‌نمایش ببینید

با `--dry-run`، گونیکس فایل‌هایی را که قرار است بنویسد به‌شکل diff نشان می‌دهد، بدون اینکه به چیزی دست بزند:

```bash
sudo gonix host add app.example.com \
  --upstream http://127.0.0.1:3000 \
  --ssl app.example.com \
  --presets security-headers,hsts \
  --dry-run
```

### بعد اجرایش کنید

همان دستور را بدون `--dry-run` بزنید. گونیکس پشت صحنه از فایل قبلی پشتیبان می‌گیرد، فایل جدید را می‌نویسد، `nginx -t` را اجرا می‌کند
و فقط اگر تست موفق بود، Nginx را reload می‌کند.

### ببینید چه ساخته

```bash
sudo gonix host show app.example.com --raw
```

نتیجه، پیکربندی معمولی Nginx در `/etc/nginx/sites-available/app.example.com` است:

```nginx
# Managed by GoNix. Manual edits are preserved but will not be
# tracked by the application unless the host is re-imported.
# Domain: app.example.com

server {
    listen 80;
    listen 443 ssl;
    server_name app.example.com;

    if ($scheme != "https") {
        return 301 https://$host$request_uri;
    }

    ssl_certificate /etc/nginx/certs/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/app.example.com/privkey.pem;
    # include دو preset، لاگ دسترسی و خطا، و location پروکسی ...
}
```

به‌صورت پیش‌فرض، درخواست‌های HTTP به HTTPS ریدایرکت می‌شوند (با `--force-https=false` می‌توانید خاموشش کنید).

### یک بار هم بررسی کنید

```bash
sudo gonix check
```

Config Check دنبال این‌ها می‌گردد: server name تکراری، فایل‌های گم‌شده، گواهی‌هایی که نزدیک انقضا هستند و upstreamهایی که در دسترس نیستند.

{{% /steps %}}

## منو را ترجیح می‌دهید؟

دستور `sudo gonix` را بزنید و **Add New Host** را انتخاب کنید. یک ویزارد قدم‌به‌قدم است که همین گزینه‌ها را می‌پرسد
(upstream، پروتکل، timeout، WebSocket، SSL، preset و رمز عبور)، و هر کاری که انجام دهد از همان مسیر امن بالا می‌گذرد.

{{< callout type="info" >}}
**چرا دو رابط داریم؟** منو بهترین راه برای *کشف کردن* امکانات است و دستور بهترین راه برای *تکرار کردن* آن‌ها: در اسکریپت، در CI
یا توسط یک عامل هوش مصنوعی. هر دو یک کد را اجرا می‌کنند، پس هرگز با هم اختلاف پیدا نمی‌کنند.
{{< /callout >}}

### کلیدها

خط پایین هر صفحه کلیدهایی را که آن صفحه می‌پذیرد نشان می‌دهد. کلیدهای پرکاربرد:

| کلید | کار |
|---|---|
| `↑` `↓` (یا `k` `j`) | حرکت در منو، پیمایش متن |
| `Enter` | انتخاب |
| `Esc` (یا `q`) | برگشت؛ در منوی اصلی، خروج |
| `Ctrl+C` | خروج از هر جا |

متن‌های بلند (پیکربندی یک هاست، راهنمای آزمایشگاه) از تمام ارتفاع ترمینال استفاده می‌کنند و خط آخرشان نشان می‌دهد کجای
متن هستید؛ مثلاً `lines 1–40 of 120 · ↓ 80 more`. در این صفحه‌ها `PgUp` / `PgDn` (یا `b` / `Space`) یک صفحه جابه‌جا می‌کنند و
`Home` / `End` (یا `g` / `G`) به ابتدا یا انتهای متن می‌روند.

## بعد از این

- چرا گونیکس این‌طور طراحی شده: [گونیکس چطور کار می‌کند]({{< relref "how-it-works" >}})
- انواع دیگر هاست (سایت استاتیک، ریدایرکت، WebSocket): [هاست‌ها]({{< relref "hosts" >}})
