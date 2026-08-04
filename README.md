# debian-network-tui

A terminal UI for managing `/etc/network/interfaces` on **Debian 11 / 12 / 13**.
Written in Go, with an **nmtui-like** workflow for **ifupdown** (including `source` / `source-directory`).

All UI text is English so it works on minimal installs without CJK fonts.

## Features

- Edit connections: add / modify / delete iface stanzas
- IPv4: `dhcp` / `static` / disabled
- IPv6: `disabled` / `dhcp` / `static` / `auto` (`manual` + `accept_ra 1`)
- `auto` and `allow-hotplug` toggles
- Static address, netmask, gateway, `dns-nameservers`
- Activate / deactivate via `ifup` / `ifdown`
- Automatic backup to `/etc/network/interfaces.bak.<timestamp>` before save

No NetworkManager dependency — suitable for servers and minimal installs.

## Requirements

- Go 1.21+ (build time only)
- Runtime: `ifupdown` (`ifup`/`ifdown`), `iproute2` (`ip`)
- Root privileges (read/write `/etc/network/interfaces`)

## Download

Get binaries from [GitHub Releases](https://github.com/Songxwn/Debian-network-tui/releases):

```bash
# amd64 example
tar -xzf debian-network-tui-v0.1.1-linux-amd64.tar.gz
sudo install -m 755 debian-network-tui-v0.1.1-linux-amd64 /usr/local/bin/debian-network-tui
```

Pushing a `v*` tag triggers GitHub Actions to build and publish a Release.

## Build

```bash
git clone https://github.com/Songxwn/Debian-network-tui.git
cd Debian-network-tui
go mod tidy
make build
# output: bin/debian-network-tui
```

Cross-compile:

```bash
make cross
# bin/debian-network-tui-linux-amd64
# bin/debian-network-tui-linux-arm64
```

## Usage

```bash
sudo debian-network-tui
```

Override config path (for testing):

```bash
sudo INTERFACES_FILE=/tmp/interfaces debian-network-tui
```

### Key bindings

| Screen       | Keys |
|--------------|------|
| Main menu    | `Up/Down` select, `Enter` confirm, `q` quit |
| Connection list | `a` add, `d` delete, `Enter` edit, `Esc` back |
| Edit form    | `Tab` next field, `Left/Right` toggle, `Ctrl+S` save, `Esc` cancel |
| Confirm      | `y` / `n` |

### Main menu (nmtui-like)

1. **Edit a connection** — edit interfaces config
2. **Activate a connection** — `ifup <iface>`
3. **Deactivate a connection** — `ifdown <iface>`
4. **Quit**

## Example config

After saving a static IPv4 connection:

```
# Managed by debian-network-tui: eth0
allow-hotplug eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1
    dns-nameservers 8.8.8.8 1.1.1.1
```

## Notes

- After editing, use **Activate a connection**, or run `ifup <iface>` / `systemctl restart networking`
- Deactivating the active NIC over SSH may disconnect you
- `lo` cannot be deleted
- Writes update the main config file; connections that only exist under `interfaces.d/` are visible, but edits are merged into the main file (clean up old sourced snippets manually if needed)

## Test

```bash
go test ./...
```

## License

MIT
