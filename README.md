# NanoKVM-Pro (community fork)

[![Latest release](https://img.shields.io/github/v/release/pekishev/NanoKVM-Pro)](https://github.com/pekishev/NanoKVM-Pro/releases/latest)
[![License: GPL-3.0](https://img.shields.io/github/license/pekishev/NanoKVM-Pro)](LICENSE)

> **This is a fork** of [sipeed/NanoKVM-Pro](https://github.com/sipeed/NanoKVM-Pro), not an official Sipeed release.
>
> **Это форк** [sipeed/NanoKVM-Pro](https://github.com/sipeed/NanoKVM-Pro), а не официальная прошивка Sipeed.

The hardware, closed components, and most of the web UI still come from upstream. This repository adds Russian input, HID macros, on-device speech recognition, an MCP agent API, and GitHub-based updates.

Железо и закрытые компоненты — как у Sipeed. Здесь — русский ввод, HID-макросы, распознавание речи на устройстве, MCP для агентов и обновления с GitHub.

**Latest build:** [Releases](https://github.com/pekishev/NanoKVM-Pro/releases/latest) (currently based on official **1.2.15**)

## What's new in this fork

### Keyboard and paste

- Russian layout on the virtual keyboard (plus English and French)
- Mixed Russian/English paste: the current host layout is used, Alt+Shift is sent when the script switches language
- Paste dialog: Alt+Shift, Enter, browser voice input, and **on-device Russian ASR** (no cloud)
- Virtual keyboard is a floating window: drag, collapse, resize; position is remembered

### HID macros

BadUSB-style scripts instead of the old Leader Key:

- Record a key or chord into the script
- `STRING`, special keys, `WIN+SHIFT+S` / `CTRL+ALT+DELETE`, `DELAY`
- `MOVE` / `CLICK` / `DBLCLICK` in percent of the screen
- `RUN OtherMacro` (one nesting level)
- `WAITIMAGE name timeout` — wait until a saved HDMI fragment appears
- Menu auto-hides while a macro is playing

### Mouse

- Extra mode **Absolute Simple** (`absolute-simple`) alongside absolute and relative

### MCP (agents)

Authenticated endpoint `/mcp` so an agent can drive the remote PC over HDMI + HID:

`status`, `screenshot`, `click`, `type_keys`, `type_text`, `stop_type`, `press`

Coordinates are full-frame HDMI pixels. Mixed EN/RU typing sends Alt+Shift by itself.

### Updates

- GitHub Releases in both the official `nanokvm_pro_*.tar.gz` format and a smaller `nanokvm-pro-*.tar.gz` (server + web)
- Settings → Update can offer **fork releases**, a custom update server, or **disable checks**
- Manual upload of a `.tar.gz` from the update page

## Install / update

On the device: **Settings → Update → Update Server**

```text
https://github.com/pekishev/NanoKVM-Pro/releases/latest/download
```

Then check for updates, or upload `nanokvm_pro_<version>.tar.gz` / `nanokvm-pro-<version>.tar.gz` from [Releases](https://github.com/pekishev/NanoKVM-Pro/releases).

Manual copy of server + web (from `nanokvm-pro-*.tar.gz`):

```bash
scp NanoKVM-Server root@DEVICE:/kvmapp/server/NanoKVM-Server
scp -r web root@DEVICE:/kvmapp/server/web.new
ssh root@DEVICE 'rm -rf /kvmapp/server/web && mv /kvmapp/server/web.new /kvmapp/server/web && chmod +x /kvmapp/server/NanoKVM-Server && systemctl restart nanokvm'
```

---

## Upstream hardware (Sipeed)

NanoKVM-Pro is an IP-KVM based on AX630 (2×A53 @ 1.2 GHz, 1 GB LPDDR4). Typical delay is about 60 ms at 2K. Two form factors:

- **NanoKVM-Desk** — metal case, 1.47" touchscreen, knob
- **NanoKVM-ATX** — internal bracket, HDMI / network / USB out of the chassis

![NanoKVM-Pro Desktop and ATX](https://wiki.sipeed.com/hardware/assets/NanoKVM/pro/introduce/combine.png)

Frontend and backend of this tree are open; some support/firmware pieces remain closed upstream.

### Specifications

| Product       | NanoKVM-Pro | NanoKVM      | GxxKVM      | JxxKVM      |
|---------------|----------|--------------|-------------|-------------|
| Main Control  | AX630C   | SG2002       | RV1126      | RV1106      |
| Core          | <2xA53@1.2G> | <1xC906@1.0G>  | <4xA7@1.5G>   | <1xA7@1.2G>   |
| Memory        | 1G LPDDR4X | 256M DDR3    | 1G DDR3     | 256M DDR3   |
| Storage       | 32G eMMC | 32G microSD  | 8G eMMC     | 16G eMMC    |
| System        | NanoKVM+PIKVM | NanoKVM      | GxxKVM      | JxxKVM      |
| Resolution    | 4K@45fps | 1080P@60fps | 4K@30fps, 2K@60fps | 1080P@60fps |
| HDMI Loop-Out | 4K Loop-Out | ×            | ×           | ×           |
| Video Encoding | MJPG/H264 | MJPG/H264    | MJPG/H264   | MJPG/H264   |
| Audio Transmission | ✓        | ×            | ✓           | ×           |
| UEFI/BIOS Support | ✓        | ✓            | ✓           | ✓           |
| Simulated USB Keyboard/Mouse | ✓ | ✓          | ✓           | ✓           |
| Simulated USB ISO | ✓        | ✓            | ✓           | ✓           |
| IPMI          | ✓        | ✓            | ✓           | ×           |
| Wake-on-LAN (WOL) | ✓        | ✓            | ✓           | ✓           |
| WebSSH        | ✓        | ✓            | ✓           | ✓           |
| Custom Scripts | ✓        | ✓            | ×           | ×           |
| Serial Terminal | 2 Channels | 2 Channels   | None        | 1 Channel   |
| Storage Performance | 32G eMMC 300MB/s | 32G MicroSD 12MB/s | 8G eMMC 120MB/s | 8G eMMC 60MB/s |
| Ethernet      | 1000M    | 100M         | 1000M       | 100M        |
| Internal Form Factor | Optional ATX version | Optional PCIe version | ×           | ×           |
| WiFi          | Optional WiFi6 | Optional WiFi6 | ×           | ×           |
| MicroSD Expansion | ✓        | ×            | ×           | ×           |
| ATX Power Control | ✓        | ✓            | +15$        | +10$        |
| Display       | 1.47-inch 320x172 LCD<br>0.96-inch 128x64 OLED | 0.96-inch 128x64 OLED | None | 1.66-inch 280x240 |
| Additional Features | Synchronized LED effects, Smart Assistant | –        | –           | –           |
| Power Consumption | 0.6A@5V  | 0.2A@5V      | 0.4A@5V     | 0.2A@5V     |
| Power Input   | USB-C/PoE | USB-C/PoE/PCIe | USB-C       | USB-C       |
| Dimensions     | 65x65x28mm | 40x36x36mm   | 80x60x7.5mm | 60x6x24-30mm |

### Where to buy (official)

- [AliExpress](https://www.aliexpress.com/item/1005010048471263.html)
- [Sipeed product page](https://sipeed.com/nanokvm/pro)

### Upstream community

- [Discord](https://discord.gg/V4sAZ9XWpN)
- QQ group: 703230713
- email: [support@sipeed.com](mailto:support@sipeed.com)
- [FAQ](https://wiki.sipeed.com/hardware/en/kvm/NanoKVM_Pro/faq.html)

## License

GPL-3.0 — see [LICENSE](LICENSE). This fork is not affiliated with Sipeed; use it at your own risk.
