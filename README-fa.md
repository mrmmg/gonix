# GoNix

*[English](README.md)*

**GoNix** — نام آن برگرفته از ترکیب **Go** و **Nginx** است — یک **رابط کاربری متنی (TUI)**
پیشرفته و تعاملی برای **مدیریت Nginx** روی لینوکس است که با زبان Go نوشته شده است.

<p align="center">
  <video src="./art/gonix_preview.mp4" controls alt="GoNix Preview"></video>
</p>

این پروژه از [Nginx Proxy Manager](https://nginxproxymanager.com/) الهام گرفته، اما رویکرد
متفاوتی دارد: به‌جای پنهان‌کردن Nginx پشت یک پایگاه‌داده و یک قالب پیکربندی اختصاصی، GoNix یک
لایه‌ی مدیریتی نازک و ایمن **روی** فایل‌های پیکربندی واقعی و قابل‌ویرایش Nginx است. شما همیشه
می‌توانید فایل `/etc/nginx/sites-available/example.com` را در یک ویرایشگر متن باز کنید و
سینتکس استاندارد و ساده‌ی Nginx را ببینید — چیزی مبهم یا اختصاصی وجود ندارد.

باینری کامپایل‌شده با نام `gonix` شناخته می‌شود.

## نصب

آخرین نسخه‌ی GoNix را با یک دستور نصب کنید:

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash
```

همین‌قدر کافی است. نصب‌کننده به‌طور خودکار معماری سیستم شما را تشخیص می‌دهد، آخرین ریلیز را
دانلود می‌کند، GoNix را نصب می‌کند و پیکربندی پیش‌فرض را آماده می‌کند. پس از پایان، اجرا کنید:

```bash
sudo gonix
```

برای نصب یک نسخه‌ی مشخص به‌جای آخرین نسخه:

```bash
curl -fsSL https://raw.githubusercontent.com/mrmmg/gonix/main/install.sh | bash -s -- v1.2.3
```

اجرای دوباره‌ی نصب‌کننده — با یا بدون آرگومان نسخه — به‌صورت **بروزرسانی** روی نصب موجود عمل
می‌کند: باینری `gonix` را با نسخه‌ی درخواست‌شده جایگزین می‌کند و نسخه‌ی قبلاً نصب‌شده را گزارش
می‌دهد، اما هرگز به پیکربندی موجود شما در `/etc/gonix` دست نمی‌زند.

اگر از قبل دسترسی `sudo` کش‌شده ندارید، نصب‌کننده هنگام نیاز به نوشتن در `/opt`، `/etc` یا
`/usr/local/bin` رمز عبور شما را می‌پرسد.

### معماری‌های پشتیبانی‌شده

| سیستم‌عامل | معماری | فایل ریلیز              |
|------------|--------|---------------------------|
| Linux      | amd64  | `gonix-linux-amd64`      |
| Linux      | arm64  | `gonix-linux-arm64`      |

هر ترکیب سیستم‌عامل/معماری دیگری با خطایی واضح توسط نصب‌کننده رد می‌شود، به‌جای اینکه چیزی
دانلود شود که اصلاً اجرا نمی‌شود.

### چه چیزی و در کجا نصب می‌شود

| مسیر                            | کاربرد                                                          |
|----------------------------------|-------------------------------------------------------------------|
| `/opt/gonix/bin/gonix`           | باینری نصب‌شده                                                     |
| `/opt/gonix/VERSION`             | نسخه‌ی فعلاً نصب‌شده                                                |
| `/usr/local/bin/gonix`           | symlink به باینری بالا، تا `gonix` از هر جای `$PATH` در دسترس باشد |
| `/etc/gonix/gonix.yaml`          | فایل پیکربندی (فقط یک‌بار ساخته می‌شود، هرگز overwrite نمی‌شود)     |
| `/etc/gonix/backups/`            | پشتیبان‌های خودکار پیکربندی                                        |
| `/etc/gonix/accesslists/`        | فایل‌های Access List برای HTTP Basic Auth                          |
| `/var/log/gonix/audit.log`       | لاگ حسابرسی (audit log)                                            |
| `/etc/logrotate.d/gonix`         | سیاست چرخش لاگ (در صورت نصب‌بودن `logrotate`)                      |

### یا نصب از سورس

اگر ترجیح می‌دهید به‌صورت محلی بسازید (بدون دانلود ریلیز از GitHub)، بخش «ساخت از سورس» را در
ادامه‌ی همین سند ببینید — دستور `make install` باینری را با `go build` می‌سازد و آن را دقیقاً در
همان ساختار `/opt/gonix` توضیح‌داده‌شده در بالا نصب می‌کند.

### نحوه‌ی ساخت ریلیزها

با push کردن یک تگ نسخه (`vX.Y.Z`، مثلاً `v1.2.3`) به این مخزن، ورک‌فلوی
[`Release` در GitHub Actions](.github/workflows/release.yml) اجرا می‌شود که باینری‌های
`linux/amd64` و `linux/arm64` را می‌سازد و — فقط در صورت موفقیت هر دو build — یک GitHub Release
دقیقاً برای همان تگ منتشر می‌کند که شامل باینری‌ها، یک `checksums.txt` و پیکربندی پیش‌فرض است.
هیچ بخشی از یک ریلیز به‌صورت دستی ساخته یا ویرایش نمی‌شود.

## امکانات

- **مدیریت هاست‌ها** — ساخت، فهرست‌کردن، فعال/غیرفعال‌کردن و حذف هاست‌های مجازی، با نام‌گذاری
  بر اساس دامنه (`sites-available/example.com`، نه یک شناسه‌ی مبهم)، و لینک‌شده به
  `sites-enabled` به روش استاندارد Nginx.
- **پراکسی معکوس و هاست‌های استاتیک** — راه‌اندازی هدایت‌شده برای host/port بالادستی، پروتکل،
  نسخه‌ی HTTP، تایم‌اوت‌ها، محدودیت حجم بدنه‌ی درخواست و هدرهای forward.
- **Location‌ها** — افزودن location‌های اضافیِ پراکسی معکوس به یک هاست، یا یک بلوک location
  کاملاً سفارشی برای موارد پیشرفته.
- **پشتیبانی از WebSocket** — یک سوییچ ساده‌ی بله/خیر دایرکتیوهای درست
  `Upgrade`/`Connection`/`proxy_http_version` را پیکربندی می‌کند؛ شما هرگز خودتان آن‌ها را
  نمی‌نویسید.
- **SSL/TLS** — گواهی‌ها را از یک دایرکتوری متمرکز
  (`/etc/nginx/certs/<domain>/{fullchain,privkey}.pem`) ارجاع می‌دهد به‌جای کپی‌کردن آن‌ها در
  هر هاست، پس تمدید یک گواهی فقط جایگزین‌کردن دو فایل است. هر دو نوع RSA و ECDSA پشتیبانی و
  به‌طور خودکار تشخیص داده می‌شوند.
- **بازرسی گواهی** — تاریخ انقضا (خوانا برای انسان و دقیق)، صادرکننده، subject، SANها،
  اثرانگشت SHA-256، تطابق کلید/گواهی و اعتبارسنجی زنجیره، همگی با استفاده از `crypto/x509` در
  Go (بدون نیاز به فراخوانی زیرفرآیند `openssl`).
- **Access List (احراز هویت پایه‌ی HTTP)** — فهرست‌های کاربریِ قابل‌استفاده‌ی مجدد و
  bcrypt-hash‌شده در قالب `htpasswd` که می‌توان روی یک هاست یا یک location خاص اعمال کرد.
- **لاگ‌های access/error** — فعال/غیرفعال‌سازی به‌ازای هر هاست، با logrotate پیکربندی‌شده در
  زمان نصب.
- **لاگ حسابرسی (audit log)** — هر تغییری که از طریق برنامه انجام شود با زمان، کاربر لینوکس،
  اکشن، هدف و نتیجه ثبت می‌شود.
- **کنترل و مانیتورینگ سرویس Nginx** — start/stop/restart/reload/test، به همراه کشف
  master/worker process، مصرف حافظه، uptime و پورت‌های در حال listen، همگی مستقیماً از `/proc`
  خوانده می‌شوند.
- **ایمنی در اولویت اول** — هر تغییر پیکربندی، پشتیبان‌گیری، نوشته و با `nginx -t` تست می‌شود،
  و فقط در صورت موفقیت تست reload می‌شود. یک تست ناموفق به‌طور خودکار پیکربندی قبلی را بازیابی
  می‌کند و خطا را در audit log ثبت می‌کند. Nginx هرگز در وضعیت خراب رها نمی‌شود.
- **همزیستی با پیکربندی دستی** — هاست‌هایی که توسط GoNix ساخته نشده‌اند به‌صورت read-only و با
  نشانگر واضح `[unmanaged]` نمایش داده می‌شوند، نه اینکه بی‌سروصدا بازنویسی شوند.

## پیش‌نیازها

- لینوکس (از `systemctl`، `/proc` و مسیرهای استاندارد Nginx استفاده می‌کند)
- [Go](https://go.dev) نسخه‌ی ۱.۲۴ یا جدیدتر (برای ساخت از سورس)
- Nginx
- `systemctl` (systemd)
- اختیاری: `logrotate` (در صورت نبود، چرخش لاگ با یک هشدار رد می‌شود)

## ساخت از سورس

### نصب از سورس با `make install`

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix
make install
```

این معادلِ ساخت‌ازسورسِ نصب‌کننده‌ی یک‌دستوریِ بالاست، برای زمانی که نمی‌خواهید باینری
از‌پیش‌ساخته را از GitHub Releases بگیرید. این دستور:

۱. باینری را از سورس می‌سازد (با `go build -trimpath -ldflags="-s -w"`).
۲. آن را در `/opt/gonix/bin/gonix` نصب و `/usr/local/bin/gonix` را به آن symlink می‌کند.
۳. پیکربندی پیش‌فرض را در `/etc/gonix/gonix.yaml` نصب می‌کند (بدون overwrite کردن نسخه‌ی
   موجود).
۴. مسیرهای `/etc/gonix/{backups,accesslists}` و `/var/log/gonix` را می‌سازد.
۵. یک سیاست logrotate در `/etc/logrotate.d/gonix` نصب می‌کند که هم لاگ‌های Nginx هر هاست و هم
   audit log را پوشش می‌دهد (در صورت نبود `logrotate` با یک پیام رد می‌شود).

سپس:

```bash
sudo gonix
```

### ساخت توسعه (development build)

```bash
git clone https://github.com/mrmmg/gonix.git
cd gonix

go mod download

# ساخت توسعه
go build -o gonix ./cmd/gonix

# اجرای محلی (root لازم است تا /etc/nginx و systemctl مدیریت شوند)
sudo ./gonix
```

### اجرا با `go run` (توسعه)

در زمان توسعه می‌توانید مرحله‌ی build صریح را حذف کنید و مستقیم از سورس با `go run` اجرا کنید.
از آنجا که `main.go` برای دسترسی به `/etc/nginx` و `systemctl` نیاز به root دارد، آن را با
`sudo` اجرا کنید (از `sudo -E` استفاده کنید اگر می‌خواهید یک `GONIX_CONFIG` سفارشی را هم عبور
دهید):

```bash
sudo go run ./cmd/gonix
```

از آنجا که cache ماژول و cache ساخت معمولاً زیر کاربر خودتان قرار دارند، اولین اجرای
`sudo go run` در یک چک‌اوت تازه ممکن است نیاز داشته باشد یک‌بار به‌عنوان root ساخته شود؛ اگر
ترجیح می‌دهید خودِ `go` را با root اجرا نکنید، به‌جای آن `sudo` را روی یک باینری توسعه‌ی
از‌پیش‌ساخته اجرا کنید (`go build -o gonix ./cmd/gonix && sudo ./gonix`).

برای اشاره به یک پیکربندی موقت به‌جای `/etc/nginx` واقعی (مناسب برای امتحان‌کردن رابط کاربری
بدون دست‌زدن به تنظیمات واقعی Nginx شما)، `configs/default.yaml` را کپی کنید، مسیرهای آن را به
دایرکتوری‌های موقت محلی تغییر دهید و اجرا کنید:

```bash
sudo GONIX_CONFIG=/path/to/dev-gonix.yaml go run ./cmd/gonix
```

### ساخت نسخه‌ی نهایی (production)

```bash
go build -trimpath -ldflags="-s -w" -o gonix ./cmd/gonix
```

- `-trimpath` مسیرهای فایل‌سیستم محلی را از باینری کامپایل‌شده حذف می‌کند، برای ساخت‌های
  تکرارپذیر (reproducible) و بدون افشای اطلاعات محلی.
- `-ldflags="-s -w"` جدول نمادها (symbol table) و اطلاعات دیباگ DWARF را حذف می‌کند و باینری
  کوچک‌تری تولید می‌کند.

### کراس-کامپایل

```bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gonix-linux-amd64 ./cmd/gonix
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o gonix-linux-arm64 ./cmd/gonix
```

این دقیقاً همان کاری است که [`.github/workflows/release.yml`](.github/workflows/release.yml)
برای هر معماری انجام می‌دهد (به‌علاوه‌ی عبور `-X .../internal/tui.Version=<tag>` تا
`gonix --version` نسخه‌ی منتشرشده را گزارش دهد)، به همین دلیل نام فایل‌ها یکسان است:
`gonix-linux-amd64` و `gonix-linux-arm64`.

### Makefile

یک `Makefile` دستورات بالا را در خود دارد:

```bash
make build     # ساخت توسعه
make release   # ساخت نهایی (trimpath + stripped)
make cross     # کراس-کامپایل برای linux/amd64 و linux/arm64
make test      # go test ./...
make vet       # go vet ./...
make lint      # بررسی vet + fmt
make install   # ساخت نهایی + نصب در /opt/gonix (در صورت نیاز رمز sudo پرسیده می‌شود)
make clean     # حذف باینری‌های ساخته‌شده
```

## دستورات توسعه

```bash
go test ./...
go vet ./...
go fmt ./...
```

تست‌ها از دایرکتوری‌های موقت و پیاده‌سازی‌های fake برای دستورات خارجی (`nginx -t`،
`systemctl`) استفاده می‌کنند، پس کل مجموعه‌ی تست بدون نیاز به نصب واقعی Nginx یا دسترسی root
اجرا می‌شود — برای هیچ‌کدام از دستورات بالا نیازی به `sudo` نیست.

چند حالت کاربردی هنگام کار روی یک پکیج خاص:

```bash
go test ./internal/nginx/...           # فقط یک پکیج
go test ./... -run TestRenderHost -v   # فقط یک تست، به‌صورت verbose
go test ./... -cover                   # همراه با خلاصه‌ی پوشش تست (coverage)
```

## ساختار پروژه

```text
gonix/
├── cmd/gonix/             نقطه‌ی ورود: پیکربندی، سرویس‌های backend و TUI را به هم متصل می‌کند
├── internal/
│   ├── tui/               صفحات Bubble Tea، ناوبری و استایل‌دهی — هرگز مستقیم به فایل‌ها دست نمی‌زند
│   ├── nginx/             مدل دامنه (Host/Location/...)، رندرکننده‌ی قالب، عملیات فایل‌سیستم
│   │                      (sites-available/enabled)، اعتبارسنج `nginx -t`، و یک
│   │                      پارسر best-effort برای پیکربندی‌های مدیریت‌شده و دستی
│   ├── hosts/             گردش‌کار امن و سطح‌بالا: اعتبارسنجی → پشتیبان‌گیری → نوشتن → تست →
│   │                      reload-یا-rollback → audit
│   ├── certificates/      کشف و بازرسی گواهی TLS (crypto/x509)
│   ├── accesslist/        «Access List»های احراز هویت پایه‌ی HTTP، بر پایه‌ی فایل‌های htpasswd (bcrypt)
│   ├── audit/             لاگ حسابرسیِ append-only
│   ├── backup/            پشتیبان‌گیری/بازیابی پیکربندی
│   ├── system/             کنترل systemctl + مانیتورینگ process و پورت بر پایه‌ی /proc
│   ├── logs/               کمک‌تابع‌های مسیر لاگ + تولید پیکربندی logrotate
│   └── config/             پیکربندی متمرکز و مبتنی بر YAML (بدون مسیر hard-code شده)
├── templates/             قالب‌های پیکربندی Nginx که embed شده‌اند (host.tmpl، location.tmpl)
├── configs/
│   ├── default.yaml       پیکربندی پیش‌فرض GoNix، که به‌عنوان یک release asset هم منتشر می‌شود
│   └── logrotate.conf     سیاست logrotate که توسط `make install` و install.sh نصب می‌شود
├── install.sh             نصب‌کننده‌ی یک‌دستوری: یک ریلیز را دانلود می‌کند، بدون نیاز به Go toolchain
├── Makefile               دستور `make install` از سورس می‌سازد و همان ساختار را نصب می‌کند
├── .github/workflows/
│   └── release.yml        با هر push شدن تگ vX.Y.Z یک GitHub Release می‌سازد و منتشر می‌کند
└── tests/                 (فایل‌های *_test.go کنار هر پکیج، مجموعه‌ی تست واقعی را تشکیل می‌دهند)
```

مسئولیت‌ها لایه‌بندی شده‌اند: TUI به `internal/hosts` (لایه‌ی گردش‌کار) فراخوانی می‌زند، که خود
به `internal/nginx` (رندر + فایل‌سیستم)، `internal/backup` و `internal/audit` فراخوانی می‌زند.
TUI هرگز مستقیماً در `/etc/nginx` نمی‌نویسد.

## پیکربندی Nginx

هاست‌ها به روش استاندارد Nginx ذخیره می‌شوند:

```text
/etc/nginx/sites-available/example.com   # همیشه موجود است، منبع اصلی حقیقت (source of truth)
/etc/nginx/sites-enabled/example.com     # symlink به sites-available، فقط زمانی که فعال باشد موجود است
```

غیرفعال‌کردن یک هاست فقط symlink را حذف می‌کند؛ خودِ فایل پیکربندی هرگز حذف نمی‌شود. فایل‌های
پیکربندیِ ساخته‌شده توسط GoNix با یک کامنت `# Managed by GoNix.` شروع می‌شوند، که برنامه از
طریق آن فایل‌های خودش را از فایل‌های دستی در اسکن‌های بعدی تشخیص می‌دهد. هاست‌های بدون این
نشانگر به‌صورت **[unmanaged]** فهرست و در TUI به‌صورت read-only نمایش داده می‌شوند — همچنان
می‌توانید پیکربندی خام آن‌ها را ببینید، اما ویرایش آن‌ها به یک ویرایشگر متن سپرده می‌شود تا از
خراب‌شدن پیکربندیِ ساخته‌نشده توسط GoNix جلوگیری شود.

## گواهی‌های SSL

گواهی‌ها باید در یک دایرکتوری متمرکز باشند (قابل‌تنظیم، پیش‌فرض `/etc/nginx/certs`):

```text
/etc/nginx/certs/
├── example.com/
│   ├── fullchain.pem
│   └── privkey.pem
└── example.org/
    ├── fullchain.pem
    └── privkey.pem
```

پیکربندی ساخته‌شده‌ی هاست مستقیماً به این فایل‌ها ارجاع می‌دهد:

```nginx
ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;
ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;
```

GoNix گواهی‌ها را در دایرکتوری‌های هر هاست کپی **نمی‌کند**، پس تمدید یک گواهی (مثلاً از طریق
`certbot` با DNS challenge، که فرض بر این است به‌صورت دستی/خارجی اجرا می‌شود) فقط جایگزین‌کردن
همان دو فایل سرجایشان است — بدون نیاز به پیکربندی مجدد.

> **TODO (کار آینده):** یکپارچه‌سازی خودکار با Let's Encrypt/Certbot (شامل ارائه‌دهندگان DNS
> challenge) عمداً هنوز پیاده‌سازی نشده است. برای محل اتصال آن به `internal/certificates`
> مراجعه کنید.

## Access List‌ها

یک Access List مجموعه‌ای نام‌گذاری‌شده و قابل‌استفاده‌ی مجدد از کاربران احراز هویت پایه‌ی HTTP
است که به‌صورت یک فایل استاندارد در قالب `htpasswd` (رمزهای عبور bcrypt-hash شده — متن خام هرگز
ذخیره نمی‌شود) در `/etc/gonix/accesslists/<name>.htpasswd` نگهداری می‌شود. اعمال یک Access List
روی یک هاست یا location این خطوط را اضافه می‌کند:

```nginx
auth_basic "Restricted";
auth_basic_user_file /etc/gonix/accesslists/<name>.htpasswd;
```

> **نکته:** رکوردهای htpasswd با هش bcrypt نیاز دارند که Nginx به یک پیاده‌سازی libc/crypt که
> از bcrypt پشتیبانی می‌کند لینک شده باشد (این مورد در glibc مدرن از طریق libxcrypt، و در اکثر
> توزیع‌های لینوکس فعلی صادق است). اگر build شما از Nginx نمی‌تواند هش‌های bcrypt را تأیید کند،
> رکوردهای Access List مربوطه را با ابزاری که الگوریتم پشتیبانی‌شده توسط `crypt(3)` شما را
> تولید می‌کند، دوباره بسازید.

## لاگ‌ها

- **لاگ‌های access/error**: به‌ازای هر هاست، در `/var/log/nginx/<domain>.access.log` و
  `<domain>.error.log`، که به‌طور مستقل از صفحه‌ی مدیریت هر هاست فعال/غیرفعال می‌شوند.
- **لاگ حسابرسی (audit log)**: `/var/log/gonix/audit.log`، هر خط برای یک اکشن:
  `2026-09-15 10:32:11 | user=root | action=host_created | target=example.com | result=success`
- **چرخش (rotation)**: هر دو توسط `logrotate` استاندارد چرخانده می‌شوند، که به‌طور خودکار توسط
  هرکدام از نصب‌کننده‌ها در `/etc/logrotate.d/gonix` پیکربندی می‌شود.

## پشتیبان‌گیری (Backups)

پیش از اینکه GoNix هر تغییری روی فایل پیکربندی یک هاست اعمال کند، یک نسخه‌ی پشتیبان با
timestamp در `/etc/gonix/backups/` ذخیره می‌کند. پس از نوشتن، `nginx -t` را اجرا می‌کند:

- **تست موفق** ← Nginx reload می‌شود و تغییر به‌عنوان موفقیت در audit log ثبت می‌شود.
- **تست ناموفق** ← پیکربندی قبلی به‌طور خودکار از همان پشتیبانی که لحظاتی پیش گرفته شد بازیابی
  می‌شود، Nginx **هرگز** با پیکربندی خراب reload نمی‌شود، و خطا (همراه با خروجی Nginx) در audit
  log ثبت می‌شود.

پشتیبان‌های قدیمی‌تر از تنظیمات `backup.keep_count` (پیش‌فرض ۲۰ نسخه به‌ازای هر فایل) به‌طور
خودکار حذف می‌شوند. صفحه‌ی «Backup / Restore» روی هر هاست به شما اجازه می‌دهد هر نسخه‌ی
نگه‌داشته‌شده را فهرست و به‌صورت دستی بازیابی کنید، با همان مسیر ایمنِ تست-پیش‌از-reload.

## توسعه

مشارکت‌ها باید از قراردادهای استاندارد Go پیروی کنند: `gofmt`، `go vet`، و تست برای رفتار
جدید. TUI (`internal/tui`) را از دسترسی مستقیم به فایل‌سیستم/Nginx دور نگه دارید — قابلیت
جدیدِ backend باید به پکیج مناسب زیر `internal/*` اضافه و از طریق `tui.Deps` در اختیار TUI قرار
گیرد.

## امکانات آینده

عمداً هنوز پیاده‌سازی نشده‌اند، اما معماری برای آن‌ها طراحی شده است:

- یکپارچه‌سازی خودکار با Let's Encrypt / Certbot و ارائه‌دهندگان DNS challenge (مثل Cloudflare)
- فهرست‌های allow/deny بر اساس IP و محدودسازی نرخ (rate limiting)
- هدرهای امنیتی، فشرده‌سازی و پیش‌تنظیم‌های caching
- یکپارچه‌سازی WAF / ModSecurity
- گروه‌های upstream، توزیع بار (load balancing) و health check
- HTTP/3، OCSP stapling، پیش‌تنظیم‌های HSTS
- حالت تعمیر (maintenance mode) و صفحات خطای سفارشی
- درون‌ریزی/دیف‌گیری پیکربندی‌های دلخواه موجود Nginx
- تاریخچه‌ی پیکربندی مبتنی بر Git و rollback
- پشتیبانی از چند نمونه (instance) Nginx
