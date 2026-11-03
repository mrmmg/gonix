---
title: صفحه‌های خطا
weight: 80
---

> **backend ریستارت می‌شود و بازدیدکننده یک «502 Bad Gateway» خاکستری و خشک می‌بیند. بهتر از این نمی‌شود؟**
> کاملاً می‌شود.

«snippet صفحه‌ی خطا» یک تکه پیکربندی نام‌دار و قابل استفاده در چند جاست که به کدهای خطای HTTP انتخابی، با صفحه‌ی HTML خودتان جواب می‌دهد.
آن‌ها را از **Main Menu ← Error Pages** بسازید و ویرایش کنید؛ پوشه‌ی پیشنهادی همان `error_pages.pages_directory` است
(`/var/www/html/error_pages`). هر snippet **یک** پوشه‌ی صفحه‌ها دارد و در یکی از دو حالت کار می‌کند.

## حالت اول: یک صفحه برای هر کد

برای هر کد یک فایل با نام `<code>.html` (مثلاً `502.html`). snippet‌ای به نام `default` که 502 و 504 را پوشش بدهد، در
`/etc/nginx/snippets/gonix-error-pages-default.conf` این‌طور نوشته می‌شود:

```nginx
error_page 502 /__gonix_error_pages/502.html;
error_page 504 /__gonix_error_pages/504.html;

location ^~ /__gonix_error_pages/ {
    internal;
    auth_basic off;
    alias /var/www/html/error_pages/;
}
```

## حالت دوم: یک template مشترک

یک فایل مشترک (مثلاً `error.html`) که هر جای آن یک placeholder داشته باشد (پیش‌فرض `ERROR_CODE_PLACEHOLDER`)؛ گونیکس آن را
با کد وضعیت جایگزین می‌کند. snippet‌ای به نام `branded` در `/etc/nginx/snippets/gonix-error-pages-branded.conf` نوشته می‌شود:

```nginx
error_page 400 401 403 404 413 429 500 502 503 504 /__gonix_error_page;

location = /__gonix_error_page {
    internal;
    auth_basic off;
    root /var/www/html/error_pages;
    try_files /error.html =500;
    types { }
    default_type text/html;
    sub_filter 'ERROR_CODE_PLACEHOLDER' $status;
    sub_filter_once off;
}
```

متغیر `$status` همان کد خطای اصلی را دارد؛ پس یک location برای همه‌ی کدها کافی است و پاسخ هم همان status را حفظ می‌کند (یعنی
هنوز یک 502 واقعی است). حالت template به Nginx‌ای نیاز دارد که با `ngx_http_sub_module` ساخته شده باشد:

```bash
nginx -V 2>&1 | grep -o http_sub_module
```

بدون این ماژول `nginx -t` شکست می‌خورد و گونیکس تغییر را برمی‌گرداند. اگر فایل template هم placeholder نداشته باشد، گونیکس هشدار می‌دهد.

## استفاده از یک snippet

آن را موقع **Add New Host** انتخاب کنید یا از منوی **Error Pages** خودِ هاست (در CLI: `--error-pages NAME`). فقط یک خط به هاست اضافه می‌شود:

```nginx
include /etc/nginx/snippets/gonix-error-pages-default.conf;
```

## چند نکته که باید بدانید

- اگر کدها یا پوشه‌ی یک snippet را عوض کنید، همه‌ی هاست‌هایی که آن را include کرده‌اند به‌روز می‌شوند. مثل هر تغییر دیگر با
  `nginx -t` آزموده می‌شود و در صورت شکست برمی‌گردد.
- snippet‌ای که هنوز استفاده می‌شود، حذف نمی‌شود.
- `auth_basic off` باعث می‌شود صفحه‌های خطا روی هاست‌هایی که رمز دارند هم نمایش داده شوند.
- صفحه‌ها از یک location داخلی سرو می‌شوند، پس کسی نمی‌تواند مستقیم آن‌ها را درخواست کند. فایل‌های وابسته را داخل خود صفحه بگذارید:
  CSS داخل `<style>` و تصویرها به‌صورت `data:` URI.
- فقط خطاهایی جایگزین می‌شوند که **خود Nginx** برمی‌گرداند؛ مثل 502 یا 504 وقتی upstream خاموش یا کند است. خطایی که برنامه‌ی
  شما بسازد، بدون تغییر رد می‌شود.
- اگر فایل یک صفحه وجود نداشته باشد، پیکربندی خراب نمی‌شود؛ گونیکس فقط هشدار می‌دهد و وضعیت هر صفحه را نشان می‌دهد.
- هاست **غیرفعال** اصلاً بلاک server ندارد؛ پس درخواست‌هایش به server پیش‌فرض Nginx می‌رسد، نه به صفحه‌های خطای خودش.
