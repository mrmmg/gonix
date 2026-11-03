---
title: مدیریت ترافیک
weight: 70
---

> **از بین صدها snippet امنیتی که در اینترنت پیدا می‌شود، کدام را باید واقعاً کپی کنید؟**
> همان‌هایی که گونیکس به‌شکل preset آماده دارد: با `nginx -t` آزمایش شده‌اند، با یک پرچم اضافه می‌شوند و با یک پرچم برداشته می‌شوند.

## Presetها

```bash
sudo gonix preset list
sudo gonix host set app.example.com --presets security-headers,hsts,gzip
```

| Preset | چه می‌کند |
|---|---|
| `security-headers` | هدرهای `X-Frame-Options`، `X-Content-Type-Options`، `Referrer-Policy` و `Permissions-Policy` را می‌گذارد (جای هدرهای خودِ upstream) |
| `hsts` | مرورگرها تا یک سال فقط با HTTPS به این دامنه وصل می‌شوند (زیردامنه‌ها شامل نمی‌شوند) |
| `gzip` | متن، JSON، JavaScript، CSS، SVG و فونت را فشرده می‌کند |
| `static-cache` | مرورگر CSS، JavaScript، تصویر، فونت و ویدیو را ۳۰ روز کش می‌کند |
| `proxy-cache` | پاسخ‌هایی را کش می‌کند که backend اجازه‌ی کش‌شدنشان را داده؛ درخواست‌های دارای cookie یا `Authorization` همیشه به backend می‌رسند |
| `block-sensitive-files` | برای dotfileها (`.git`، `.env`، ...) و فایل‌های پشتیبان یا dump (`.bak`، `.sql`، ...) خطای `404` برمی‌گرداند، به‌جز `/.well-known/` |
| `block-bad-bots` | برای user agentهای معروف اسکنرها (sqlmap، nikto، nuclei و ...) خطای `403` برمی‌گرداند |

{{< callout type="warning" >}}
**HSTS را نمی‌شود پس گرفت.** وقتی مرورگر یک بار آن را ببیند، تا یک سال برای آن دامنه HTTP ساده را قبول نمی‌کند. فقط وقتی روشنش کنید
که مطمئنید HTTPS درست کار می‌کند.
{{< /callout >}}

preset مربوط به `proxy-cache` پاسخ‌ها را در `nginx.cache_dir` ذخیره می‌کند (پیش‌فرض `/var/cache/nginx/gonix`).

## محدودیت نرخ (Rate limiting)

> **یک کلاینت بدرفتار هر ثانیه ۵۰ بار به `/login` درخواست می‌زند. چه کنیم؟**
> برای آن مسیر یک سهمیه تعیین کنید.

محدودیت نرخ یک سیاست نام‌دار است که می‌توانید روی یک هاست یا یک location اعمال کنید. سه preset آماده، حالت‌های رایج را پوشش می‌دهند:

| Preset | نرخ | Burst | توضیح |
|---|---|---|---|
| `general` | ۳۰ در ثانیه | ۶۰، بدون تأخیر | تا ۵۰ اتصال هم‌زمان |
| `api` | ۱۰ در ثانیه | ۲۰، بدون تأخیر | |
| `login` | ۵ در دقیقه | ۵ | |

```bash
sudo gonix ratelimit create login --preset login
sudo gonix host path add app.example.com /login --upstream http://127.0.0.1:3000 --rate-limit login
```

یا سیاست خودتان را بسازید:

```bash
sudo gonix ratelimit create uploads --rate 2 --per s --burst 5 --nodelay --conn 10
```

{{< callout type="info" >}}
**چرا برای هر location جداگانه؟** صفحه‌ی ورود و یک CDN تصویر به محدودیت‌های کاملاً متفاوتی نیاز دارند. اگر برای حفاظت از یک فرم، کل سایت را
محدود کنید، همه‌ی کاربران تاوان می‌دهند. یادتان هم باشد که محدودیت‌ها بر اساس آدرس کار می‌کنند؛
[IP واقعی کاربر]({{< relref "access-control" >}}) را حتماً بخوانید.
{{< /callout >}}

## حالت تعمیر (Maintenance mode)

> **باید deploy کنید و ده دقیقه همه‌چیز به‌هم‌ریخته است. آیا بازدیدکننده‌ها یک سایت نیمه‌کاره می‌بینند؟**
> نه، می‌توانند یک صفحه‌ی `503` مرتب ببینند و شما در همان حال کارتان را انجام دهید.

```bash
sudo gonix host set app.example.com --maintenance on --bypass "203.0.113.5"
# ... deploy و تست ...
sudo gonix host set app.example.com --maintenance off
```

همه به‌جز آدرس‌های bypass خطای `503` می‌گیرند. می‌توانید با یک [صفحه‌ی خطای]({{< relref "error-pages" >}}) اختصاصی برای 503 ترکیبش کنید؛
آدرس‌های داخلی صفحه‌های خطای گونیکس از این محدودیت مستثنا هستند، پس صفحه‌ی اختصاصی شما نمایش داده می‌شود.
