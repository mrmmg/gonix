---
title: دستورهای آماده
weight: 2
---

جواب‌های کوتاه و کامل برای سؤال‌های «چطور می‌شود ...». به هرکدام که خواستید `--dry-run` اضافه کنید تا اول پیش‌نمایشش را ببینید.

## یک برنامه را با HTTPS بالا بیاورید

```bash
sudo gonix host add app.example.com --upstream http://127.0.0.1:3000 \
  --ssl app.example.com --presets security-headers,hsts --websocket
```

## یک کلاینت مزاحم را بلاک کنید

> **مشکل:** یک آدرس دارد به سایتتان فشار می‌آورد.

```bash
sudo gonix logs app.example.com --summary          # مزاحم اصلی را پیدا کنید
sudo gonix iplist create blocked --mode block --entries 192.0.2.77
sudo gonix host set app.example.com --ip-list blocked
sudo gonix iplist add blocked 192.0.2.78           # بعداً هر چقدر خواستید اضافه کنید
```

اگر سایت پشت کلودفلر است، اول `sudo gonix realip cloudflare` را بزنید تا لاگ‌ها بازدیدکننده‌های واقعی را نشان بدهند.

## `/admin/` را فقط برای دفتر باز کنید

```bash
sudo gonix iplist create office --mode allow --entries "203.0.113.0/24"
sudo gonix host path add app.example.com /admin/ \
  --upstream http://127.0.0.1:4000 --ip-list office
```

## یک دامنه را منتقل کنید و لینک‌های قدیمی را زنده نگه دارید

```bash
sudo gonix host add old.example.com --alias www.old.example.com \
  --redirect https://new.example.com --ssl old.example.com
```

مسیر به‌صورت پیش‌فرض حفظ می‌شود؛ پس `old.example.com/pricing` به `new.example.com/pricing` می‌رود.

## یک پنجره‌ی تعمیر

```bash
sudo gonix host set app.example.com --maintenance on --bypass "203.0.113.5"
# deploy ...
sudo gonix host set app.example.com --maintenance off
```

## جلوی brute force روی فرم ورود را بگیرید

```bash
sudo gonix ratelimit create login --preset login
sudo gonix host path add app.example.com /login \
  --upstream http://127.0.0.1:3000 --rate-limit login
```

## یک چیزی خراب شد. حالا چه کنم؟

```bash
sudo gonix check
sudo gonix logs app.example.com --error --lines 50
sudo gonix history list --lines 10
sudo gonix history restore <commit> --yes
```

## در اسکریپت استفاده‌اش کنید

از `--json` استفاده کنید و کد خروج را بررسی کنید:

```bash
if ! out=$(sudo gonix host add "$domain" --upstream "$url" --json); then
  echo "gonix failed: $out" >&2
  exit 1
fi
```

تغییرهای خودکار را برچسب بزنید تا در audit log بشود آن‌ها را از تغییرهای دستی تشخیص داد:

```bash
sudo GONIX_ACTOR=deploy-bot gonix host set app.example.com --max-body 50m
```
