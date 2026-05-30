<p align="center">
  <img src="img/iPst.svg" alt="iPShadowT Logo" width="300"/>
</p>


<p align="center">
  <strong>موتور تانل چند-ترنسپورت ضد فیلترینگ</strong>
</p>

<p align="center">
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/VERSION"><img src="https://img.shields.io/badge/نسخه-v1.0.0-blue?style=flat-square" alt="Version"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/blob/master/LICENSE"><img src="https://img.shields.io/badge/لایسنس-MIT-green?style=flat-square" alt="License"/></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"/></a>
  <a href="https://github.com/iPmartNetwork/iPShadowT/releases"><img src="https://img.shields.io/badge/پلتفرم-linux%20%7C%20macos%20%7C%20windows-lightgrey?style=flat-square" alt="Platform"/></a>
</p>

<p align="center">
  <a href="#-شروع-سریع">شروع سریع</a> •
  <a href="#-ویژگیها">ویژگی‌ها</a> •
  <a href="#-ترنسپورتها">ترنسپورت‌ها</a> •
  <a href="#-ضد-dpi">ضد DPI</a> •
  <a href="#-پیکربندی">پیکربندی</a> •
  <a href="CHANGELOG.md">تغییرات</a> •
  <a href="README.md">English</a>
</p>

---

## 📋 معرفی

iPShadowT یک موتور تانلینگ با عملکرد بالا و بدون وابستگی خارجی است که برای عبور از سیستم‌های بازرسی عمیق بسته (DPI) و سانسور اینترنت طراحی شده. این ابزار ۸ پروتکل انتقال، ۱۵ تکنیک پنهان‌سازی و انتخاب هوشمند خودکار را در یک باینری Go ترکیب کرده است.

ساخته شده برای بقا حتی در شدیدترین سناریوهای فیلترینگ — از جمله قطعی کامل اینترنت که فقط ترافیک DNS اجازه عبور دارد.

---

## ⚡ شروع سریع

```bash
# دانلود
wget https://github.com/iPmartNetwork/iPShadowT/releases/latest/download/ipshadowt-linux-amd64
chmod +x ipshadowt-linux-amd64

# تولید کلید
./ipshadowt-linux-amd64 --gen-reality-keys

# اجرای سرور (خارج)
./ipshadowt-linux-amd64 -c server.toml

# اجرای کلاینت (ایران)
./ipshadowt-linux-amd64 -c client.toml
```

یا نصب با یک خط:

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh -o ipshadowt-manager.sh && sudo bash ipshadowt-manager.sh
```

---

## ✨ ویژگی‌ها

### 🚀 هسته

| ویژگی | توضیح |
|--------|--------|
| ۸ ترنسپورت | TCP, WebSocket, HTTP/2, gRPC, REALITY, ShadowTLS, QUIC, KCP, Reverse |
| ۱۵ تکنیک پنهان‌سازی | uTLS, Fragment, ECH, Shaping, Domain Fronting, DNS Tunnel و بیشتر |
| مالتی‌پلکسینگ | هزاران stream روی یک اتصال (smux) |
| چند مسیر | سوئیچ خودکار بین مسیرها |
| رمزنگاری | XChaCha20-Poly1305 AEAD |
| بدون وابستگی | یک باینری استاتیک، بدون نیاز به ابزار خارجی |

### 🛡️ ضد DPI

| تکنیک | هدف |
|--------|------|
| REALITY | سرور به اسکنرها وبسایت واقعی نشان می‌دهد |
| uTLS | جعل fingerprint مرورگر (Chrome/Firefox/Safari) |
| TLS Fragmentation | شکستن ClientHello برای مخفی کردن SNI |
| ECH | رمزنگاری Client Hello |
| Traffic Shaping | ترافیک شبیه وب‌گردی عادی |
| Domain Fronting | مخفی کردن مقصد واقعی پشت CDN |
| DNS Tunnel | آخرین راه نجات — وقتی فقط DNS باز باشد |
| Protocol Morphing | تبدیل ترافیک به شکل HTTP/2, TLS, DNS |
| Decoy Traffic | تولید نویز برای مخفی کردن الگو |
| HalfDuplex | کانال‌های جدا برای آپلود/دانلود |

### 📡 شبکه

| ویژگی | توضیح |
|--------|--------|
| Port Forwarding | TCP, UDP, SOCKS5, HTTP proxy |
| Split Tunneling | IP‌های ایران مستقیم، بقیه از تانل |
| Load Balancer | ۵ استراتژی (round-robin, least-conn, weighted, IP-hash, fastest) |
| پشتیبانی CDN | Cloudflare, Fastly, آروان، سفارشی |
| DNS over HTTPS | عبور از DNS Poisoning |
| TUN/TAP | گرفتن تمام ترافیک سیستم (لایه ۲ و ۳) |

### 🔧 مدیریت

| ویژگی | توضیح |
|--------|--------|
| پنل وب | داشبورد با آمار لحظه‌ای |
| REST API | API کامل مدیریت |
| مدیریت کاربران | چند کاربر با محدودیت حجم و انقضا |
| لینک اشتراک | سازگار با V2RayNG/Clash |
| Prometheus Metrics | مانیتورینگ با Grafana |
| آپدیت خودکار | از GitHub releases |
| بکاپ/بازیابی | بکاپ خودکار دوره‌ای |
| Hot-Reload | تغییر config بدون restart |

### 🧠 هوشمندسازی

| ویژگی | توضیح |
|--------|--------|
| تشخیص DPI | شناسایی خودکار DPI فعال |
| انتخاب خودکار پروتکل | بهترین ترنسپورت برای شرایط فعلی |
| سوئیچ هوشمند | تغییر ترنسپورت در صورت افت کیفیت |
| تست سرعت | اندازه‌گیری throughput |

---

## 🔌 ترنسپورت‌ها

| ترنسپورت | پورت | مقاومت DPI | CDN | سرعت |
|-----------|------|-----------|-----|------|
| `tcpmux` | هر پورت | ⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `wsmux` | 443 | ⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `h2mux` | 443 | ⭐⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `grpc` | 443 | ⭐⭐⭐⭐ | ✅ | ⭐⭐⭐⭐ |
| `reality` | 443 | ⭐⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |
| `shadowtls` | 443 | ⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |
| `quic` | 443 | ⭐⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `kcp` | هر پورت | ⭐⭐⭐⭐⭐ | ❌ | ⭐⭐⭐⭐⭐ |
| `reverse` | 443 | ⭐⭐⭐ | ❌ | ⭐⭐⭐⭐ |

---

## ⚙️ پیکربندی

### سرور (server.toml)

```toml
mode = "server"
transport = "reality"
bind_addr = "0.0.0.0:443"
password = "رمز-مشترک"

[reality]
server_name = "www.google.com"
private_key = "کلید_خصوصی_سرور"
short_id = "شناسه_کوتاه"
dest = "www.google.com:443"

[performance]
nodelay = true
kernel_tuning = true
```

### کلاینت (client.toml)

```toml
mode = "client"
transport = "reality"
remote_addr = "آدرس-سرور:443"
password = "رمز-مشترک"

[reality]
server_name = "www.google.com"
public_key = "کلید_عمومی_سرور"
short_id = "شناسه_کوتاه"

[anti_dpi]
enabled = true
utls_fingerprint = "chrome"
fragment = true

[[forwards]]
name = "socks5"
type = "socks5"
listen = "127.0.0.1:1080"
```

---

## 🏗️ معماری

```
┌─────────────────────────────────────────────────┐
│                  iPShadowT                      │
├─────────────────────────────────────────────────┤
│  ورودی: SOCKS5 / HTTP / TCP / UDP / TUN         │
│  ↓                                              │
│  Split Tunnel (ایران مستقیم، بقیه پروکسی)       │
│  ↓                                              │
│  مالتی‌پلکسر (smux - هزاران stream)             │
│  ↓                                              │
│  رمزنگاری (XChaCha20-Poly1305 + Padding)       │
│  ↓                                              │
│  ضد-DPI (uTLS + Fragment + Shaping + ECH)      │
│  ↓                                              │
│  ترنسپورت (REALITY / WS / H2 / gRPC / ...)    │
│  ↓                                              │
│  چند مسیر (سوئیچ خودکار بین مسیرها)            │
└─────────────────────────────────────────────────┘
```

---

## 📦 کامپایل

```bash
git clone https://github.com/iPmartNetwork/iPShadowT.git
cd iPShadowT
go mod tidy
make build-linux
```

---

## 🐳 داکر

```bash
docker build -t ipshadowt .
docker run -v ./config.toml:/etc/ipshadowt/config.toml -p 443:443 ipshadowt
```

---

## 🖥️ اسکریپت مدیریت

مدیریت کامل تعاملی با یک دستور:

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/iPShadowT/master/deploy/ipshadowt-manager.sh -o ipshadowt-manager.sh && sudo bash ipshadowt-manager.sh
```

قابلیت‌ها:
- 🚀 نصب یک‌کلیکی (دانلود خودکار باینری + پیش‌نیازها)
- ⚙️ ویزارد تعاملی تنظیم تانل (ایران/خارج)
- 🔀 مولتی‌تانل: یک‌به‌چند، چندبه‌یک، یک‌به‌یک
- 🧪 تشخیص خودکار بهترین ترنسپورت
- 🔑 تولید کلید REALITY
- 📊 وضعیت زنده، لاگ، تست سرعت، اطلاعات سیستم
- 🌐 ابزارهای شبکه (تشخیص DPI، بررسی پورت، BBR)
- 🐕 Watchdog اتصال (ریستارت خودکار در صورت قطعی)
- 🔥 تنظیم خودکار فایروال
- 💾 بکاپ / بازیابی با بکاپ خودکار (cron)
- 📡 مدیریت Port Forward (اضافه/حذف از منو)
- 📤 خروجی config کلاینت (آماده کپی)
- 🔄 آپدیت یک‌کلیکی از GitHub

---

## 🔒 امنیت

- رمزنگاری XChaCha20-Poly1305
- تبادل کلید ECDH X25519 (REALITY)
- احراز هویت HMAC-SHA256 با محافظت replay
- Certificate pinning
- IP whitelist/blacklist
- محافظت brute-force
- لاگ کامل audit

---

## 📄 لایسنس

[MIT](LICENSE)

---

<p align="center">
  <img src="img/iPst.png" alt="iPShadowT" width="150"/>
  <br/>
  <sub>ساخته شده با ❤️ توسط <a href="https://github.com/iPmartNetwork">iPmart Network</a> (Ali Hassanzadeh)</sub>
  <br/>
  <sub>© 2026 iPmart Network. تمامی حقوق محفوظ است.</sub>
</p>

