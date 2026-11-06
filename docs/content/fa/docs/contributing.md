---
title: مشارکت در پروژه
weight: 190
---

> **می‌خواهم یک باگ را درست کنم یا قابلیتی اضافه کنم. از کجا شروع کنم؟**
> پروژه را بسازید، تست‌ها را اجرا کنید (نه root لازم است، نه Nginx) و ببینید هر چیز کجاست.

## چه چیزهایی لازم دارید

- لینوکس، [Go](https://go.dev) نسخه‌ی **۱.۲۶ یا بالاتر** (به `go.mod` نگاه کنید) و `git`.
- Nginx اختیاری است. بدون آن هم تست‌ها اجرا می‌شوند و تست‌هایی که به Nginx واقعی نیاز دارند خودشان رد می‌شوند.

## ساخت از سورس

### مثل نصب‌کننده نصبش کنید

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
make install
```

این دستور معادل نصب‌کننده‌ی یک‌دستوری است، برای وقتی که نمی‌خواهید فایل آماده را از GitHub Releases بگیرید. این کارها را انجام می‌دهد:

1. برنامه را با `go build -trimpath -ldflags="-s -w"` می‌سازد.
2. در `/opt/gonix/bin/gonix` نصبش می‌کند و `/usr/local/bin/gonix` را به آن لینک می‌کند.
3. پیکربندی پیش‌فرض را در `/etc/gonix/gonix.yaml` می‌گذارد، بی‌آنکه فایل موجود را بازنویسی کند.
4. پوشه‌های `/etc/gonix/{backups,accesslists}` و `/var/log/gonix` را می‌سازد.
5. سیاست logrotate را در `/etc/logrotate.d/gonix` نصب می‌کند (برای لاگ Nginx هر هاست و audit log). اگر `logrotate` نصب نباشد، با یک پیام رد می‌شود.

### ساخت برای توسعه

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
go mod download

go build -o gonix ./cmd/gonix
sudo ./gonix          # برای مدیریت /etc/nginx و systemctl به root نیاز است
```

### اجرا با `go run`

هنگام توسعه می‌توانید مرحله‌ی build را رد کنید. برنامه برای دست زدن به `/etc/nginx` و `systemctl` به root نیاز دارد، پس با `sudo` اجرایش کنید؛
اگر می‌خواهید `GONIX_CONFIG` سفارشی هم منتقل شود، از `sudo -E` استفاده کنید:

```bash
sudo go run ./cmd/gonix
```

کش ماژول‌ها و build معمولاً زیر کاربر خودتان است، پس اولین `sudo go run` در یک checkout تازه ممکن است یک بار با root بسازد.
اگر نمی‌خواهید خود `go` با root اجرا شود، اول یک فایل اجرایی بسازید و بعد با `sudo` اجرایش کنید: `go build -o gonix ./cmd/gonix && sudo ./gonix`.

اگر می‌خواهید رابط را **بدون دست زدن به Nginx واقعی** امتحان کنید، `configs/default.yaml` را کپی کنید، مسیرهایش را روی چند پوشه‌ی آزمایشی بگذارید و اجرا کنید:

```bash
sudo GONIX_CONFIG=/path/to/dev-gonix.yaml go run ./cmd/gonix
```

### ساخت نهایی (production)

```bash
go build -trimpath -ldflags="-s -w" -o gonix ./cmd/gonix
```

- `-trimpath` مسیرهای محلی فایل‌سیستم را از فایل اجرایی حذف می‌کند، تا ساخت قابل تکرار باشد و اطلاعات شخصی بیرون نرود.
- `-ldflags="-s -w"` جدول symbolها و اطلاعات debug (DWARF) را حذف می‌کند و فایل اجرایی کوچک‌تر می‌شود.

### کراس‌کامپایل

```bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gonix-linux-amd64 ./cmd/gonix
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o gonix-linux-arm64 ./cmd/gonix
```

[`release.yml`](https://github.com/mrmmg/gonix/blob/main/.github/workflows/release.yml) دقیقاً همین را برای هر معماری اجرا می‌کند و علاوه بر آن
`-X .../internal/tui.Version=<tag>` را می‌دهد تا `gonix --version` نسخه‌ی منتشرشده را نشان دهد. برای همین هم نام فایل‌ها یکی است:
`gonix-linux-amd64` و `gonix-linux-arm64`.

### Makefile

```bash
make build     # ساخت برای توسعه
make release   # ساخت نهایی (trimpath + strip)
make cross     # کراس‌کامپایل linux/amd64 و linux/arm64
make test      # go test ./...
make vet       # go vet ./...
make lint      # vet + بررسی fmt
make install   # ساخت release و نصب در /opt/gonix (در صورت نیاز sudo می‌خواهد)
make clean     # حذف فایل‌های اجرایی ساخته‌شده
make docs      # ساخت همین سایت مستندات
```

## تست‌ها

```bash
go test ./...
go vet ./...
go fmt ./...
```

تست‌ها از پوشه‌های موقت و پیاده‌سازی‌های جعلی دستورهای بیرونی (`nginx -t` و `systemctl`) استفاده می‌کنند؛ پس کل مجموعه **بدون Nginx واقعی و بدون root**
اجرا می‌شود. برای هیچ‌کدام از دستورهای بالا `sudo` لازم نیست.

چند حالت مفید وقتی روی یک قسمت کار می‌کنید:

```bash
go test ./internal/nginx/...           # فقط یک بسته
go test ./... -run TestRenderHost -v   # فقط یک تست، با خروجی کامل
go test ./... -cover                   # همراه با خلاصه‌ی پوشش تست
```

اگر `nginx` نصب باشد، تست‌های اضافه هر پیکربندی تولیدشده را از `nginx -t` واقعی می‌گذرانند و بعضی‌ها Nginx را روی پورت‌های آزاد محلی بالا می‌آورند تا
رفتار را بررسی کنند (ریدایرکت، لیست IP، محدودیت نرخ، حالت تعمیر، کش). اگر `nginx` نباشد، خودشان رد می‌شوند. بررسی پروکسی stream به ماژول stream نیاز دارد،
پس در صورت درخواست داخل Docker اجرا می‌شود:

```bash
GONIX_DOCKER_TEST=1 go test ./internal/streams/ -run Docker
```

## ساختار پروژه

```text
gonix/
├── cmd/gonix/             نقطه‌ی ورود: پیکربندی، سرویس‌های backend و TUI را به هم وصل می‌کند
├── internal/
│   ├── tui/               صفحه‌ها، ناوبری و ظاهر (Bubble Tea)؛ هرگز مستقیم به فایل‌ها دست نمی‌زند
│   ├── nginx/             مدل دامنه (Host/Location/...)، ساخت فایل از template، کار با
│   │                      فایل‌سیستم (sites-available/enabled)، اعتبارسنج `nginx -t` و یک
│   │                      parser پیکربندی‌های managed، و هاست‌های adopt‌شده (دست‌نویس) که درجا ویرایش و با بازخوانی بررسی می‌شوند؛
│   │   └── ast/           درخت نحوی بی‌اتلاف Nginx: Print(Parse(x)) == x، بایت‌به‌بایت
│   ├── hosts/             workflow امن: بررسی ورودی → پشتیبان → نوشتن → تست →
│   │                      reload یا برگرداندن → audit
│   ├── certificates/      پیدا کردن و بررسی گواهی TLS (crypto/x509) و گواهی‌های certbot
│   ├── acme/              گواهی Wildcard: certbot + چالش DNS-01 با API کلودفلر
│   ├── accesslist/        لیست‌های رمز (HTTP Basic Auth) بر پایه‌ی فایل htpasswd (bcrypt)
│   ├── errorpages/        snippetهای نام‌دار صفحه‌ی خطا (error_page → <code>.html) برای include در هاست‌ها
│   ├── audit/             audit log (فقط افزودنی)
│   ├── backup/            نسخه‌ی پشتیبان و برگرداندن پیکربندی
│   ├── system/            کنترل systemctl + پایش پردازه‌ها و پورت‌ها از /proc
│   ├── logs/              مسیر لاگ‌ها، پیکربندی logrotate و parse لاگ دسترسی برای لاگ زنده
│   ├── config/            پیکربندی متمرکز با YAML (بدون مسیر ثابت داخل کد)
│   ├── snippet/           مجموعه‌ای از فایل‌های تولیدشده را با یک nginx -t، برگرداندن و dry-run می‌نویسد
│   ├── iplist/ realip/ presets/ ratelimit/ maintenance/ geoblock/
│   │                      برای هر قابلیت snippet‌دار یک بسته
│   ├── upstreams/ streams/ health/
│   │                      گروه‌های upstream، پروکسی TCP/UDP و بررسی در دسترس بودن upstream
│   ├── lint/              Config Check
│   ├── history/ transfer/ npm/ diff/
│   │                      تاریخچه‌ی Git، export/import، import از NPM، diff یکپارچه
│   ├── app/               همه‌ی سرویس‌ها را از روی پیکربندی می‌سازد (مشترک بین TUI و CLI)
│   ├── cli/               دستورهای غیرتعاملی و سرور MCP
│   └── nginxtest/         ابزار تست که یک nginx واقعی را روی پورت‌های آزاد اجرا می‌کند
├── skills/gonix/          skill عامل هوش مصنوعی (SKILL.md)، داخل فایل اجرایی جاسازی شده
├── templates/             templateهای جاسازی‌شده‌ی Nginx (host.tmpl، location.tmpl، errorpages.tmpl)
├── configs/
│   ├── default.yaml       پیکربندی پیش‌فرض، که همراه نسخه‌ی منتشرشده هم می‌آید
│   └── logrotate.conf     سیاست logrotate که `make install` و install.sh نصب می‌کنند
├── install.sh             نصب‌کننده‌ی یک‌دستوری: نسخه‌ی منتشرشده را دانلود می‌کند، بدون نیاز به Go
├── Makefile               `make install` از سورس می‌سازد و در همان ساختار نصب می‌کند
├── .github/workflows/
│   ├── release.yml        با هر push تگ vX.Y.Z نسخه‌ی منتشرشده را می‌سازد و منتشر می‌کند
│   ├── download-count.yml badge دانلود README را به‌روز می‌کند (روی شاخه‌ی `badges`)
│   └── docs.yml           همین مستندات را می‌سازد و روی GitHub Pages منتشر می‌کند
├── docs/                  همین مستندات (یک ماژول Go جدا؛ هرگز جزو فایل اجرایی نیست)
└── tests/                 تست‌های اصلی کنار خود بسته‌ها، در فایل‌های *_test.go هستند
```

هر لایه مسئولیت خودش را دارد: TUI به `internal/hosts` (لایه‌ی workflow) می‌رسد و آن هم `internal/nginx` (ساخت فایل و فایل‌سیستم)،
`internal/backup` و `internal/audit` را صدا می‌زند. **TUI هرگز مستقیم در `/etc/nginx` نمی‌نویسد.**

## قواعد مشارکت

- از قراردادهای استاندارد Go پیروی کنید: `gofmt`، `go vet` و تست برای رفتار جدید.
- TUI (`internal/tui`) نباید مستقیم به فایل‌سیستم یا Nginx دسترسی داشته باشد. قابلیت جدید backend در بسته‌ی `internal/*` مناسبش می‌رود و از طریق `tui.Deps` به TUI می‌رسد.
- تغییری که پیکربندی Nginx می‌نویسد باید از workflow مشترک بگذرد (پشتیبان، `nginx -t`، برگرداندن، audit)؛ تا CLI، TUI و MCP همان تضمین‌ها را داشته باشند.

## نسخه‌های منتشرشده چطور ساخته می‌شوند

با push کردن تگ `vX.Y.Z`، [workflow `Release`](https://github.com/mrmmg/gonix/blob/main/.github/workflows/release.yml) اجرا می‌شود. `linux/amd64` و
`linux/arm64` را می‌سازد و فقط اگر هر دو موفق شوند، یک GitHub Release دقیقاً برای همان تگ منتشر می‌کند، شامل فایل‌های اجرایی، `checksums.txt` و پیکربندی پیش‌فرض.
هیچ‌چیز دستی ساخته یا ویرایش نمی‌شود. پوشه‌های `art/` و `docs/` پیش از ساخت از context حذف می‌شوند؛ پس هیچ‌کدام وارد نسخه‌ی منتشرشده نمی‌شوند.

یک workflow جدا (`download-count.yml`) تعداد دانلود فایل‌های اجرایی را جمع می‌زند و به‌شکل badge در README منتشر می‌کند.

## کار روی همین مستندات

مستندات یک سایت [Hugo](https://gohugo.io) با قالب [Hextra](https://imfing.github.io/hextra/) است. به Hugo نسخه‌ی **extended** و ۰.۱۵۸ یا بالاتر نیاز دارد.

```bash
cd docs
hugo server       # http://localhost:1313/gonix/
```

صفحه‌ها فایل Markdown در `docs/content/en` و `docs/content/fa` هستند. هر دو زبان را هماهنگ نگه دارید و برای هر صفحه، در هر دو زبان یک نام فایل یکسان بگذارید.
با push روی `main`، سایت خودکار منتشر می‌شود.
