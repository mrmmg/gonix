---
title: نقشه‌ی راه
weight: 200
---

> **گونیکس چه چیزهایی کم دارد، و آیا عمدی است؟**
> بیشترشان بله. برای این موارد جا گذاشته‌ایم، اما عمداً هنوز آن‌ها را نساخته‌ایم.

- **ارائه‌دهنده‌های DNS بیشتر برای گواهی Wildcard.** فعلاً فقط کلودفلر پشتیبانی می‌شود.
- **یکپارچگی با WAF / ModSecurity.**
- **بررسی سلامت فعال (active health check).** Nginx متن‌باز فقط نوع منفعل را دارد.
- **وارد کردن پیکربندی دست‌نویس دلخواه** به هاست‌های managed. فعلاً آن هاست‌ها فقط‌خواندنی و `[unmanaged]` می‌مانند.

## عمداً کنار گذاشته شده

**OCSP stapling.** Let's Encrypt از سال ۲۰۲۵ سرورهای OCSP را خاموش کرده، پس چیزی برای staple کردن وجود ندارد.

## ایده‌ای دارید؟

در [github.com/mrmmg/gonix/issues](https://github.com/mrmmg/gonix/issues) یک issue باز کنید. فهرست تغییرات نسخه‌ها را هم
در [changelog](https://github.com/mrmmg/gonix/blob/main/CHANGELOG.md) ببینید.
