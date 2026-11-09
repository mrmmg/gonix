---
title: نصب
weight: 10
---

> **برای امتحان یک ابزار واقعاً باید چیزی کامپایل کنم یا یک repository اضافه کنم؟**
> نه. گونیکس فقط یک فایل اجرایی است و باقی کارها را نصب‌کننده انجام می‌دهد.

## نصب با یک دستور

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
```

نصب‌کننده معماری سیستم را تشخیص می‌دهد، آخرین نسخه را دانلود و نصب می‌کند و یک پیکربندی پیش‌فرض می‌سازد. بعد از آن:

```bash
sudo gonix
```

{{< callout type="info" >}}
**چرا `sudo`؟** گونیکس فایل‌های `/etc/nginx` را ویرایش می‌کند و `systemctl` را صدا می‌زند، و هر دو به root نیاز دارند.
خیالتان راحت باشد: چیزی در پس‌زمینه اجرا نمی‌شود. وقتی از برنامه خارج شوید، گونیکس هم تمام شده است.
{{< /callout >}}

### نصب یک نسخه‌ی مشخص

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash -s -- v1.2.3
```

اگر رمز `sudo` در حافظه نباشد، نصب‌کننده هر وقت بخواهد در `/opt`، `/etc` یا `/usr/local/bin` بنویسد، رمز را از شما می‌پرسد.

### به‌روزرسانی

همان دستور نصب را دوباره اجرا کنید (با شماره‌ی نسخه یا بدون آن). فایل اجرایی جایگزین می‌شود و نصب‌کننده می‌گوید قبلاً
چه نسخه‌ای داشتید. به `/etc/gonix` دست نمی‌زند؛ پس پیکربندی، پشتیبان‌ها و فایل‌های رمز شما همیشه سالم می‌مانند.

یا از خود GoNix به‌روز کنید: `sudo gonix update` آخرین نسخه را از GitHub می‌خواند، checksum از نوع SHA-256 را بررسی
می‌کند، نسخه‌ی فعلی را با نام `gonix.old` نگه می‌دارد و فایل اجرایی را عوض می‌کند (`--rollback` نسخه‌ی قبلی را
برمی‌گرداند و `gonix update --check` فقط بررسی می‌کند). منوی **Updates** در TUI می‌گوید نسخه‌ی جدیدی هست و همین دستور را
نشان می‌دهد. GoNix فقط از Releaseهای GitHub همین پروژه به‌روز می‌شود.

## چه چیزی کجا نصب می‌شود؟

| مسیر                       | کاربرد                                                          |
|----------------------------|-----------------------------------------------------------------|
| `/opt/gonix/bin/gonix`     | فایل اجرایی                                                     |
| `/opt/gonix/VERSION`       | نسخه‌ی نصب‌شده                                                  |
| `/usr/local/bin/gonix`     | پیوند (symlink) به فایل اجرایی، تا `gonix` از هر جا کار کند     |
| `/etc/gonix/gonix.yaml`    | فایل پیکربندی (فقط یک بار ساخته می‌شود و هرگز بازنویسی نمی‌شود) |
| `/etc/gonix/backups/`      | پشتیبان‌های خودکار                                              |
| `/etc/gonix/accesslists/`  | فایل‌های کاربر و رمز عبور (HTTP Basic Auth)                     |
| `/var/log/gonix/audit.log` | لاگ تغییرات (audit log)                                         |
| `/etc/logrotate.d/gonix`   | سیاست چرخش لاگ (فقط اگر `logrotate` نصب باشد)                   |

## معماری‌های پشتیبانی‌شده

| سیستم‌عامل | معماری | نام فایل در نسخه‌ی منتشرشده |
|------------|--------|-----------------------------|
| Linux      | amd64  | `gonix-linux-amd64`         |
| Linux      | arm64  | `gonix-linux-arm64`         |

## پیش‌نیازها

- لینوکس با systemd (گونیکس از `systemctl` و `/proc` استفاده می‌کند) روی **amd64** یا **arm64**. روی هر معماری دیگری
  نصب‌کننده با یک پیام روشن متوقف می‌شود، به‌جای اینکه فایلی دانلود کند که اجرا نمی‌شود.
- Nginx.
- موارد اختیاری، که هر کدام فقط برای یک قابلیت لازم‌اند: `logrotate` (چرخش لاگ)، `git` ([تاریخچه‌ی پیکربندی]({{< relref "backups-history" >}}))،
  `certbot` ([گواهی Wildcard]({{< relref "ssl" >}}))، ماژول `geoip2` (بلاک کردن کشورها) و ماژول `stream` (پروکسی TCP/UDP).

{{< callout type="warning" >}}
گونیکس بعضی فایل‌ها را در `/etc/nginx/conf.d` می‌نویسد، پس `nginx.conf` باید این پوشه را داخل بلاک `http` include کند.
در Debian، Ubuntu و RHEL این کار به‌صورت پیش‌فرض انجام شده است. اگر نشده باشد،
[Config Check]({{< relref "logs-and-monitoring" >}}) به شما هشدار می‌دهد.
{{< /callout >}}

## دانلود دستی و بررسی صحت فایل

اگر نمی‌خواهید یک اسکریپت را مستقیم به `bash` بدهید، هر [نسخه‌ی منتشرشده](https://github.com/mrmmg/gonix/releases) فایل‌های اجرایی،
`checksums.txt` و `gonix.yaml` پیش‌فرض را دارد:

```bash
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/gonix-linux-amd64
curl -fsSLO https://github.com/mrmmg/gonix/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

اگر معماری شما arm64 است، `amd64` را با `arm64` عوض کنید. نصب‌کننده دقیقاً همین کار را می‌کند و علاوه بر آن پوشه‌های جدول بالا
را هم می‌سازد؛ برای همین، مسیر پیشنهادی ما همان نصب با یک دستور است.

## نصب از سورس

در صفحه‌ی [مشارکت در پروژه]({{< relref "contributing" >}}) توضیح داده‌ایم: `make install` با `go build` برنامه را می‌سازد و
در همین ساختار نصب می‌کند.
