---
title: مستندات
weight: 1
---

> **تا حالا شده بترسید `systemctl reload nginx` را بزنید؟**
> همان لحظه‌ی تردید: «نکند یک سمی‌کالن جا مانده باشد؟ مسیر گواهی درست بود؟ نکند همه‌ی سایت‌های سرور از کار بیفتند؟»
> گونیکس را ساخته‌ایم تا این ترس از بین برود.

این مستندات را برای خواندن نوشته‌ایم، نه فقط برای جستجو. هر صفحه با سؤالی شروع می‌شود که احتمالاً خودتان هم دارید،
بعد مشکل را نشان می‌دهد و در پایان توضیح می‌دهد گونیکس چطور آن را حل می‌کند. برای هر تصمیم طراحی هم دلیلش را
می‌گوییم تا فقط یاد نگیرید «چه کاری»، بلکه بفهمید «چرا».

## گونیکس چیست؟

اسم گونیکس از **Go** + **Nginx** می‌آید: یک رابط ترمینالی (TUI) پیشرفته و تعاملی برای مدیریت Nginx روی لینوکس که با Go نوشته شده.
فایل اجرایی آن `gonix` نام دارد.

ایده‌اش را از [Nginx Proxy Manager](https://nginxproxymanager.com/) گرفته‌ایم، ولی مسیرمان فرق می‌کند. به‌جای اینکه Nginx را پشت یک دیتابیس
و یک فرمت اختصاصی پنهان کنیم، گونیکس لایه‌ای نازک و امن است که **روی** همان فایل‌های واقعی و قابل ویرایش Nginx می‌نشیند. همیشه می‌توانید
`/etc/nginx/sites-available/example.com` را با یک ویرایشگر متن باز کنید و همان سینتکس ساده و استاندارد Nginx را ببینید؛ هیچ چیز مبهم یا اختصاصی در کار نیست.

{{< preview >}}

می‌خواهید بدانید گونیکس دقیقاً چه کارهایی می‌کند؟ به صفحه‌ی [امکانات]({{< relref "features" >}}) بروید.

## از کجا شروع کنم؟

{{< cards >}}
  {{< card link="features" title="امکانات" subtitle="همه‌ی کارهایی که گونیکس می‌کند، در یک فهرست." icon="sparkles" >}}
  {{< card link="install" title="نصب" subtitle="یک دستور، و اینکه چه چیزی کجا نصب می‌شود." icon="download" >}}
  {{< card link="quickstart" title="شروع سریع" subtitle="اولین سایت HTTPS شما، در پنج دقیقه." icon="play" >}}
  {{< card link="how-it-works" title="گونیکس چطور کار می‌کند" subtitle="سه ایده‌ی اصلی. فقط یک بار بخوانید." icon="light-bulb" >}}
  {{< card link="hosts" title="هاست‌ها" subtitle="پروکسی، سایت استاتیک، ریدایرکت و WebSocket." icon="server" >}}
  {{< card link="ssl" title="گواهی SSL" subtitle="ساختار پوشه‌ها، بررسی گواهی و گواهی Wildcard." icon="lock-closed" >}}
  {{< card link="access-control" title="کنترل دسترسی" subtitle="رمز عبور، لیست IP، کشورها و IP واقعی کاربر." icon="shield-check" >}}
  {{< card link="traffic" title="مدیریت ترافیک" subtitle="Presetها، محدودیت نرخ و حالت تعمیر." icon="adjustments" >}}
  {{< card link="error-pages" title="صفحه‌های خطا" subtitle="به‌جای یک 502 خشک و خاکستری، صفحه‌ی خودتان." icon="exclamation-circle" >}}
  {{< card link="load-balancing" title="توزیع بار و Stream" subtitle="گروه‌های upstream و پروکسی TCP/UDP." icon="switch-horizontal" >}}
  {{< card link="logs-and-monitoring" title="لاگ و پایش" subtitle="لاگ زنده، لاگ تغییرات و Config Check." icon="chart-bar" >}}
  {{< card link="prometheus-grafana" title="Prometheus و Grafana" subtitle="exporter، متریک‌ها و داشبورد آماده." icon="chart-pie" >}}
  {{< card link="backups-history" title="پشتیبان و تاریخچه" subtitle="هر تغییری را می‌شود برگرداند." icon="clock" >}}
  {{< card link="migration" title="جابه‌جایی سرور" subtitle="رفتن به سرور جدید، یا خداحافظی با Nginx Proxy Manager." icon="truck" >}}
  {{< card link="cli" title="خط فرمان" subtitle="اسکریپت، CI و مرجع کامل دستورها." icon="terminal" >}}
  {{< card link="ai-agents" title="عامل‌های هوش مصنوعی" subtitle="Skill و سرور MCP برای Claude Code و ابزارهای مشابه." icon="chip" >}}
  {{< card link="instances" title="چند نمونه‌ی Nginx" subtitle="چند Nginx، یک گونیکس." icon="collection" >}}
  {{< card link="configuration" title="مرجع پیکربندی" subtitle="همه‌ی کلیدهای gonix.yaml." icon="cog" >}}
  {{< card link="troubleshooting" title="عیب‌یابی" subtitle="علامت، علت و راه‌حل." icon="exclamation" >}}
  {{< card link="contributing" title="مشارکت در پروژه" subtitle="ساخت از سورس، اجرای تست‌ها و ساختار پروژه." icon="code" >}}
  {{< card link="roadmap" title="نقشه‌ی راه" subtitle="چه چیزهایی را عمداً هنوز نساخته‌ایم." icon="map" >}}
{{< /cards >}}

## مثال‌ها را چطور بخوانید

- خط‌هایی که با `sudo` شروع می‌شوند، همان چیزی‌اند که خودتان در ترمینال تایپ می‌کنید. تقریباً همه‌ی کارها به root نیاز دارند.
- بلاک‌های `nginx` همان پیکربندی‌ای را نشان می‌دهند که گونیکس برایتان می‌نویسد؛ تا ببینید هیچ جادویی در کار نیست.
- هر دستوری که چیزی را تغییر می‌دهد `--dry-run` دارد. اگر مطمئن نیستید، آن را اضافه کنید و اول تفاوت‌ها (diff) را ببینید.
