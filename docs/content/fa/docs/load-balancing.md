---
title: توزیع بار و Stream
weight: 90
---

> **یک backend یعنی یک نقطه‌ی شکست. و هر چیزی هم که می‌خواهید پروکسی کنید، HTTP نیست.**
> گروه‌های upstream بار HTTP را بین چند سرور پخش می‌کنند و پروکسی‌های stream، ترافیک خام TCP/UDP را عبور می‌دهند.

## گروه‌های Upstream

«گروه upstream» مجموعه‌ای نام‌دار از backendهاست. هاست به‌جای یک URL، ترافیک را به این گروه می‌فرستد.

```bash
sudo gonix upstream create app \
  --servers "10.0.0.11:3000 weight=3, 10.0.0.12:3000, 10.0.0.13:3000 backup" \
  --method least_conn
sudo gonix host add example.com --group app
```

| `--method` | رفتار | چه زمانی انتخابش کنید |
|---|---|---|
| `round_robin` (پیش‌فرض) | نوبتی | هزینه‌ی درخواست‌ها تقریباً یکسان است |
| `least_conn` | سروری که کمترین اتصال فعال را دارد | مدت پردازش درخواست‌ها خیلی متفاوت است |
| `ip_hash` | هر کلاینت همیشه به یک backend می‌رسد | یک «چسبندگی» ساده لازم دارید |

- **وزن** (`weight=3`) باعث می‌شود سرورهای قوی‌تر سهم بیشتری از ترافیک بگیرند.
- **سرور پشتیبان** (`backup`) فقط وقتی ترافیک می‌گیرد که بقیه‌ی سرورها از کار افتاده باشند.
- **بررسی سلامت منفعل (passive health check)**: سروری که خطا بدهد، برای مدتی از چرخه کنار گذاشته می‌شود. Nginx متن‌باز بیشتر از این
  امکانی ندارد؛ بررسی سلامت فعال در [نقشه‌ی راه]({{< relref "roadmap" >}}) هست.

دستور `gonix check` هم در دسترس بودن upstreamها را آزمایش می‌کند.

## پروکسی Stream (برای TCP و UDP)

> **چطور یک دیتابیس، سرور بازی یا یک jump host برای SSH را پشت Nginx بگذارید؟**
> با ماژول `stream`؛ این ماژول فقط ترافیک خام TCP/UDP را منتقل می‌کند و ربطی به HTTP ندارد.

```bash
sudo gonix stream setup
sudo gonix stream create pg --port 5432 --backend 10.0.0.20:5432 --ip-list office
sudo gonix stream create dns --port 53 --backend 10.0.0.2:53 --udp
sudo gonix stream list
```

- اگر `nginx.conf` بلاک `stream { }` نداشته باشد، `stream setup` آن را در `nginx.main_include_dir` می‌سازد.
- با `--ip-list` مشخص می‌کنید چه کسانی اجازه‌ی اتصال دارند؛ همان [لیست‌های IP]({{< relref "access-control" >}}) که قبلاً دیدید.
- فایل‌های stream در `nginx.streams_dir` نوشته می‌شوند.

برای این قابلیت ماژول stream لازم است (در Debian و Ubuntu: `libnginx-mod-stream`).

{{< callout type="warning" >}}
**باز گذاشتن پورت یک دیتابیس روی اینترنت یک تصمیم آگاهانه است، نه حالت پیش‌فرض.** برای هر streamی که چیز حساسی را عبور می‌دهد، لیست IP بگذارید.
{{< /callout >}}
