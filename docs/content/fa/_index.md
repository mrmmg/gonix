---
title: گونیکس
layout: hextra-home
---

{{< logo >}}

{{< hextra/hero-badge link="https://github.com/mrmmg/gonix/releases/latest" >}}
  <span>رایگان، متن‌باز، با مجوز MIT</span>
  {{< icon name="arrow-circle-left" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Nginx را از ترمینال مدیریت کنید.&nbsp;<br class="hx:sm:block hx:hidden" />بی‌آنکه چیزی از شما پنهان شود.
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  گونیکس یک لایه‌ی امن و تعاملی است که روی خود فایل‌های Nginx شما می‌نشیند.&nbsp;<br class="hx:sm:block hx:hidden" />هر کدام را با ویرایشگر باز کنید، همان Nginx ساده و آشناست.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="شروع کنید" link="docs/quickstart" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="هیچ‌چیز پنهان نیست"
    subtitle="نه دیتابیسی در کار است و نه فرمت اختصاصی. هر هاست یک فایل معمولی در sites-available است و اسمش همان دامنه‌اش."
  >}}
  {{< hextra/feature-card
    title="Nginx هرگز خراب نمی‌ماند"
    subtitle="قبل از هر تغییر نسخه‌ی پشتیبان گرفته می‌شود، با nginx -t تست می‌شود و اگر تست رد شود، همه‌چیز خودکار به حالت قبل برمی‌گردد."
  >}}
  {{< hextra/feature-card
    title="TUI، CLI و عامل‌های هوش مصنوعی"
    subtitle="هر کاری را هم از منوی ترمینال می‌شود انجام داد، هم با دستور (با --json و --dry-run برای اسکریپت‌ها)، هم از طریق سرور MCP."
  >}}
  {{< hextra/feature-card
    title="هرچه لازم دارید"
    subtitle="ریورس‌پروکسی، ریدایرکت، SSL و گواهی Wildcard، رمز و لیست IP، محدودیت نرخ، توزیع بار، پروکسی TCP/UDP و لاگ زنده."
  >}}
  {{< hextra/feature-card
    title="ماشین زمان هم دارد"
    subtitle="هر تغییر موفق یک کامیت Git است (بیرون از /etc/nginx). تفاوت‌ها را ببینید و به هر نقطه‌ای که خواستید برگردید."
  >}}
  {{< hextra/feature-card
    title="یک باینری ایستا"
    subtitle="با یک دستور نصب می‌شود (linux/amd64 و linux/arm64). نه runtime می‌خواهد، نه daemon؛ چیزی در پس‌زمینه اجرا نمی‌ماند."
  >}}
{{< /hextra/feature-grid >}}
