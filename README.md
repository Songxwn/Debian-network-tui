# debian-network-tui

A terminal UI for managing `/etc/network/interfaces` on **Debian 11 / 12 / 13**.
Written in Go, with an **nmtui-like** workflow for **ifupdown** (including `source` / `source-directory`).

All UI text is English so it works on minimal installs without CJK fonts.

## Features

- Lists **all system interfaces** (not only ones already in the config file)
- Edit connections: add / modify / delete iface stanzas
- Connection types: **Ethernet**, **VLAN**, **Bond**
- VLAN on ethernet **or on bond** (`bond0.100` with `vlan-raw-device bond0`)
- Bond: slaves, mode (802.3ad / active-backup / …), miimon, LACP rate; slaves auto-written as `manual` + `bond-master`
- IPv4: `dhcp` / `static` / `manual` / disabled
- IPv6: `disabled` / `dhcp` / `static` / `auto` (`manual` + `accept_ra 1`)
- `auto` and `allow-hotplug` toggles
- Activate / deactivate via `ifup` / `ifdown`
- Automatic backup before save

No NetworkManager dependency — suitable for servers and minimal installs.

## Requirements

- Go 1.21+ (build time only)
- Runtime: `ifupdown` (`ifup`/`ifdown`), `iproute2` (`ip`)
- Root privileges (read/write `/etc/network/interfaces`)

## Download

Get binaries from [GitHub Releases](https://github.com/Songxwn/Debian-network-tui/releases):

```bash
# amd64 example
tar -xzf debian-network-tui-v0.2.0-linux-amd64.tar.gz
sudo install -m 755 debian-network-tui-v0.2.0-linux-amd64 /usr/local/bin/debian-network-tui
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

Idle timeout: exits after **65 seconds** without keyboard input.
Override with `IDLE_TIMEOUT_SEC` (set `0` to disable):

```bash
sudo IDLE_TIMEOUT_SEC=60 debian-network-tui
```

### Key bindings

| Screen       | Keys |
|--------------|------|
| Main menu    | `Up/Down` select, `Enter` confirm, `q` quit |
| Connection list | `a` add, `d` delete, `Enter` edit, `Esc` back |
| Edit form    | `Tab` next field, `Space` toggle (bond slaves / options), `Left/Right` change, `Ctrl+S` save, `Esc` cancel |
| Confirm      | `y` / `n` |

### Main menu (nmtui-like)

1. **Edit a connection** — edit interfaces config
2. **Activate a connection** — `ifup <iface>`
3. **Deactivate a connection** — `ifdown <iface>`
4. **Restart networking** — `systemctl restart networking`
5. **Clear all connections** — wipe all ifaces except `lo` (backs up first)
6. **Quit**

## Example: bond + VLAN

```
auto bond0
iface bond0 inet manual
    bond-slaves eth0 eth1
    bond-mode 802.3ad
    bond-miimon 100
    bond-lacp-rate fast

auto eth0
iface eth0 inet manual
    bond-master bond0

auto eth1
iface eth1 inet manual
    bond-master bond0

allow-hotplug bond0.100
iface bond0.100 inet static
    vlan-raw-device bond0
    vlan_id 100
    address 10.10.10.2
    netmask 255.255.255.0
    gateway 10.10.10.1
```

Requires `ifenslave` (bonding) and `vlan` packages on Debian:

```bash
sudo apt-get install -y ifenslave vlan
```

## Example config (ethernet)

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
