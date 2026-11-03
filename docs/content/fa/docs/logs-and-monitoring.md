---
title: لاگ و پایش
weight: 100
---

> **کاربری می‌گوید «سایت کند است». اول کجا را نگاه می‌کنید؟**
> قطعاً نه داخل یک فایل لاگ ۲ گیگابایتی با `grep`. گونیکس سه نوع اطلاعات را نگه می‌دارد که هرکدام جواب یک سؤال را می‌دهند.

| منبع | جواب می‌دهد | کجاست |
|---|---|---|
| لاگ دسترسی و خطا | چه درخواست‌هایی به این هاست می‌رسد؟ | `/var/log/nginx/<domain>.access.log` و `<domain>.error.log` |
| لاگ تغییرات (audit log) | چه کسی چه چیزی را عوض کرد و موفق شد؟ | `/var/log/gonix/audit.log` |
| Config Check | همین الان پیکربندی مشکلی دارد؟ | هر وقت اجرا کنید |

## لاگ دسترسی و خطا

این دو لاگ برای هر هاست جداگانه‌اند و هرکدام را از صفحه‌ی هاست می‌توانید مستقل روشن یا خاموش کنید (در CLI: `--access-log on|off` و
`--error-log on|off`). نصب‌کننده `logrotate` را هم در `/etc/logrotate.d/gonix` تنظیم می‌کند.

### لاگ زنده

```bash
sudo gonix logs app.example.com --lines 50
sudo gonix logs app.example.com --filter "status:5xx" --summary
sudo gonix logs app.example.com --error
```

فیلترها با هم ترکیب می‌شوند: `status:5xx`، `ip:203.0.113.9` و `path:/login`. گزینه‌ی `--summary` پرتکرارترین IPها، مسیرها و statusها را
نشان می‌دهد؛ معمولاً جواب این سؤال که «چه کسی دارد فشار می‌آورد؟». در TUI، نمای زنده درخواست‌ها را همان لحظه‌ای که می‌رسند نشان
می‌دهد و می‌توانید یک IP مزاحم را **مستقیم از همان لاگ بلاک کنید**.

{{< callout type="info" >}}
اگر سایتتان پشت پروکسی است، اول [IP واقعی کاربر]({{< relref "access-control" >}}) را روشن کنید؛ وگرنه در همه‌ی خط‌های لاگ، آدرس خود پروکسی را می‌بینید.
{{< /callout >}}

## لاگ تغییرات (audit log)

برای هر کاری که انجام شود یک خط:

```text
2026-09-15 10:32:11 | user=root | action=host_created | target=example.com | result=success
```

تغییراتی که از CLI می‌آیند `| actor=cli` اضافه می‌کنند (یا مقدار `GONIX_ACTOR`)، و سرور MCP از `actor=mcp` استفاده می‌کند.
هر [نمونه‌ی اضافه‌ی Nginx]({{< relref "instances" >}}) هم لاگ تغییرات خودش را در `/var/log/gonix/<instance>-audit.log` دارد.

```bash
sudo gonix audit --lines 20
```

اگر تغییری شکست بخورد، خطای Nginx هم همراهش ثبت می‌شود؛ پس همیشه می‌توانید بفهمید «چرا این تغییر برگردانده شد؟».

## Config Check

> **مشکل از گواهی است، یا server name تکراری، یا یک backend که مرده؟**
> Config Check همه‌ی این‌ها را یک‌جا بررسی می‌کند.

```bash
sudo gonix check
sudo gonix check --no-upstreams --json
```

این موارد را پیدا می‌کند: server nameهای تکراری، فایل‌های گم‌شده، گواهی‌هایی که نزدیک انقضا یا ناهماهنگ‌اند، upstreamهایی که در
دسترس نیستند، snippetهای بی‌استفاده، و نبودن include مربوط به `nginx.http_snippets_dir`. اگر نمی‌خواهید بررسی‌های شبکه انجام شود، `--no-upstreams` را بزنید.

## سرویس Nginx و پایش

از TUI می‌توانید Nginx را start، stop، restart، reload و تست کنید. پردازه‌های master و worker، مصرف حافظه، uptime و پورت‌هایی که
Nginx روی آن‌ها گوش می‌دهد هم مستقیم از `/proc` خوانده و نمایش داده می‌شوند. از CLI:

```bash
sudo gonix nginx status
sudo gonix nginx test
sudo gonix nginx reload
```
