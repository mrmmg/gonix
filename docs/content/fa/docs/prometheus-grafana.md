---
title: Prometheus و Grafana
weight: 105
---

> **آیا می‌توانم گواهی‌ها، backendها و نرخ خطاها را روی داشبورد Grafana ببینم؟**
> بله. `gonix exporter` هرچه گونیکس می‌داند را به‌صورت متریک Prometheus ارائه می‌دهد، و خود گونیکس
> exporter را اجرا و تنظیم می‌کند.

exporter یک سرویس کوچک است (`gonix exporter run`) که به `GET /metrics` پاسخ می‌دهد. unit مربوط به systemd و
تنظیمات آن در اختیار گونیکس است؛ بنابراین تغییر پورت یا فیلتر یک دستور (یا یک صفحه در TUI) است و هرگز لازم
نیست فایل unit را دستی ویرایش کنید.

## روشن کردن

```bash
sudo gonix exporter enable      # unit را نصب می‌کند؛ همین حالا و در هر بوت اجرا می‌شود
sudo gonix exporter status
curl -s http://127.0.0.1:9113/metrics | head
```

در TUI: گزینهٔ **Monitoring** در منوی اصلی. وضعیت اجرا را نشان می‌دهد و امکان enable، disable، start، stop،
restart، تغییر تنظیمات، پیش‌نمایش یک scrape و دیدن unit را می‌دهد.

| دستور | کار |
|---|---|
| `exporter enable` / `disable` | نصب unit و اجرا (الان و در بوت) / توقف و جلوگیری از اجرا در بوت |
| `exporter start` / `stop` / `restart` | کنترل بدون تغییر رفتار بوت |
| `exporter uninstall` | حذف unit (تنظیمات می‌ماند) |
| `exporter set ...` | تغییر تنظیمات و restart در صورت اجرا |
| `exporter status` | وضعیت سرویس و تنظیمات (با `--json` هم) |
| `exporter metrics` | چاپ یک scrape، برای بررسی بدون Prometheus |

## تنظیمات

تنظیمات در بخش `monitoring:` فایل `gonix.yaml` و کنار بقیهٔ تنظیمات گونیکس نگه‌داری می‌شود. به‌جای ویرایش
دستی از `gonix exporter set` (یا TUI) استفاده کنید: اعتبارسنجی، ذخیره، اعمال و restart را انجام می‌دهد.

```bash
sudo gonix exporter set --listen 127.0.0.1:9200          # پورت جدید؛ unit تغییر نمی‌کند
sudo gonix exporter set --hosts '*.example.com' --exclude-hosts 'staging.*'
sudo gonix exporter set --collectors certificates,upstreams,nginx
sudo gonix exporter set --cache-ttl 1m
sudo gonix exporter set --stub-status on                 # شمارندهٔ اتصال‌ها
```

unit هیچ تنظیمی را در خود ندارد و فقط `gonix exporter run --config <file>` را اجرا می‌کند؛ به همین دلیل
پورت جدید نیازی به ویرایش unit ندارد.

### فیلتر در هر scrape

```text
/metrics?host=*.example.com&host=api.example.org   فقط این hostها
/metrics?exclude_host=staging.*                    این‌ها حذف شوند
/metrics?collect[]=certificates&collect[]=nginx    فقط این collectorها
```

سری‌هایی که دربارهٔ یک host نیستند (سرویس Nginx، فضای دیسک) هرگز فیلتر نمی‌شوند.

## متریک‌ها

هر سری برچسب `gonix_instance` دارد. collectorها قابل خاموش و روشن شدن هستند.

| Collector | متریک‌ها |
|---|---|
| `gonix` | `gonix_hosts{state}`، `gonix_host_enabled`، `gonix_host_maintenance`، `gonix_lint_findings{severity}`، `gonix_audit_events_total{action,result}`، `gonix_last_change_timestamp_seconds{result}` |
| `certificates` | `gonix_certificate_remaining_seconds`، `_due_for_renewal`، `_valid`، `_key_matches`، `_chain_valid`، `gonix_certificates{status}` |
| `upstreams` | `gonix_upstream_up`، `gonix_upstream_connect_seconds` (شامل upstream groupها) |
| `nginx` | `gonix_nginx_up`، `_config_valid` (همان `nginx -t`)، `_processes{type}`، `_uptime_seconds`، `_memory_resident_bytes`؛ با stub_status: `gonix_nginx_connections{state}` و شمارنده‌های درخواست و اتصال |
| `traffic` | `gonix_http_requests_total{host,code}`، `gonix_http_response_bytes_total{host}`، `gonix_error_log_entries_total{host,level}` |
| `logfiles` | `gonix_log_size_bytes{host,type}`، `gonix_log_last_write_timestamp_seconds{host}`، `gonix_log_dir_free_bytes` |

درخواست‌های مسدودشده متریک جدا ندارند: همان `code="403"` و `code="429"` هستند.

نکات:

- **شمارنده‌های traffic از صفر شروع می‌شوند** و فقط آنچه از لحظهٔ شروع exporter به لاگ دسترسی اضافه شود را
  می‌شمارند (با پشتیبانی از چرخش لاگ). مثل هر counter دیگر از `rate()` و `increase()` استفاده کنید. در
  پیش‌نمایش (`exporter metrics`) ترافیکی دیده نمی‌شود.
- تأخیر (latency) صادر نمی‌شود، چون فرمت پیش‌فرض لاگ زمان درخواست ندارد.
- `stub_status` به ماژول `http_stub_status_module` نیاز دارد؛ گونیکس یک server block فقط-محلی اضافه می‌کند و
  اگر Nginx آن را نپذیرد، `nginx -t` تغییر را برمی‌گرداند.

## داشبورد Grafana

گونیکس یک داشبورد آماده دقیقاً برای همین متریک‌ها می‌نویسد:

```bash
sudo gonix exporter dashboard --out /var/lib/grafana/dashboards/    # فایل، یا یک پوشه (gonix-dashboard.json)
gonix exporter dashboard > gonix-dashboard.json                      # یا stdout؛ نیازی به root ندارد
```

در TUI: **Monitoring** و سپس **Save Grafana Dashboard** که مسیر را می‌پرسد. بعد از هر ارتقای گونیکس دوباره
اجرایش کنید: نسخهٔ قبلی جایگزین می‌شود و چون داشبورد را همان گونیکسی می‌سازد که متریک‌ها را صادر می‌کند،
این دو همیشه با هم می‌خوانند. لازم نیست دنبال نسخهٔ سازگار بگردید.

آن را در Grafana وارد کنید (Dashboards، New، Import) یا در پوشه‌ای که provisioning پایش می‌کند بگذارید. یک
منبع داده Prometheus می‌خواهد و سه متغیر دارد: **Instance**، **Host** و منبع داده. هفت ردیف دارد:
نمای کلی، ترافیک، گواهی‌ها، Upstreamها، Nginx، لاگ‌ها و دیسک، و خود گونیکس.

داشبورد قانون هشدار ندارد. پنل‌های collectorی که خاموش کرده‌اید خالی می‌مانند و پنل «Connections» به
`stub_status` نیاز دارد. تولیدکنندهٔ داشبورد از همان تعریف‌هایی ساخته می‌شود که exporter استفاده می‌کند
(`internal/metrics/defs.go`) و تست‌ها هر query را با آن‌ها می‌سنجند: وجود متریک و برچسب‌ها، استفادهٔ counterها
فقط با `rate()` یا `increase()`، و تعریف‌شده بودن متغیرها.

## چند instance

هر [instance]({{< relref "instances" >}}) exporter، unit و تنظیمات خودش را دارد:

```bash
sudo gonix --instance staging exporter enable      # unit: gonix-exporter-staging
```

instanceی که آدرس خودش را تعیین نکرده باشد یک پورت پایدار مخصوص خود می‌گیرد (از نامش مشتق می‌شود)، پس دو
exporter روی `9113` با هم تداخل نمی‌کنند. `exporter enable` و `start` آدرس اشغال‌شده را رد می‌کنند و روش
انتخاب آدرس دیگر را می‌گویند.
