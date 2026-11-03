---
title: هاست‌ها
weight: 40
---

> **هاست «فقط» یک بلاک `server {}` است؛ پس چرا دستی ننویسمش؟**
> می‌توانید، و گونیکس هم نتیجه را کامل نشانتان می‌دهد. اما آن ۴۰ خطی که یک ریورس‌پروکسی درست را می‌سازند
> (هدرهای forward، ارتقای WebSocket، timeoutها، ریدایرکت به HTTPS) هر بار همان‌هایند، و یک غلط تایپی در هرکدام
> سایت را از کار می‌اندازد. گونیکس این خط‌ها را یک بار و درست می‌نویسد.

## ریورس‌پروکسی

رایج‌ترین نوع هاست: فرستادن یک دامنه به یک برنامه.

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000
```

در TUI، گزینه‌ی **Add New Host** همین گزینه‌ها را یکی‌یکی می‌پرسد: آدرس و پورت upstream، پروتکل، نسخه‌ی HTTP،
timeoutها، حداکثر حجم body و هدرهای forward.

| گزینه | پرچم | چه زمانی عوضش کنید |
|---|---|---|
| timeout اتصال، خواندن و ارسال | `--connect-timeout`، `--read-timeout`، `--send-timeout` | گزارش‌های سنگین، long polling |
| حداکثر حجم body | `--max-body 10m` | آپلود فایل به‌صورت پیش‌فرض با خطای `413` رد می‌شود |
| هدرهای forward | پیش‌فرض روشن است؛ با `--no-forward-headers` خاموش می‌شود | برنامه‌تان می‌خواهد IP و scheme بازدیدکننده را ببیند |

وقتی forward روشن باشد، گونیکس `Host`، `X-Real-IP`، `X-Forwarded-For` و `X-Forwarded-Proto` را می‌فرستد. برنامه‌ی شما
هم بازدیدکننده‌ی واقعی را می‌بیند، نه `127.0.0.1` را.

## WebSocket، فقط با یک کلید

> **چرا برنامه‌ی چت من وصل می‌شود و چند لحظه بعد قطع؟**
> چون Nginx تا به آن نگویید، درخواست ارتقای پروتکل را رد نمی‌کند.

بدون هدرهای درست، handshake مربوط به WebSocket شکست می‌خورد. با گونیکس فقط یک کلید را روشن می‌کنید:

```bash
sudo gonix host set app.example.com --websocket on
```

گونیکس پشت صحنه `proxy_set_header Upgrade $http_upgrade;` و `proxy_set_header Connection "upgrade";` را اضافه می‌کند و
`proxy_http_version 1.1` را برای ارتباط با upstream تنظیم می‌کند. دیگر لازم نیست خودتان آن‌ها را بنویسید.

## سایت استاتیک

```bash
sudo gonix host add docs.example.com --root /var/www/docs
```

فایل‌ها را از همان پوشه سرو می‌کند و `index.html` را صفحه‌ی اصلی می‌گیرد.

## هاست ریدایرکت

> **وقتی برندتان از `example.net` به `example.com` رفت، با `example.net` چه کنید؟**

```bash
sudo gonix host add example.net --redirect https://example.com --code 301
```

- مقدار `--code` یکی از `301`، `302`، `307` و `308` است. برای انتقال دائمی `301` یا `308` و برای موقت `302` یا `307`
  بگذارید. `307` و `308` متد HTTP را حفظ می‌کنند، که برای درخواست‌های `POST` مهم است.
- به‌طور پیش‌فرض مسیر و query هم منتقل می‌شوند (`example.net/pricing?x=1` به `example.com/pricing?x=1` می‌رود).
  اگر می‌خواهید همه به صفحه‌ی اول مقصد بروند، `--no-preserve-path` را اضافه کنید.

## چند نام برای یک هاست

```bash
sudo gonix host add example.com --upstream http://127.0.0.1:3000 \
  --alias www.example.com
```

می‌توانید `--alias` را چند بار تکرار کنید. بعداً هم با `host set example.com --aliases "www.example.com blog.example.com"`
فهرست را عوض کنید (یا با `--aliases none` همه را پاک کنید).

## locationهای اضافه

یک دامنه، چند backend:

```bash
sudo gonix host path add example.com /api/ --upstream http://127.0.0.1:4000 --websocket
sudo gonix host path remove example.com /api/
```

هر location می‌تواند لیست IP و محدودیت نرخ مخصوص خودش را داشته باشد (`--ip-list` و `--rate-limit`). برای هر چیزی که
گونیکس تنظیمی برایش ندارد، TUI یک **location سفارشی** دارد که در آن دستورهای خام Nginx را خودتان می‌نویسید.

## HTTPS، HTTP/2 و HTTP/3

با دادن یک گواهی، هاست روی پورت 443 هم گوش می‌دهد:

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 --ssl app.example.com --http3
```

- ریدایرکت HTTP به HTTPS به‌صورت پیش‌فرض روشن است (با `--force-https=false` خاموش می‌شود).
- HTTP/2 هم به‌صورت پیش‌فرض روشن است، و گونیکس سینتکس درست را بر اساس نسخه‌ی Nginx شما می‌نویسد.
- **HTTP/3 (QUIC)** فقط وقتی پیشنهاد می‌شود که Nginx شما از آن پشتیبانی کند؛ پس نمی‌توانید چیزی را فعال کنید که
  `nginx -t` را رد کند.

{{< callout type="info" >}}
**چرا به گواهی «ارجاع» می‌دهیم و آن را کپی نمی‌کنیم؟** در صفحه‌ی [گواهی SSL]({{< relref "ssl" >}}) توضیح داده‌ایم.
نتیجه‌اش این است که تمدید گواهی یعنی فقط جایگزین کردن دو فایل.
{{< /callout >}}

## فعال، غیرفعال، حذف

```bash
sudo gonix host disable app.example.com   # فقط symlink پاک می‌شود؛ فایل می‌ماند
sudo gonix host enable app.example.com
sudo gonix host delete app.example.com --yes   # یک نسخه‌ی پشتیبان باقی می‌ماند
```

حذف چون برگشت‌ناپذیر به نظر می‌رسد، `--yes` می‌خواهد. با این حال یک نسخه‌ی پشتیبان از هاست باقی می‌ماند.

## تغییر دادن یک هاست

`host set` همان پرچم‌های `host add` را می‌گیرد، به‌علاوه‌ی کلیدهایی مثل `--access-log on|off`:

```bash
sudo gonix host set app.example.com --max-body 50m --read-timeout 120s --dry-run
```

برای جدا کردن یک مورد، مقدار `none` بدهید (مثلاً `--ssl none` یا `--ip-list none`).

بیشتر بخوانید: [کنترل دسترسی]({{< relref "access-control" >}}) و [مدیریت ترافیک]({{< relref "traffic" >}}).
