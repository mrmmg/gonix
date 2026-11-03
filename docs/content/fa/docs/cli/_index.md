---
title: خط فرمان
weight: 140
---

> **می‌توانم گونیکس را در اسکریپت، در CI یا از cron هم استفاده کنم، یا فقط یک منوی خوشگل است؟**
> هر کاری که از TUI بشود کرد، یک دستور هم دارد.

`gonix` را بدون آرگومان اجرا کنید تا TUI باز شود، یا یک دستور به آن بدهید:

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 --ssl app.example.com --presets security-headers,hsts
sudo gonix host add example.net --redirect https://example.com --code 301
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24 198.51.100.7"
sudo gonix host set app.example.com --ip-list office --dry-run   # فقط diff را نشان بده، چیزی را عوض نکن
sudo gonix logs app.example.com --filter "status:5xx" --summary
sudo gonix check --json
```

## سه پرچمی که مدام به کارتان می‌آید

| پرچم | چه کاری می‌کند |
|---|---|
| `--dry-run` | فایل‌هایی را که قرار است عوض شوند، به‌شکل diff نشان می‌دهد، بدون اینکه به چیزی دست بزند |
| `--json` | خروجی قابل خواندن برای برنامه‌ها می‌دهد. خطاها به شکل `{"ok":false,"error":"..."}` هستند |
| `--yes`، `-y` | دستورهای خطرناک (حذف، برگرداندن، بازنویسی) را تأیید می‌کند |

{{< callout type="info" >}}
**چرا `--yes` وجود دارد؟** اسکریپتی که به‌خاطر یک غلط تایپی یک هاست را حذف کند، نباید فقط یک کلید با فاجعه فاصله داشته باشد.
دستورهای خطرناک از شما می‌خواهند صریحاً تأییدشان کنید.
{{< /callout >}}

پرچم‌های سراسری دیگر: `--config PATH` (یا `GONIX_CONFIG`) و `--instance NAME` (یا `GONIX_INSTANCE`).

## کدهای خروج

| کد | معنی |
|---|---|
| `0` | موفق |
| `1` | دستور شکست خورد (با `--json`: `{"ok":false,"error":"..."}`) |
| `2` | خطای استفاده: پرچم یا آرگومان اشتباه |

## همان مسیر امن TUI

هر تغییر از همان مسیر TUI می‌گذرد: پشتیبان، `nginx -t` و بعد reload یا برگرداندن. در audit log ثبت می‌شود (با `actor=cli`
یا مقدار `GONIX_ACTOR`) و در تاریخچه کامیت می‌شود.

## یادگرفتن دستورها

```bash
gonix help               # همه‌ی دستورها
gonix help host          # فقط دستورهایی که «host» دارند
```

در خروجی help، دستورهایی که با `*` مشخص شده‌اند پیکربندی را تغییر می‌دهند و `--dry-run` را می‌پذیرند.

{{< cards >}}
  {{< card link="reference" title="مرجع دستورها" subtitle="همه‌ی دستورها، بر اساس موضوع." icon="book-open" >}}
  {{< card link="recipes" title="دستورهای آماده" subtitle="کارهای واقعی، از اول تا آخر." icon="beaker" >}}
{{< /cards >}}
