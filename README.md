# debian-network-tui

面向 **Debian 11 / 12 / 13** 的终端网卡配置工具，用 Go 编写，交互风格接近 `nmtui`，直接管理 **ifupdown** 的 `/etc/network/interfaces`（含 `source` / `source-directory`）。

## 功能

- 编辑连接：新建 / 修改 / 删除 iface 配置
- IPv4：`dhcp` / `static` / 禁用
- IPv6：`disabled` / `dhcp` / `static` / `auto`（`manual` + `accept_ra 1`）
- `auto`、`allow-hotplug` 开关
- 静态地址、掩码、网关、`dns-nameservers`
- 激活 / 停用：调用 `ifup` / `ifdown`
- 保存前自动备份为 `/etc/network/interfaces.bak.<时间戳>`

不依赖 NetworkManager，适合服务器与最小化安装。

## 依赖

- Go 1.21+（仅编译时）
- 运行时：`ifupdown`（`ifup`/`ifdown`）、`iproute2`（`ip`）
- root 权限（读写 `/etc/network/interfaces`）

## 下载

从 [GitHub Releases](https://github.com/Songxwn/Debian-network-tui/releases) 下载对应架构的二进制包，例如：

```bash
# amd64 示例
tar -xzf debian-network-tui-v0.1.0-linux-amd64.tar.gz
sudo install -m 755 debian-network-tui-v0.1.0-linux-amd64 /usr/local/bin/debian-network-tui
```

打 `v*` 标签推送后，GitHub Actions 会自动编译并发布 Release。

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
# bin/debian-network-tui-linux-amd64
# bin/debian-network-tui-linux-arm64
```

## 使用

```bash
sudo ./bin/debian-network-tui
```

指定配置文件（测试用）：

```bash
sudo INTERFACES_FILE=/tmp/interfaces ./bin/debian-network-tui
```

### 快捷键

| 界面     | 按键 |
|----------|------|
| 主菜单   | `↑/↓` 选择，`Enter` 确认，`q` 退出 |
| 连接列表 | `a` 新建，`d` 删除，`Enter` 编辑，`Esc` 返回 |
| 编辑表单 | `Tab` 切换字段，`←/→` 改选项，`Ctrl+S` 保存，`Esc` 取消 |
| 确认框   | `y` / `n` |

### 主菜单（对照 nmtui）

1. **编辑连接** — 修改 interfaces 配置  
2. **激活连接** — `ifup <iface>`  
3. **停用连接** — `ifdown <iface>`  
4. **退出**

## 配置示例

保存 static IPv4 后大致会写入：

```
# Managed by debian-network-tui: eth0
allow-hotplug eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1
    dns-nameservers 8.8.8.8 1.1.1.1
```

## 注意

- 修改配置后需在菜单中「激活连接」，或手动 `ifup <iface>` / `systemctl restart networking`
- 远程 SSH 操作时，停用当前网卡可能导致断连
- `lo` 不允许删除
- 写入只更新主配置文件；`interfaces.d/` 中已有连接可查看，编辑会合并进主文件（原 sourced 片段需自行清理）

## 测试

```bash
go test ./...
```

## 许可证

MIT
