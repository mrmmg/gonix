---
title: چند نمونه‌ی Nginx
weight: 160
---

> **اگر یک سرور هم Nginx پروداکشن داشته باشد، هم staging، هم یک نسخه‌ی سفارشی چه؟**
> یک گونیکس می‌تواند همه را مدیریت کند. هر Nginx یک «نمونه» (instance) است.

هر Nginx اضافه، فایل پیکربندی خودش را در `/etc/gonix/instances/` دارد:

```bash
sudo gonix instances                              # فهرست نمونه‌ها
sudo gonix --instance staging                     # TUI برای /etc/gonix/instances/staging.yaml
sudo gonix --instance staging host list           # یا اجرای یک دستور روی همان نمونه
```

متغیرهای `GONIX_INSTANCE` و `GONIX_CONFIG` هم دقیقاً مثل پرچم‌های متناظرشان کار می‌کنند.

## چه چیزهایی خودکار جدا می‌شوند؟

اگر مسیرهایی مثل پشتیبان‌ها، تاریخچه، audit log و فایل‌های رمز را تغییر نداده باشید، برای هر نمونه **خودکار جدا** می‌شوند. پس staging
هرگز در پشتیبان‌ها یا تاریخچه‌ی پروداکشن نمی‌نویسد. مثلاً هر نمونه audit log خودش را در `/var/log/gonix/<instance>-audit.log` دارد.

## اشاره به یک Nginx دیگر

برای Nginx‌ای که همان Nginx اصلی سیستم نیست، این کلیدها را در فایل نمونه تنظیم کنید:

```yaml
nginx:
  config_dir: /opt/staging-nginx/etc
  config_file: /opt/staging-nginx/etc/nginx.conf
  service: nginx-staging
```

مقدار `nginx.config_file` به `nginx -t -c` داده می‌شود و `nginx.service` نام سرویس systemd است که بعد از هر تغییر reload می‌شود.
برای فهرست کامل کلیدها به [مرجع پیکربندی]({{< relref "configuration" >}}) بروید.
