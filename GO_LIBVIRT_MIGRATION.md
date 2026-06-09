# go-libvirt 渐进迁移方案

## 目标

将高频 libvirt 操作从 `virsh` 命令行调用迁移至 DigitalOcean 的 [go-libvirt](https://github.com/digitalocean/go-libvirt) 纯 Go RPC 库，低频操作保持 `virsh` 命令不变。两者混用，逐步过渡。

## 为什么迁移

| 对比维度 | 当前 virsh 方案 | go-libvirt 方案 |
|---------|----------------|-----------------|
| 执行方式 | 每次调用 fork 子进程 | 复用一条 Unix socket RPC 连接 |
| 返回数据 | 字符串解析（正则/awk/grep） | Go 原生类型（struct/int/string） |
| 100 台 VM 列表查询 | ~400 次进程 fork | 2-3 次 RPC 调用 |
| 错误处理 | 解析 stderr 文本 | Go error 类型 |
| 事件通知 | 轮询 `virsh domstate` | 原生 `DomainEventLifecycle` 订阅 |
| CGO 依赖 | 无 | 无（同样纯 Go） |
| 跨平台编译 | 简单 | 同样简单 |

## 操作频率分级

### 高频操作 → 迁移到 go-libvirt

这些操作在每次页面加载、每次资源采集周期（10 秒）中反复调用，迁移收益最大：

| 序号 | 操作 | 当前 virsh 命令 | go-libvirt 对应 API | 调用场景 |
|------|------|---------------|---------------------|---------|
| 1 | 列出所有 VM | `virsh list --all --name` | `ConnectListAllDomains()` | VM 列表页、缓存刷新 |
| 2 | 列出运行中 VM | `virsh list --name --state-running` | `ConnectListAllDomains(flags)` | 资源采集器 |
| 3 | 获取 VM 状态 | `virsh domstate <name>` | `DomainGetInfo()` → `State` 字段 | 列表/详情页 |
| 4 | 获取 VM 基本信息 | `virsh dominfo <name>` | `DomainGetInfo()` | CPU/内存/状态 |
| 5 | 获取 VM XML | `virsh dumpxml --inactive` | `DomainGetXMLDesc()` | 配置解析 |
| 6 | CPU 统计 | `virsh cpu-stats --total` | `DomainGetCPUStats()` | 资源采集器 |
| 7 | 内存统计 | `virsh dommemstat <name>` | `DomainMemoryStats()` | 资源采集器 |
| 8 | 磁盘 I/O | `virsh domblkstat <name> <dev>` | `DomainBlockStats()` | 资源采集器 |
| 9 | 网络 I/O | `virsh domifstat <name> <iface>` | `DomainInterfaceStats()` | 资源采集器 |
| 10 | 获取 VNC 端口 | `virsh vncdisplay <name>` | 解析 XML `DomainGetXMLDesc()` | 详情页 |
| 11 | 获取磁盘列表 | `virsh domblklist <name>` | 解析 XML `DomainGetXMLDesc()` | 详情页 |
| 12 | 获取网卡列表 | `virsh domiflist <name>` | 解析 XML `DomainGetXMLDesc()` | 详情页 |

### 中频操作 → 迁移到 go-libvirt

这些操作在 VM 管理动作时调用，频率中等但收益仍明显：

| 序号 | 操作 | 当前 virsh 命令 | go-libvirt 对应 API |
|------|------|---------------|---------------------|
| 13 | 启动 VM | `virsh start <name>` | `DomainCreate()` |
| 14 | 启动 VM（暂停） | `virsh start --paused <name>` | `DomainCreateWithFlags()` |
| 15 | 正常关机 | `virsh shutdown <name>` | `DomainShutdown()` / `DomainShutdownFlags()` |
| 16 | 强制断电 | `virsh destroy <name>` | `DomainDestroy()` |
| 17 | 重启 | `virsh reboot <name>` | `DomainReboot()` |
| 18 | 硬重置 | `virsh reset <name>` | `DomainReset()` |
| 19 | 开机自启 | `virsh autostart <name> [--disable]` | `DomainSetAutostart()` |
| 20 | 暂停/恢复 | `virsh suspend/resume <name>` | `DomainSuspend()` / `DomainResume()` |
| 21 | 持久化配置 | `virsh define <xml>` | `DomainDefineXML()` |
| 22 | 设置 CPU | `virsh setvcpus ...` | `DomainSetVcpusFlags()` |
| 23 | 设置内存 | `virsh setmem/setmaxmem ...` | `DomainSetMemoryFlags()` / `DomainSetMaxMemory()` |

### 低频操作 → 保留 virsh

这些操作调用频率低、参数复杂、或依赖外部工具，暂时保留 virsh 方式：

| 操作类别 | 说明 | 保留原因 |
|---------|------|---------|
| 快照管理 | snapshot-create/delete/revert/list | 外部快照操作复杂，go-libvirt 支持有限 |
| 虚拟机迁移 | migrate + SSH 远程执行 | 涉及跨主机 SSH、证书、存储迁移等复杂链路 |
| 磁盘热插拔 | attach-disk/detach-disk | 需要配合目标宿主机路径处理 |
| 光盘热插拔 | change-media | 操作简单，频率极低 |
| QEMU Monitor | qemu-monitor-command --hmp | 直接 QMP/HMP 命令，无 RPC 等价 API |
| 虚拟机创建 | virt-install | 非 libvirt API 范畴 |
| 虚拟机删除 | undefine + 磁盘清理 | 需要配合文件系统操作 |
| 模板操作 | qemu-img + virt-sysprep | 非 libvirt API 范畴 |
| 网络/防火墙 | brctl/ovs-vsctl/iptables | 非 libvirt API 范畴 |
| VPC/VLAN 管理 | dnsmasq/ovs 操作 | 非 libvirt API 范畴 |
| 带宽限速 | tc/ovs QoS | 非 libvirt API 范畴 |
| IP 地址获取 | domifaddr + agent/arp/lease | 多层 fallback 逻辑复杂，先保持 virsh |
| XML 元数据读写 | virsh metadata | go-libvirt 文档中未明确覆盖 |

## 架构设计

### 1. 新增 libvirt 连接管理层

创建 `server/service/libvirt_rpc.go`，统一管理 go-libvirt 连接：

```go
package service

import (
    "sync"
    "github.com/digitalocean/go-libvirt"
)

var (
    libvirtConn     *libvirt.Libvirt
    libvirtConnOnce sync.Once
    libvirtConnMu   sync.RWMutex
)

// GetLibvirt 获取 libvirt RPC 连接（单例，自动重连）
func GetLibvirt() (*libvirt.Libvirt, error) {
    // 懒初始化 + 断线重连
}

// CloseLibvirt 关闭连接（程序退出时调用）
func CloseLibvirt() error {
    // ...
}
```

### 2. 新增高频操作封装

创建 `server/service/libvirt_rpc_domain.go`，封装迁移后的高频和中频操作，保持与现有函数签名兼容：

```go
// 使用 go-libvirt 的新实现
func ListVMsRPC(options ...VMListOptions) ([]VmInfo, error) {
    l, err := GetLibvirt()
    // ...
    domains, _, err := l.ConnectListAllDomains(1, flags)
    // 遍历 domains 构建 VmInfo 列表
}

func GetVMStateRPC(name string) (string, error) { ... }
func GetVMInfoRPC(name string) (*DomainInfo, error) { ... }
func GetVMXMLRPC(name string) (string, error) { ... }
func GetVMCPUStatsRPC(name string) (float64, error) { ... }
func GetVMMemoryStatsRPC(name string) (map[string]int64, error) { ... }
func GetVMBlockStatsRPC(name, dev string) (int64, int64, error) { ... }
func GetVMInterfaceStatsRPC(name, iface string) (int64, int64, error) { ... }
func StartVMRPC(name string) error { ... }
func ShutdownVMRPC(name string) error { ... }
func DestroyVMRPC(name string) error { ... }
```

### 3. 现有函数改造方式

在现有 `libvirt.go` 的高频函数中，增加 go-libvirt 调用作为首选路径，virsh 作为 fallback：

```go
// 示例：ListVMs 改造
func ListVMs(options ...VMListOptions) ([]VmInfo, error) {
    // 优先尝试 go-libvirt RPC
    if l, err := GetLibvirt(); err == nil {
        if vms, err := ListVMsRPC(l, options...); err == nil {
            return vms, nil
        }
        log.Printf("[警告] go-libvirt 列表查询失败，降级为 virsh: %v", err)
    }
    // fallback: 继续使用原有 virsh 逻辑
    return ListVMsVirsh(options...)
}
```

### 4. 连接生命周期

```
程序启动 → BootstrapVMCacheFromHost()
         → GetLibvirt() 首次调用时自动连接
         
程序运行 → 所有高频调用复用同一连接
         → 连接断开时自动重连（带退避策略）
         
程序退出 → CloseLibvirt() / defer Disconnect()
```

## 迁移顺序（建议）

### 第一阶段：基础设施（1-2 天）

1. `go get github.com/digitalocean/go-libvirt`
2. 创建 `libvirt_rpc.go`（连接管理 + 自动重连）
3. 创建 `libvirt_rpc_domain.go`（Domain 高频操作封装）
4. 编写单元测试验证 RPC 连接和基本操作

### 第二阶段：只读查询迁移（2-3 天）

5. `ListVMs` → 使用 `ConnectListAllDomains()` 替代 `virsh list --all`
6. `GetVMState` → 使用 `DomainGetInfo()` 替代 `virsh domstate`
7. `GetVMInfo` → 使用 `DomainGetInfo()` 替代 `virsh dominfo`
8. `GetVMXML` → 使用 `DomainGetXMLDesc()` 替代 `virsh dumpxml`
9. 磁盘/网卡列表 → 从 XML 解析替代 `virsh domblklist/domiflist`

### 第三阶段：资源采集迁移（1-2 天）

10. `GetVMStats` → 使用 `DomainGetCPUStats()` + `DomainMemoryStats()` 替代 `virsh cpu-stats/dommemstat`
11. 磁盘 I/O 采集 → 使用 `DomainBlockStats()` 替代 `virsh domblkstat`
12. 网络 I/O 采集 → 使用 `DomainInterfaceStats()` 替代 `virsh domifstat`
13. `collectAllVMStats` 中运行中 VM 列表直接复用 go-libvirt 连接

### 第四阶段：控制操作迁移（2-3 天）

14. `StartVM` / `ShutdownVM` / `DestroyVM` / `RebootVM` / `ResetVM`
15. `SetVMAutostart`
16. `EditVMConfig`（setvcpus / setmem / setmaxmem）

### 第五阶段：事件订阅（可选，1-2 天）

17. 使用 `ConnectDomainEventCallbackRegister()` 订阅虚拟机生命周期事件
18. 替代当前的轮询 `virsh domstate` 来更新 VM 状态缓存

## 注意事项

### 1. go-libvirt API 局限性

go-libvirt 是基于 libvirt RPC 协议的自动生成的绑定，不覆盖：
- `virsh qemu-monitor-command`（直接 QMP 命令）—— **保留 virsh**
- `virsh domifaddr`（Guest Agent 接口地址查询）—— **可能有 `DomainInterfaceAddresses()`，需验证**
- `virsh metadata`（XML 元数据读写）—— **可能需要直接操作 XML**
- `virt-xml` / `virt-install` 等辅助工具 —— **保留 CLI**

### 2. XML 解析

迁移后磁盘列表、网卡列表从 XML 中解析（已有成熟的解析代码），不再调用 `virsh domblklist/domiflist`。需要注意：
- go-libvirt 的 `DomainGetXMLDesc()` 返回的 XML **完全等价**于 `virsh dumpxml`
- 现有的 `ParseVMBootTypeFromDomainXML()` 等解析函数无需修改

### 3. 错误处理与降级

每个使用 go-libvirt 的函数必须保留 virsh fallback：

```
go-libvirt 调用成功 → 返回结果
go-libvirt 调用失败 → 记录日志 → 降级为 virsh → 返回结果
virsh 也失败 → 返回错误
```

### 4. 连接断开恢复

go-libvirt 连接可能因 libvirtd 重启而断开，需要：
- 每次调用前检查连接状态
- 断开时自动重连（最多重试 3 次，间隔 1s/2s/4s）
- 重连失败时触发 fallback

### 5. Domain 对象管理

go-libvirt 中每个 VM 通过 `Domain` 对象（即 UUID）引用。需要转换名称 ↔ UUID：
- `ConnectListAllDomains()` 返回包含 `Name` 和 `UUID` 的结构体
- 可通过 `DomainLookupByName()` 根据名称获取 Domain
- 可在内存中缓存 Name → UUID 映射

### 6. 依赖更新

需要在 `install.sh` 中确保：
- 不再需要 `libvirt-client`（virsh 命令行）作为运行时依赖
- 但如果保留部分 virsh 操作，仍需要 `libvirt-client` 包

## 性能预期

以 50 台 VM、列表页加载为例：

| 指标 | 当前 virsh | go-libvirt | 提升 |
|------|-----------|-----------|------|
| 进程 fork 次数 | ~200+ | 0 | 100% |
| 查询耗时 | ~3-8 秒 | ~0.3-1 秒 | ~5-10x |
| 资源采集（10s 周期） | fork 50+ 进程 | 0 fork | 100% |
| 错误解析失败风险 | 中（文本解析） | 低（类型安全） | - |

## 回退方案

如果 go-libvirt 出现兼容性或稳定性问题：
1. 设置配置项 `use_go_libvirt: false` 全局禁用
2. 所有函数自动回退到 virsh 路径
3. go-libvirt 作为可选增强，不影响核心功能
