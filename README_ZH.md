# debian-network-tui

面向 **Debian 11 / 12 / 13** 的终端网卡管理工具，用 Go 编写，交互类似 `nmtui`，直接管理 **ifupdown** 的 `/etc/network/interfaces`（含 `source` / `source-directory`）。

界面文案为英文，便于在无中文字体的最小化系统上正常显示。

[English README](README.md)

## 截图

### 主菜单

![主菜单](docs/main-menu.png)

### 创建 Bond（勾选已 UP 网卡作为 slave）

![创建 Bond](docs/add-bond.png)

## 功能

- 列出**全部系统网卡**（不仅限于配置文件中已有项）
- 编辑连接：新建 / 修改 / 删除 iface
- 连接类型：**Ethernet**、**VLAN**、**Bond**
- 可在以太网或 **Bond 上建 VLAN**（如 `bond0.100`）
- Bond：从已 UP 网卡勾选 slaves，支持模式 / miimon / LACP；自动为 slave 写入 `manual` + `bond-master`
- VLAN：从已 UP 网卡/bond 选择 Parent，VLAN ID 范围 **2–4094**
- IPv4：`dhcp` / `static` / `manual` / disabled（新建默认 disabled）
- IPv6：`disabled` / `dhcp` / `static` / `auto`
- 独立编辑 DNS：`/etc/resolv.conf`
- `auto`、`allow-hotplug`
- 激活 / 停用：`ifup` / `ifdown`
- 重启网络服务
- 清空全部网卡配置（保留 `lo`，先备份）
- 本地安装 `ifenslave` / `vlan` 的 `.deb`
- 清空 / 应用 apt 源（从程序目录读取配置文件）
- 配置 SSH：安装 openssh-server、允许 root 密钥登录、导入公钥
- 保存前自动备份；空闲 **65 秒**无操作自动退出

不依赖 NetworkManager，适合服务器与最小化安装。

## 依赖

- 编译：Go 1.21+
- 运行：`ifupdown`、`iproute2`
- 需要 root 权限

## 下载

从 [GitHub Releases](https://github.com/Songxwn/Debian-network-tui/releases) 下载：

```bash
# amd64 示例（版本号按 Release 实际修改）
tar -xzf debian-network-tui-v0.3.3-linux-amd64.tar.gz
sudo install -m 755 debian-network-tui-v0.3.3-linux-amd64 /usr/local/bin/debian-network-tui
```

推送 `v*` 标签后，GitHub Actions 会自动编译并发布。

## 编译

```bash
git clone https://github.com/Songxwn/Debian-network-tui.git
cd Debian-network-tui
go mod tidy
make build
# 产物: bin/debian-network-tui
```

交叉编译：

```bash
make cross
```

## 使用

```bash
sudo debian-network-tui
```

测试用配置路径：

```bash
sudo INTERFACES_FILE=/tmp/interfaces debian-network-tui
sudo RESOLV_CONF=/tmp/resolv.conf debian-network-tui
```

空闲超时默认 65 秒，可用环境变量调整（`0` 关闭）：

```bash
sudo IDLE_TIMEOUT_SEC=120 debian-network-tui
```

### 快捷键

| 界面 | 按键 |
|------|------|
| 主菜单 | `↑/↓` 选择，`Enter` 确认，`q` 退出 |
| 连接列表 | `a` 新建，`d` 删除，`Enter` 编辑，`Esc` 返回 |
| 编辑表单 | `Tab` 切换，`Space` 勾选（VLAN Parent / Bond slaves），`←/→` 改选项，`Ctrl+S` 保存，`Esc` 取消 |
| 确认框 | `y` / `n` |

### 主菜单

1. **Edit a connection** — 编辑网卡配置
2. **Edit DNS (/etc/resolv.conf)** — 编辑 DNS
3. **Activate a connection** — `ifup`
4. **Deactivate a connection** — `ifdown`
5. **Restart networking** — 重启网络服务
6. **Clear all connections** — 清空网卡配置（保留 lo）
7. **Install ifenslave/vlan (.deb)** — 安装本地 deb
8. **Clear apt sources** — 清空 apt 源
9. **Apply apt sources from file** — 从文件应用 apt 源
10. **Configure SSH server (root key)** — 配置 SSH 与 root 密钥登录
11. **Quit** — 退出

### 本地文件约定（与二进制同目录）

**Bond/VLAN 包（菜单 7）：**

- `ifenslave_*.deb`
- `vlan_*.deb`

**APT 源（菜单 8–9）：**

- `sources.list` 或 `apt-sources.list` → `/etc/apt/sources.list`
- `*.list` / `*.sources` → `/etc/apt/sources.list.d/`
- 示例：`examples/sources.list`

**SSH（菜单 10）：**

- `openssh-server_*.deb`（可选，没有则走 apt）
- `ssh-root.conf`（可选）：

```ini
PubkeyFile=root.pub
```

- `root.pub`：OpenSSH 公钥内容  
  示例：`examples/ssh-root.conf`、`examples/root.pub`

## 配置示例：Bond + VLAN

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

需要软件包：

```bash
sudo apt-get install -y ifenslave vlan
```

也可在菜单中用本地 `.deb` 安装。

## 注意事项

- 改配置后请使用「激活连接」或「重启网络服务」
- 远程 SSH 下停用当前网卡可能导致断连
- 不能删除 `lo`
- 写入主要更新主配置文件；仅存在于 `interfaces.d/` 的条目编辑后会合并进主文件
- DNS 与网卡配置分离，请用「Edit DNS」修改 `/etc/resolv.conf`
- SSH 会设置 `PermitRootLogin prohibit-password`（仅允许密钥登录 root）

## 测试

```bash
go test ./...
```

## 许可证

MIT
