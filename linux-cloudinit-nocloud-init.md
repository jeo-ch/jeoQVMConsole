# Linux 虚拟机 cloud-init NoCloud 初始化方案调研与初步实现设计

## 背景

当前项目 Linux 虚拟机克隆初始化采用的是「在线 SSH 初始化」模式：

```
克隆磁盘 → virt-customize 重置身份（离线）→ 启动 VM → 等待 DHCP 获取 IP → SSH 连入 → 执行脚本（hostname/用户/密码/磁盘扩容）
```

该方案存在以下问题：
- 强依赖网络，VM 必须先拿到 DHCP 地址才能继续
- 固定 sleep 30s + SSH 轮询，平均等待 60s 以上
- 需要在模板元数据中保存明文 root 密码
- 并发克隆时 DHCP IP 冲突风险（已有 virt-customize 临时修复）
- 部分模板可能不开放 SSH 或密码方式登录

---

## 方案二：cloud-init NoCloud ISO 注入

### 核心原理（PVE 的做法）

Proxmox VE 官方文档原文：
> "Proxmox VE generates an ISO image to pass the Cloud-Init data to the VM. For that purpose, all Cloud-Init VMs need to have an assigned CD-ROM drive."

本质是利用 cloud-init 的 **NoCloud 数据源**：cloud-init 在首次启动时按优先级扫描可能的数据来源，当检测到一块标签（Volume Label）为 `cidata` 或 `CIDATA` 的 iso9660/vfat 文件系统时，会从中读取配置文件完成自初始化，**整个过程完全不依赖网络**。

### ISO 结构

```
seed.iso（卷标必须是 cidata）
├── meta-data        # 实例基本信息（YAML）
├── user-data        # 初始化配置（cloud-config YAML）
└── network-config   # 网络配置（可选，YAML）
```

### cloud-init 执行流程

```
VM 启动
  ├── cloud-init detect datasource
  │     └── 扫描到 /dev/sr0 或 /dev/vdb 卷标为 cidata
  ├── init-local（无网络阶段）
  │     ├── 读取 meta-data → 设置 instance-id、hostname
  │     └── 读取 network-config → 配置网络接口
  ├── init-network（网络就绪后）
  │     └── 读取 user-data → 创建用户、设置密码、注入 SSH 密钥
  ├── modules:config
  │     └── 执行磁盘扩容（growpart + resize_rootfs）
  └── modules:final
        └── 执行 runcmd 命令列表
```

---

## 文件格式详解

### meta-data（必须）

```yaml
instance-id: iid-vm-name-20240101120000   # 唯一标识，变化才会触发重初始化
local-hostname: my-vm-hostname
```

**关键**：`instance-id` 必须每次克隆都不同，否则 cloud-init 认为不是首次启动，跳过所有配置。

### user-data（必须，以 `#cloud-config` 开头）

```yaml
#cloud-config

# 主机名（与 meta-data 保持一致）
hostname: my-vm-hostname
manage_etc_hosts: true

# 禁止密码过期，允许密码 SSH 登录
chpasswd:
  expire: false
ssh_pwauth: true

# 修改现有用户密码（不新建用户）
chpasswd:
  list: |
    root:NewPassword123
    existinguser:NewPassword123
  expire: false

# 新建用户（适用于 cloud image 基础模板）
users:
  - name: myuser
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    groups: [sudo, adm]
    shell: /bin/bash
    lock_passwd: false
    plain_text_passwd: "NewPassword123"

# 磁盘自动扩容
growpart:
  mode: auto
  devices: ['/']
resize_rootfs: true

# 执行自定义命令
runcmd:
  - hostnamectl set-hostname my-vm-hostname
```

### network-config（可选，用于静态 IP）

```yaml
version: 2
ethernets:
  enp1s0:                 # 网卡名，cloud image 通常是 enp1s0 或 eth0
    dhcp4: true           # DHCP

# 静态 IP 示例
version: 2
ethernets:
  enp1s0:
    addresses:
      - 192.168.100.10/24
    routes:
      - to: default
        via: 192.168.100.1
    nameservers:
      addresses: [8.8.8.8, 114.114.114.114]
```

---

## 宿主机操作流程

### Step 1：生成 seed.iso

工具选择（二选一）：
- `cloud-localds`（推荐，来自 `cloud-image-utils` 包）
- `genisoimage`（更通用）

```bash
# 方式一：cloud-localds（最简洁）
cloud-localds \
  /tmp/vm-name-seed.iso \
  user-data \
  meta-data \
  --network-config network-config   # 可选

# 方式二：genisoimage
genisoimage \
  -output /tmp/vm-name-seed.iso \
  -volid cidata \
  -joliet -rock \
  user-data meta-data
```

### Step 2：将 seed.iso 挂载为 CD-ROM 启动 VM

通过 libvirt XML 添加 CD-ROM 设备：

```xml
<disk type='file' device='cdrom'>
  <driver name='qemu' type='raw'/>
  <source file='/tmp/vm-name-seed.iso'/>
  <target dev='sda' bus='sata'/>
  <readonly/>
</disk>
```

或通过 virt-install 参数：
```bash
--disk path=/tmp/vm-name-seed.iso,device=cdrom
```

### Step 3：等待 cloud-init 完成，摘除 ISO

cloud-init 完成后，ISO 已无价值。可以：
1. **不摘除**：VM 不会重复执行（cloud-init 通过 instance-id 幂等化）
2. **主动摘除**（推荐，更干净）：

```bash
# 弹出 CD-ROM（保留设备槽）
virsh change-media vm-name sda --eject --force
# 删除 seed.iso 文件
rm /tmp/vm-name-seed.iso
```

---

## 与现有方案的兼容性分析

### 前提条件

| 条件 | 说明 |
|------|------|
| 模板内预装 cloud-init | **必须**，否则无法读取 seed.iso |
| 宿主机安装 cloud-localds 或 genisoimage | 生成 ISO 需要 |
| 模板类型标识 | 需要在 `.meta.json` 中增加 `cloud_init_enabled: true` 字段 |

### 两种模式共存策略（兼容方案）

考虑到现有模板大量未安装 cloud-init，建议保留两套路径，通过模板元数据字段切换：

```
TemplateMeta.CloudInitMode = "nocloud" → 使用 seed.iso 初始化
TemplateMeta.CloudInitMode = ""       → 使用现有 SSH 初始化（默认兼容）
```

---

## 初步 Go 代码实现设计

### 1. 新增模板元数据字段

在 `server/service/template/types.go` 的 `TemplateMeta` 结构体中新增：

```go
// CloudInitMode 模板初始化方式: "" 或 "nocloud"
// nocloud = 使用 cloud-init NoCloud ISO 注入方式（无需 SSH）
CloudInitMode string `json:"cloud_init_mode,omitempty"`
```

同步更新 `server/service/clone/deps.go` 中的镜像 `TemplateMeta`。

### 2. 新增 seed.iso 生成函数

新建 `server/service/clone/linux_cloudinit.go`：

```go
package clone

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "kvm_console/utils"
)

// CloudInitSeedConfig cloud-init NoCloud 数据源配置
type CloudInitSeedConfig struct {
    InstanceID    string // 唯一标识（用于幂等化）
    Hostname      string
    Username      string // 目标用户名
    Password      string // 明文密码
    TemplateUser  string // 模板中的原始用户名（用于 chpasswd）
    SSHAuthorizedKeys []string // 可选：注入 SSH 公钥
    NetworkConfig string // 可选：network-config YAML 内容
}

// BuildCloudInitSeedISO 生成 cloud-init NoCloud seed.iso
// 返回 ISO 文件路径，调用方负责在使用后删除
func BuildCloudInitSeedISO(workDir string, cfg CloudInitSeedConfig) (string, error) {
    tmpDir, err := os.MkdirTemp(workDir, "cloudinit-*")
    if err != nil {
        return "", fmt.Errorf("创建临时目录失败: %w", err)
    }
    defer os.RemoveAll(tmpDir)

    // 生成 meta-data
    metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n",
        cfg.InstanceID, cfg.Hostname)
    if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
        return "", fmt.Errorf("写入 meta-data 失败: %w", err)
    }

    // 生成 user-data
    userData := buildCloudInitUserData(cfg)
    if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
        return "", fmt.Errorf("写入 user-data 失败: %w", err)
    }

    isoPath := filepath.Join(workDir, fmt.Sprintf("cloudinit-seed-%d.iso", time.Now().UnixNano()))

    // 写入 network-config（如果有）
    args := []string{isoPath, filepath.Join(tmpDir, "user-data"), filepath.Join(tmpDir, "meta-data")}
    if cfg.NetworkConfig != "" {
        ncPath := filepath.Join(tmpDir, "network-config")
        if err := os.WriteFile(ncPath, []byte(cfg.NetworkConfig), 0644); err != nil {
            return "", fmt.Errorf("写入 network-config 失败: %w", err)
        }
        args = append(args, "--network-config", ncPath)
    }

    // 优先用 cloud-localds，fallback 到 genisoimage
    result := utils.ExecCommand("cloud-localds", args...)
    if result.Error != nil {
        // fallback: genisoimage
        genoArgs := []string{
            "-output", isoPath,
            "-volid", "cidata",
            "-joliet", "-rock",
            filepath.Join(tmpDir, "user-data"),
            filepath.Join(tmpDir, "meta-data"),
        }
        result2 := utils.ExecCommand("genisoimage", genoArgs...)
        if result2.Error != nil {
            return "", fmt.Errorf("生成 cloud-init seed.iso 失败 (cloud-localds: %s; genisoimage: %s)",
                result.Stderr, result2.Stderr)
        }
    }

    return isoPath, nil
}

// buildCloudInitUserData 构造 user-data cloud-config YAML
func buildCloudInitUserData(cfg CloudInitSeedConfig) string {
    var sb strings.Builder
    sb.WriteString("#cloud-config\n\n")

    // hostname
    sb.WriteString(fmt.Sprintf("hostname: %s\n", cfg.Hostname))
    sb.WriteString("manage_etc_hosts: true\n\n")

    // 密码设置
    sb.WriteString("ssh_pwauth: true\n\n")

    // 设置密码（针对模板中已有用户和 root）
    if cfg.Password != "" {
        sb.WriteString("chpasswd:\n  expire: false\n  list: |\n")
        sb.WriteString(fmt.Sprintf("    root:%s\n", cfg.Password))
        if cfg.TemplateUser != "" {
            sb.WriteString(fmt.Sprintf("    %s:%s\n", cfg.TemplateUser, cfg.Password))
        }
        if cfg.Username != "" && cfg.Username != cfg.TemplateUser {
            // 后续通过 runcmd 重命名用户（cloud-init 不直接支持重命名）
            sb.WriteString(fmt.Sprintf("    %s:%s\n", cfg.Username, cfg.Password))
        }
        sb.WriteString("\n")
    }

    // SSH 公钥注入
    if len(cfg.SSHAuthorizedKeys) > 0 {
        sb.WriteString("ssh_authorized_keys:\n")
        for _, key := range cfg.SSHAuthorizedKeys {
            sb.WriteString(fmt.Sprintf("  - %s\n", key))
        }
        sb.WriteString("\n")
    }

    // 磁盘自动扩容
    sb.WriteString("growpart:\n  mode: auto\n  devices: ['/']\n")
    sb.WriteString("resize_rootfs: true\n\n")

    // runcmd：hostname 强制写入 + 用户名重命名
    var runcmds []string
    runcmds = append(runcmds,
        fmt.Sprintf("hostnamectl set-hostname %s 2>/dev/null || true", cfg.Hostname),
    )

    // 用户名重命名（如果目标用户名不同于模板用户）
    if cfg.Username != "" && cfg.TemplateUser != "" && cfg.Username != cfg.TemplateUser {
        runcmds = append(runcmds,
            fmt.Sprintf("usermod -l %s %s 2>/dev/null || true", cfg.Username, cfg.TemplateUser),
            fmt.Sprintf("usermod -d /home/%s -m %s 2>/dev/null || true", cfg.Username, cfg.Username),
            fmt.Sprintf("groupmod -n %s %s 2>/dev/null || true", cfg.Username, cfg.TemplateUser),
            fmt.Sprintf("sed -i 's|%s|%s|g' /etc/sudoers.d/* 2>/dev/null || true", cfg.TemplateUser, cfg.Username),
        )
    }

    if len(runcmds) > 0 {
        sb.WriteString("runcmd:\n")
        for _, cmd := range runcmds {
            sb.WriteString(fmt.Sprintf("  - %s\n", cmd))
        }
    }

    return sb.String()
}
```

### 3. 在 VM XML 中插入 CD-ROM 设备

在 `server/service/clone/xml.go` 或相关 XML 构建函数中，新增辅助函数：

```go
// BuildCloudInitCDROMXML 生成 cloud-init seed.iso 的 CD-ROM 设备 XML 片段
func BuildCloudInitCDROMXML(isoPath string) string {
    return fmt.Sprintf(`
  <disk type='file' device='cdrom'>
    <driver name='qemu' type='raw'/>
    <source file='%s'/>
    <target dev='sda' bus='sata'/>
    <readonly/>
  </disk>`, isoPath)
}
```

### 4. 修改 core.go Linux 克隆主流程

在 `linux` 类型克隆流程中，根据模板元数据的 `CloudInitMode` 字段选择初始化路径：

```go
// 伪代码，展示核心逻辑
if tplType == "linux" && meta.CloudInitMode == "nocloud" {
    // NoCloud 路径
    progressFn(25, "生成 cloud-init seed.iso...")
    seedCfg := CloudInitSeedConfig{
        InstanceID:   fmt.Sprintf("iid-%s-%d", params.Name, time.Now().Unix()),
        Hostname:     params.Hostname,
        Username:     params.User,
        Password:     params.Password,
        TemplateUser: params.TemplateUser,
    }
    seedISO, err := BuildCloudInitSeedISO(os.TempDir(), seedCfg)
    if err != nil {
        return nil, err
    }
    defer os.Remove(seedISO)   // 任务结束后删除临时 ISO

    // 将 CD-ROM 插入 VM XML（在 defineAndStartNonWindowsClone 前）
    params.CloudInitSeedISO = seedISO  // 通过参数传递给 XML 构建函数

    // 启动 VM 后等待 cloud-init 完成（通过 QEMU Guest Agent 或超时）
    // 不再需要 SSH 等待和 InitLinuxClone
    progressFn(90, "等待 cloud-init 完成...")
    time.Sleep(60 * time.Second)  // 初期可用固定等待，后续改为 guest-agent 检测
} else {
    // 原有 SSH 路径（兼容旧模板）
    ...
}
```

---

## cloud-init 完成检测方案

cloud-init 初始化是异步的，有以下几种等待方式：

### 方式一：固定超时等待（最简单，初期可用）

等待固定时间（约 60~90s），适用于绝大多数情况。

### 方式二：通过 QEMU Guest Agent 轮询

在 VM 启动后，通过 QEMU Guest Agent 执行命令检查 cloud-init 状态：

```bash
virsh qemu-agent-command vm-name \
  '{"execute":"guest-exec","arguments":{"path":"cloud-init","arg":["status"],"capture-output":true}}'
```

当输出中包含 `"status": "done"` 时认为完成。

**前提**：模板需安装 `qemu-guest-agent`，且 cloud-init user-data 中启用它。

### 方式三：通过 SSH 检查完成标志（混合方案）

cloud-init 完成后会写入 `/run/cloud-init/result.json`，可通过 SSH 检查。
这是最准确的方式，但仍然依赖 SSH（不过此时 SSH 只用于检测，无需模板密码）。

---

## 模板适配要求

要使用 NoCloud 方式，模板磁盘内必须满足：

| 要求 | Ubuntu Cloud Image | 普通 Ubuntu Server | Debian Cloud Image |
|------|--------------------|-------------------|-------------------|
| cloud-init 已安装 | ✅ 预装 | ❌ 需手动安装 | ✅ 预装 |
| cloud-init 已启用 | ✅ | ❌ | ✅ |
| NoCloud 数据源支持 | ✅ | ✅（安装后） | ✅ |
| growpart 支持 | ✅ cloud-guest-utils | ⚠️ 需安装 | ✅ |
| QEMU Guest Agent | ✅ 可安装 | ⚠️ 需安装 | ✅ 可安装 |

**推荐模板制作流程**（针对 NoCloud 模板）：
```bash
# 在模板机器中执行
apt install -y cloud-init cloud-guest-utils qemu-guest-agent cloud-initramfs-growroot

# 清理 cloud-init 缓存（制作模板前必须执行）
cloud-init clean --logs --seed
rm -rf /var/lib/cloud/*

# 确保 NoCloud 数据源在列表中（通常默认已有）
cat /etc/cloud/cloud.cfg.d/90_dpkg.cfg
```

---

## 注意事项与已知问题

### 1. 用户名重命名的局限性

cloud-init 的 `users` 模块**只能新建用户**，不能重命名已有用户。`usermod -l` 需要通过 `runcmd` 实现，但 `runcmd` 在 `modules:final` 阶段执行，此时用户已经登录过（如果有自动登录）可能失败。

**解决方案**：
- 放弃用户名重命名语义，改为"创建新用户，禁用原用户"
- 或在 `bootcmd`（早于 runcmd，每次启动都执行）中执行重命名，加幂等判断

```yaml
bootcmd:
  - |
    if id olduser &>/dev/null && ! id newuser &>/dev/null; then
      usermod -l newuser olduser
      usermod -d /home/newuser -m newuser
      groupmod -n newuser olduser
    fi
```

### 2. 网卡名称不确定

cloud-init 的 `network-config` 需要指定网卡名（如 `enp1s0`），但不同模板的网卡名可能不同。

**解决方案**：使用 MAC 地址匹配方式（network-config v2 支持）：
```yaml
version: 2
ethernets:
  id0:
    match:
      macaddress: "52:54:00:xx:xx:xx"
    set-name: eth0
    dhcp4: true
```

或者默认使用 DHCP，不指定静态 IP（当前项目也是 DHCP 模式）。

### 3. cloud-init 幂等性

cloud-init 通过 `instance-id` 判断是否首次启动：
- 同一 `instance-id` 重启不会重跑配置
- 更换 seed.iso 且更改 `instance-id` 可强制重跑

这意味着：一旦 VM 用 seed.iso 初始化完成后，**即使 ISO 仍挂载也不会重复执行**，是安全的。

### 4. seed.iso 存储位置

ISO 文件应放在与 VM 磁盘相同的存储路径下，建议：
- 路径：`{cloneDir}/{vmName}-cloudinit-seed.iso`
- VM 删除时同步删除 ISO

---

## 实施路线图

### Phase 1：验证（1~2天）
1. 手动在测试机上用现有 Ubuntu cloud image 走通 seed.iso 注入流程
2. 验证 hostname、密码、growpart 均生效
3. 确认 cloud-init 完成后 seed.iso 可安全摘除

### Phase 2：Go 代码实现（2~3天）
1. 新增 `TemplateMeta.CloudInitMode` 字段
2. 实现 `BuildCloudInitSeedISO()` 函数
3. 修改 `core.go` 支持 nocloud 分支
4. 修改 VM XML 构建，支持附加 CD-ROM

### Phase 3：模板管理 UI（1天）
1. 模板编辑页新增"初始化方式"选项（SSH / NoCloud）
2. NoCloud 模式下不再要求填写模板 root 密码

### Phase 4：清理（1天）
1. VM 启动完成后自动摘除 seed.iso
2. VM 删除时清理残留 ISO 文件

---

## 依赖变更

需要在宿主机安装：
- `cloud-image-utils`（提供 `cloud-localds` 命令）或
- `genisoimage`（提供 `genisoimage` 命令）

两者均可用 apt 安装，应更新到 `docs/dependencies.md`：

```markdown
## cloud-init NoCloud 初始化（可选）

适用于启用了 cloud-init NoCloud 模式的模板：

```bash
# 推荐方式
apt install cloud-image-utils

# 备用方式
apt install genisoimage
```

**说明**：仅当使用"NoCloud 初始化模式"的模板克隆时需要。普通 SSH 初始化模式不依赖此工具。
```

---

## 总结

cloud-init NoCloud 是 PVE 等主流虚拟化平台的标准初始化方式，核心价值在于：

1. **零网络依赖**：不等 DHCP，不等 SSH，扫到 CD-ROM 立即处理
2. **零密码依赖**：无需在元数据中存储模板 root 密码
3. **自带扩容**：`growpart + resize_rootfs` 是 cloud-init 内置功能
4. **幂等安全**：多次重启不会重复执行

适用范围是**预装了 cloud-init 的模板**（Ubuntu/Debian Cloud Image 默认支持，普通安装镜像需手动配置）。

与现有 SSH 初始化方式通过 `TemplateMeta.CloudInitMode` 字段区分，兼容并存，渐进迁移。
