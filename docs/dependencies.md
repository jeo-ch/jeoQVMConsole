# QVMConsole 宿主机依赖包清单

本文档汇总了 QVMConsole 项目运行所需的宿主机 apt 依赖包。

## 应用层依赖补充

除宿主机 apt 依赖外，当前版本新增了以下项目内依赖：

| 依赖 | 位置 | 用途 |
|------|------|------|
| `github.com/pquerna/otp` | `server/go.mod` | 生成与校验 TOTP 2FA 动态验证码 |
| `qrcode` | `web/package.json` | 前端将 `otpauth_url` 生成为二维码，供用户绑定 2FA |

说明：

- SMTP 发信能力使用 Go 标准库与已有项目代码实现，没有新增宿主机 apt 依赖
- 邀请注册、邮箱找回密码、高风险邮箱验证都依赖已配置好的 SMTP 服务
- 动态内存功能不新增 apt 或第三方工具依赖，使用已有 `libvirt` / `virsh` / QEMU virtio balloon 能力
- VPC 网络、安全组、交换机总带宽和 ACL 功能未新增宿主机依赖，复用已有 `openvswitch-switch`、`dnsmasq-base`、`nftables` 和 `iproute2`
- 公网 IP / 浮动 IP 功能未新增宿主机 apt 依赖，复用已有 `iptables`、`iproute2`、`openvswitch-switch`、`nftables` 和 `conntrack`
- VM 网络抓包诊断新增宿主机 apt 依赖 `tcpdump`
- 轻量云 VM 注册确认流程未新增宿主机 apt 依赖或前后端第三方库，复用已有模板克隆、任务队列、SMTP、VPC 和安全组能力；轻量云 VM 网卡上行整形复用 `iproute2` 的 `tc` / `ip`、Linux 内核 IFB 模块和 `fq_codel` 队列算法
- 跨节点虚拟机迁移新增宿主机 apt 依赖 `rsync`，并复用 `sshpass`、`openssh-client`、`libvirt-clients`、`qemu-utils`；热迁移评估会可选检测 `kvm_stat`，检测不到时仅跳过 `kvm_page_fault` 辅助指标
- 本机虚拟机硬盘迁移不新增 apt 依赖，复用已有 `virsh`、`qemu-img` 和 `coreutils`
- UEFI 内存快照 NVRAM 兼容处理不新增 apt 依赖，复用已有 `qemu-img`、`virsh` 和 `ovmf`
- 自定义宿主机存储池和模板 backing 盘的 libvirt / AppArmor 访问规则不新增 apt 依赖，复用系统已有 `apparmor_parser`；未启用 AppArmor 时会自动跳过
- CPU 亲和性（vCPU pinning）不新增 apt 依赖，复用 `coreutils` 提供的 `nproc` 命令获取系统 CPU 核心数，通过已有的 `virsh vcpupin` 命令实现绑定
- 宿主机内存检查（创建虚拟机前校验可用内存）不新增 apt 依赖，通过读取 `/proc/meminfo` 的 `MemAvailable` 实现，回退使用 `free` 命令
- 虚拟机强制删除（僵尸虚拟机清理）不新增 apt 依赖，复用已有 `virsh`、`systemctl` 和 `coreutils` 命令
- 虚拟机删除磁盘容错（磁盘文件缺失时跳过快照清理）不新增 apt 依赖，通过 `test -f` 命令检查文件存在性
- **Linux 克隆离线初始化（NoCloud 方案）**新增对 `libguestfs-tools`（`virt-customize`）的依赖；`virt-customize --password` 用于离线修改密码，加上 `cloud-init` NoCloud seed 文件注入取代旧版本的 SSH 在线初始化方案；模板内预装 `cloud-init` 可获得首次启动自动扩容能力

## 一键安装

```bash
apt-get update
apt-get install -y \
  ca-certificates \
  curl \
  tar \
  gzip \
  qemu-system-x86 \
  qemu-utils \
  libvirt-daemon-system \
  libvirt-daemon-driver-qemu \
  libvirt-clients \
  openvswitch-switch \
  dnsmasq-base \
  virtinst \
  libguestfs-tools \
  ntfs-3g \
  sshpass \
  rsync \
  cloud-image-utils \
  ovmf \
  lvm2 \
  cloud-guest-utils \
  quota \
  e2fsprogs \
  util-linux \
  nftables \
  iproute2 \
  iptables \
  tcpdump \
  ufw \
  nmap \
  arp-scan \
  conntrack \
  openssh-client \
  openssh-server
```

## 依赖说明

### 核心虚拟化

| 包名 | 提供的命令 | 用途 |
|------|-----------|------|
| `qemu-system-x86` | `qemu-system-x86_64` | KVM 虚拟化引擎 |
| `qemu-utils` | `qemu-img` | 磁盘镜像创建、克隆、扩容、信息查询与格式转换 |
| `libvirt-daemon-system` / `libvirt-daemon-driver-qemu` | `libvirtd` | libvirt 守护进程与 QEMU 驱动，管理虚拟机生命周期 |
| `libvirt-clients` | `virsh` | 虚拟机管理 CLI，用于创建/启动/关机/快照/网络管理等全部操作 |
| `openvswitch-switch` | `ovs-vsctl`, `ovs-ofctl`, `ovs-vswitchd` | 创建独立 OVS 网桥 `br-ovs`，虚拟机通过 Open vSwitch bridge 接入内网，并通过 OVS QoS/Queue 与 OpenFlow meter 对外网流量限速 |
| `dnsmasq-base` | `dnsmasq` | OVS 与 VPC 网络 DHCP 服务 |
| `virtinst` | `virt-install` | 创建虚拟机定义（普通创建和模板克隆均使用） |

### 模板与克隆

| 包名 | 提供的命令 | 用途 |
|------|-----------|------|
| `libguestfs-tools` | `virt-filesystems`, `virt-customize`, `guestfish`, `virt-win-reg`, `ntfsclone`, `ntfsfix`, `ntfsresize` | 检测/修改模板与克隆磁盘；Windows 克隆扩容时移动恢复分区并扩展 NTFS；FnOS 克隆时离线扩展 ext 系统分区 |
| `ntfs-3g` | `ntfsfix`, `ntfsresize`, `ntfsclone` | 不同发行版中 NTFS 工具可能由该包补充提供 |
| `sshpass` | `sshpass` | 克隆后 SSH 自动登录虚拟机执行初始化（设置 hostname、用户名、密码、磁盘扩容） |
| `ovmf` | UEFI 固件文件 | 为 UEFI 启动的虚拟机提供固件支持（`/usr/share/OVMF/`） |

### 虚拟机内磁盘扩容（SSH 初始化时使用）

| 包名 | 提供的命令 | 用途 |
|------|-----------|------|
| `cloud-guest-utils` | `growpart` | 扩展磁盘分区（克隆后磁盘扩容时在 VM 内执行） |
| `parted` | `parted` | 当模板内缺少 `growpart` 时作为分区扩容回退工具 |
| `util-linux` | `sfdisk`, `partx`, `blockdev` | 当模板内缺少 `growpart` 和 `parted` 时作为分区扩容回退工具 |
| `lvm2` | `pvresize`, `lvextend`, `pvs`, `lvs`, `vgs` | LVM 逻辑卷扩容（如果 VM 使用 LVM 分区） |

> **注意**：`growpart`、`resize2fs`、`xfs_growfs`、`pvresize`、`lvextend` 等命令是通过 SSH 在虚拟机内部执行的，需要在**模板镜像内**预装这些工具。Debian 13 等 Linux 模板还需要在模板内预装 `sudo`、`openssh-server`、`cloud-guest-utils`、`e2fsprogs` 和 `lvm2`，并确保模板登录用户具备 sudo 权限。若模板没有 `sudo`，系统会尝试使用同一模板密码通过 `su - root` 提权；若模板没有 `growpart`，系统会依次尝试 `parted` 和 `sfdisk` 回退扩容分区。宿主机安装 `cloud-guest-utils` 和 `lvm2` 是为了在需要时也可用。

### 用户存储配额

| 包名 | 提供的命令 | 用途 |
|------|-----------|------|
| `quota` | `setquota`, `repquota`, `quotacheck`, `quotaon` | Linux 文件系统配额管理，用于限制用户存储空间 |
| `e2fsprogs` | `mkfs.ext4`, `chattr` | 创建 ext4 project quota 文件系统，并给目录设置 project ID |
| `nftables` | `nft` | KVM 全局网络防火墙，生成独立 `table inet kvm_console_fw` 管控虚拟机入站/出站区域策略 |
| `iproute2` | `tc`, `ip` | VM/容器带宽限制、网络接口和路由检测；Ubuntu 默认通常已安装 |
| `iptables` | `iptables` | OVS/VPC NAT、端口转发和兼容转发规则 |
| `tcpdump` | `tcpdump` | 管理员在 VM 网络诊断中对运行态 vnet/tap 接口做临时抓包，并生成 pcap 文件 |
| `nmap` | `nmap` | 桥接模式虚拟机 IP 发现：通过 ping 扫描（ARP 探测）填充宿主机 ARP 表 |
| `arp-scan` | `arp-scan` | 桥接模式虚拟机 IP 发现：直接发送 ARP 请求扫描子网，比 nmap 更快更精准 |
| `ufw` | `ufw` | 宿主机防火墙控制，管理 SSH、面板服务、VNC、端口转发和自定义入站规则 |
| `conntrack` | `conntrack` | 连接清理辅助工具，未安装时连接关闭功能会退化为 `ss -K` 尝试关闭 TCP socket |
| `dnsmasq-base` | `dnsmasq` | OVS 内网 DHCP 租约与静态 IP 分配；通常由系统或 libvirt 依赖安装，缺失时需补装 |
| `openssh-client` / `openssh-server` | `ssh`, `ssh-keygen`, `sshd` | 克隆初始化 SSH 连接、known_hosts 管理和面板用户 SSH 开关 |
| `util-linux` | `lsblk`, `findmnt`, `blkid`, `wipefs`, `mount`, `zramctl`, `mkswap`, `swapon`, `swapoff` | 宿主机物理硬盘存储池识别、格式化前清理文件系统标记、挂载和开机自启配置；zRAM 挡位创建压缩 swap；多数发行版默认已安装 |

> **注意**：安装脚本会自动创建专用存储文件系统（ext4 + project quota），无需手动修改 `/etc/fstab` 或根分区配置。

### 可选/辅助

| 包名 | 提供的命令 | 用途 |
|------|-----------|------|
| `cloud-image-utils` | `cloud-localds` | 用于制作 cloud-init 数据盘（当前代码未直接使用，预留） |
| `ca-certificates` / `curl` / `tar` / `gzip` | `curl`, `tar`, `gzip` | 在线下载 GitHub Release 与解压发行包 |
| `polkitd` 或 `policykit-1` | `pkaction`, `polkit.service` | 用户级 libvirt 授权规则；安装脚本会按发行版可用包名尝试补装 |
| `linux-tools-generic` | `kvm_stat` | 可选辅助工具；若发行版提供 `kvm_stat`，热迁移预检会读取 `kvm_page_fault` 辅助指标；检测不到时核心判断仍以 libvirt dirty-rate 为准 |

## 代码中使用的外部命令汇总

| 命令 | 来源包 | 使用场景 |
|------|-------|---------|
| `virsh` | `libvirt-clients` | 虚拟机全部管理操作：启动/关机/快照/网络/存储池/磁盘等 |
| `ovs-vsctl` | `openvswitch-switch` | OVS 网桥创建、端口验证与状态检查 |
| `ovs-ofctl` | `openvswitch-switch` | 写入 OVS OpenFlow flow/meter，按外网流量导入对应限速路径 |
| `dnsmasq` | `dnsmasq-base` | OVS 内网 DHCP 服务，读取 `/etc/kvm-console/ovs/dhcp-hosts` 并写入租约 |
| `virt-install` | `virtinst` | 创建虚拟机（普通创建 + 模板克隆） |
| `qemu-img` | `qemu-utils` | 磁盘镜像操作：create/resize/info/convert |
| `virt-filesystems` | `libguestfs-tools` | UEFI 自动检测（扫描模板分区类型）与 Windows 克隆扩容前的分区识别 |
| `guestfish` | `libguestfs-tools` | Windows 克隆扩容时离线调整 GPT 分区表、移动恢复分区 |
| `ntfsclone` / `ntfsfix` / `ntfsresize` | `libguestfs-tools` | Windows 恢复分区搬迁、恢复分区 NTFS 标记修复、系统分区 NTFS 扩容 |
| `virt-win-reg` | `libguestfs-tools` | Windows 离线注册表修改工具，预留用于 Windows 模板初始化兼容处理 |
| `sshpass` | `sshpass` | 克隆后 SSH 自动登录执行初始化 |
| `rsync` | `rsync` | 跨节点迁移时复制 VM overlay 磁盘和 XML 临时文件，保留稀疏文件特性 |
| `kvm_stat` | `linux-tools-generic` 或发行版对应 linux-tools 包 | 可选读取热迁移 `kvm_page_fault` 辅助指标；检测不到时核心判断仍以 libvirt dirty-rate 为准 |
| `ssh` / `ssh-keygen` | `openssh-client` | SSH 连接和 known_hosts 管理（系统自带） |
| `nproc` | `coreutils` | 获取系统 CPU 核心总数，用于 CPU 亲和性校验（系统自带） |
| `cp` | `coreutils` | Other 类型模板直接复制磁盘（系统自带） |
| `du` / `ls` / `cat` / `rm` / `mkdir` | `coreutils` | 基础文件操作（系统自带） |
| `sed` / `grep` / `awk` | `sed` / `grep` / `gawk` | 文本处理（系统自带） |
| `setquota` | `quota` | 设置/更新用户文件系统存储配额 |
| `repquota` | `quota` | 查询用户文件系统配额使用情况 |
| `lsblk` / `findmnt` / `blkid` / `wipefs` / `mount` / `zramctl` / `mkswap` / `swapon` / `swapoff` | `util-linux` | 宿主机物理硬盘存储池列表、UUID 查询、格式化前清理、挂载；zRAM 设备创建、初始化与启停 |
| `nft` | `nftables` | 校验、应用、回滚 KVM 网络防火墙规则 |
| `ufw` | `ufw` | 宿主机防火墙启停、规则增删改查和端口转发外层放通 |
| `conntrack` | `conntrack` | 连接清理辅助；当前实现优先保障 `ss -K` 可用，安装后便于后续扩展清理 NAT/转发连接 |
| `tc` | `iproute2` | 清理旧版 VM 端口限速规则、容器网络速率限制 |
| `tcpdump` | `tcpdump` | VM 网络抓包诊断，支持文本摘要和 pcap 下载 |
| `nmap` | `nmap` | 桥接模式 VM IP 发现：ping 扫描填充宿主 ARP 表 |
| `arp-scan` | `arp-scan` | 桥接模式 VM IP 发现：ARP 请求扫描子网

## GeoIP 数据源

网络防火墙支持本地导入 CIDR，也支持在线下载国家/地区 IPv4 CIDR。默认在线源为 IPdeny 聚合 zone 文件：

```text
https://www.ipdeny.com/ipblocks/data/aggregated/{code}-aggregated.zone
```

使用在线源时请遵守 IPdeny 的版权和使用限制。生产环境无法访问外网时，可以只使用本地导入模式。

## 验证安装

安装完成后可用以下命令验证：

```bash
# 验证核心组件
virsh version
qemu-img --version
virt-install --version
virt-filesystems --version
sshpass -V
ovs-vsctl --version
dnsmasq --version
nft --version
iptables --version
tcpdump --version
setquota --version
repquota --version

# 验证 libvirt 服务运行状态
systemctl status libvirtd

# 验证 UEFI 固件
ls /usr/share/OVMF/OVMF_CODE_4M.fd

```
### 桥接直通网桥

桥接直通网桥功能未新增 apt 依赖，复用已有系统工具：

- `openvswitch-switch`：创建和管理 OVS 网桥、端口、meter。
- `iproute2`：读取和迁移网卡地址、默认路由、链路状态。
- `systemd`：通过 `kvm-console-bridges.service` 恢复面板管理的桥接网桥。

### 硬件直通 (PCI Passthrough)

硬件直通功能未新增 apt 依赖，复用已有内核模块：

- `vfio-pci`：PCI 设备直通内核模块，需要在内核启动参数中启用 IOMMU（`intel_iommu=on iommu=pt` 或 `amd_iommu=on iommu=pt`）。
- `virsh nodedev-list`：libvirt 提供的 PCI 设备枚举能力。
- `lspci`：系统自带的 PCI 设备列表工具，用于获取设备名称和厂商信息。

> **注意**：硬件直通需要满足：
> 1. 宿主机 BIOS 中启用 VT-d（Intel）或 AMD-Vi（AMD）
> 2. 内核启动参数添加 IOMMU 支持
> 3. IOMMU 组正确隔离（直通设备所在组不包含系统关键设备）
> 4. 设备已绑定到 vfio-pci 驱动（面板会自动处理）
