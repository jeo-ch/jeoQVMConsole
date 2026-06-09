package service

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"
)

// ==================== vCPU 标志常量 ====================
// go-libvirt 未定义 vcpu flags 常量，此处按 libvirt C 头文件补充

const (
	domainVcpuLive       uint32 = 1 // VIR_DOMAIN_VCPU_LIVE
	domainVcpuConfig     uint32 = 2 // VIR_DOMAIN_VCPU_CONFIG
	domainVcpuMaximum    uint32 = 4 // VIR_DOMAIN_VCPU_MAXIMUM
	domainVcpuCurrent    uint32 = 8 // VIR_DOMAIN_VCPU_CURRENT
	domainVcpuHotpluggable uint32 = 16 // VIR_DOMAIN_VCPU_HOTPLUGGABLE
	domainVcpuGuest      uint32 = 32 // VIR_DOMAIN_VCPU_GUEST
)

// ==================== 内存标志常量 ====================
// go-libvirt 未定义内存 flags 常量，此处按 libvirt C 头文件补充

const (
	domainMemLive    libvirt.DomainMemoryModFlags = 1 // VIR_DOMAIN_AFFECT_LIVE
	domainMemConfig  libvirt.DomainMemoryModFlags = 2 // VIR_DOMAIN_AFFECT_CONFIG
	domainMemMaximum libvirt.DomainMemoryModFlags = 4 // VIR_DOMAIN_MEM_MAXIMUM
)

// ==================== 状态映射 ====================

// domainStateToString 将 libvirt 状态码映射为与 virsh 输出一致的字符串
// 0=nostate, 1=running, 2=blocked, 3=paused, 4=shutdown, 5=shutoff, 6=crashed, 7=pmsuspended
func domainStateToString(state libvirt.DomainState) string {
	switch state {
	case libvirt.DomainNostate:
		return "no state"
	case libvirt.DomainRunning:
		return "running"
	case libvirt.DomainBlocked:
		return "blocked"
	case libvirt.DomainPaused:
		return "paused"
	case libvirt.DomainShutdown:
		return "shutdown"
	case libvirt.DomainShutoff:
		return "shut off"
	case libvirt.DomainCrashed:
		return "crashed"
	case libvirt.DomainPmsuspended:
		return "pmsuspended"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}

// ==================== 辅助函数 ====================

// lookupDomainByName 通过名称查找 Domain 对象
func lookupDomainByName(name string) (libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("查找域 %s 失败: %w", name, err)
	}
	dom, err := l.DomainLookupByName(name)
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("域 %s 不存在: %w", name, err)
	}
	return dom, nil
}

// ==================== 高频只读操作 ====================

// listAllDomainsRPC 列出所有 VM（替代 virsh list --all --name）
func listAllDomainsRPC() ([]libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域列表失败: %w", err)
	}
	// NeedResults=1 表示要返回结果, flags=0 表示所有状态（等价于 Active|Inactive）
	domains, _, err := l.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("列出所有域失败: %w", err)
	}
	return domains, nil
}

// getDomainStateRPC 获取 VM 状态字符串（替代 virsh domstate）
func getDomainStateRPC(name string) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("获取域 %s 状态失败: %w", name, err)
	}
	state, _, _, _, _, err := l.DomainGetInfo(dom)
	if err != nil {
		return "", fmt.Errorf("获取域 %s 信息失败: %w", name, err)
	}
	return domainStateToString(libvirt.DomainState(state)), nil
}

// getDomainInfoRPC 获取 VM 基本信息（替代 virsh dominfo）
// 返回 vcpu数, 最大内存KB, 已用内存KB, 是否自动启动
func getDomainInfoRPC(name string) (vcpu int, maxMemKB uint64, usedMemKB uint64, autostart bool, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, 0, false, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("获取域 %s 信息失败: %w", name, err)
	}

	// 获取 vcpu / 内存信息
	state, maxMem, memory, nrVirtCPU, _, infoErr := l.DomainGetInfo(dom)
	if infoErr != nil {
		return 0, 0, 0, false, fmt.Errorf("获取域 %s 基础信息失败: %w", name, infoErr)
	}
	_ = state // 状态由 getDomainStateRPC 单独获取

	// 获取自动启动状态
	autoVal, autoErr := l.DomainGetAutostart(dom)
	if autoErr != nil {
		// 某些域可能不支持 autostart，不阻断整体信息获取
		autoVal = 0
	}

	return int(nrVirtCPU), maxMem, memory, autoVal == 1, nil
}

// getDomainXMLRPC 获取 VM XML 配置（替代 virsh dumpxml）
// flags: 0 = 当前状态, libvirt.DomainXMLInactive = inactive 配置
func getDomainXMLRPC(name string, flags libvirt.DomainXMLFlags) (string, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return "", err
	}
	l, err := GetLibvirt()
	if err != nil {
		return "", fmt.Errorf("获取域 %s XML 失败: %w", name, err)
	}
	xml, err := l.DomainGetXMLDesc(dom, flags)
	if err != nil {
		return "", fmt.Errorf("获取域 %s XML 描述失败: %w", name, err)
	}
	return xml, nil
}

// getDomainCPUStatsRPC 获取 CPU 时间统计（替代 virsh cpu-stats --total）
// 返回 CPU 时间（纳秒）
func getDomainCPUStatsRPC(name string) (cpuTime uint64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, fmt.Errorf("获取域 %s CPU 统计失败: %w", name, err)
	}
	_, _, _, _, cput, infoErr := l.DomainGetInfo(dom)
	if infoErr != nil {
		return 0, fmt.Errorf("获取域 %s CPU 时间失败: %w", name, infoErr)
	}
	return cput, nil
}

// memoryStatTagToString 将 DomainMemoryStatTag 映射为字符串 key
var memoryStatTagToString = map[int32]string{
	0:  "swap_in",
	1:  "swap_out",
	2:  "major_fault",
	3:  "minor_fault",
	4:  "unused",
	5:  "available",
	6:  "actual",
	7:  "rss",
	8:  "usable",
	9:  "last_update",
	10: "disk_caches",
	11: "hugetlb_pgalloc",
	12: "hugetlb_pgfail",
}

// getDomainMemoryStatsRPC 获取内存统计（替代 virsh dommemstat）
// 返回 map: "actual"→实际分配KB, "rss"→RSS, "available"→总可用, "unused"→未使用 等
func getDomainMemoryStatsRPC(name string) (map[string]uint64, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return nil, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 内存统计失败: %w", name, err)
	}
	// MaxStats=16 覆盖所有已知统计项
	stats, err := l.DomainMemoryStats(dom, 16, 0)
	if err != nil {
		return nil, fmt.Errorf("获取域 %s 内存统计失败: %w", name, err)
	}

	result := make(map[string]uint64, len(stats))
	for _, s := range stats {
		key, ok := memoryStatTagToString[s.Tag]
		if !ok {
			key = fmt.Sprintf("tag_%d", s.Tag)
		}
		result[key] = s.Val
	}
	return result, nil
}

// getDomainBlockStatsRPC 获取磁盘 I/O 统计（替代 virsh domblkstat）
// 返回 读取字节数, 写入字节数
func getDomainBlockStatsRPC(name, dev string) (rdBytes, wrBytes int64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, fmt.Errorf("获取域 %s 磁盘统计失败: %w", name, err)
	}
	rdReq, rdByt, wrReq, wrByt, errs, statErr := l.DomainBlockStats(dom, dev)
	if statErr != nil {
		return 0, 0, fmt.Errorf("获取域 %s 磁盘 %s 统计失败: %w", name, dev, statErr)
	}
	_ = rdReq
	_ = wrReq
	_ = errs
	return rdByt, wrByt, nil
}

// getDomainInterfaceStatsRPC 获取网络 I/O 统计（替代 virsh domifstat）
// 返回 接收字节数, 发送字节数
func getDomainInterfaceStatsRPC(name, iface string) (rxBytes, txBytes int64, err error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, 0, fmt.Errorf("获取域 %s 网络统计失败: %w", name, err)
	}
	rxByt, rxPkt, rxErr, rxDrop, txByt, txPkt, txErr, txDrop, statErr := l.DomainInterfaceStats(dom, iface)
	if statErr != nil {
		return 0, 0, fmt.Errorf("获取域 %s 网卡 %s 统计失败: %w", name, iface, statErr)
	}
	_ = rxPkt
	_ = rxErr
	_ = rxDrop
	_ = txPkt
	_ = txErr
	_ = txDrop
	return rxByt, txByt, nil
}

// ==================== 中频控制操作 ====================

// startDomainRPC 启动 VM（替代 virsh start）
func startDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("启动域 %s 失败: %w", name, err)
	}
	if err := l.DomainCreate(dom); err != nil {
		return fmt.Errorf("启动域 %s 失败: %w", name, err)
	}
	return nil
}

// startDomainPausedRPC 以暂停模式启动 VM（替代 virsh start --paused）
func startDomainPausedRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("暂停启动域 %s 失败: %w", name, err)
	}
	// DomainCreateWithFlags 的 Flags 参数类型为 uint32
	if _, err := l.DomainCreateWithFlags(dom, uint32(libvirt.DomainStartPaused)); err != nil {
		return fmt.Errorf("暂停启动域 %s 失败: %w", name, err)
	}
	return nil
}

// shutdownDomainRPC 正常关机（替代 virsh shutdown）
func shutdownDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("关机域 %s 失败: %w", name, err)
	}
	if err := l.DomainShutdown(dom); err != nil {
		return fmt.Errorf("关机域 %s 失败: %w", name, err)
	}
	return nil
}

// destroyDomainRPC 强制断电（替代 virsh destroy）
func destroyDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("强制断电域 %s 失败: %w", name, err)
	}
	if err := l.DomainDestroy(dom); err != nil {
		return fmt.Errorf("强制断电域 %s 失败: %w", name, err)
	}
	return nil
}

// rebootDomainRPC 重启（替代 virsh reboot）
func rebootDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("重启域 %s 失败: %w", name, err)
	}
	if err := l.DomainReboot(dom, 0); err != nil {
		return fmt.Errorf("重启域 %s 失败: %w", name, err)
	}
	return nil
}

// resetDomainRPC 硬重置（替代 virsh reset）
func resetDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("硬重置域 %s 失败: %w", name, err)
	}
	if err := l.DomainReset(dom, 0); err != nil {
		return fmt.Errorf("硬重置域 %s 失败: %w", name, err)
	}
	return nil
}

// suspendDomainRPC 暂停（替代 virsh suspend）
func suspendDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("暂停域 %s 失败: %w", name, err)
	}
	if err := l.DomainSuspend(dom); err != nil {
		return fmt.Errorf("暂停域 %s 失败: %w", name, err)
	}
	return nil
}

// resumeDomainRPC 恢复（替代 virsh resume）
func resumeDomainRPC(name string) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("恢复域 %s 失败: %w", name, err)
	}
	if err := l.DomainResume(dom); err != nil {
		return fmt.Errorf("恢复域 %s 失败: %w", name, err)
	}
	return nil
}

// setDomainAutostartRPC 设置自动启动（替代 virsh autostart）
func setDomainAutostartRPC(name string, autostart bool) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 自动启动失败: %w", name, err)
	}
	autostartInt := int32(0)
	if autostart {
		autostartInt = 1
	}
	if err := l.DomainSetAutostart(dom, autostartInt); err != nil {
		return fmt.Errorf("设置域 %s 自动启动失败: %w", name, err)
	}
	return nil
}

// defineDomainXMLRPC 定义/更新 VM 配置（替代 virsh define）
func defineDomainXMLRPC(xmlContent string) (libvirt.Domain, error) {
	l, err := GetLibvirt()
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("定义域失败: %w", err)
	}
	dom, err := l.DomainDefineXML(xmlContent)
	if err != nil {
		return libvirt.Domain{}, fmt.Errorf("定义域失败: %w", err)
	}
	return dom, nil
}

// setDomainVcpusFlagsRPC 设置 vCPU 数量（替代 virsh setvcpus）
// flags 组合使用 domainVcpu* 常量（如 domainVcpuConfig | domainVcpuMaximum）
func setDomainVcpusFlagsRPC(name string, count uint32, flags uint32) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s vCPU 失败: %w", name, err)
	}
	if err := l.DomainSetVcpusFlags(dom, count, flags); err != nil {
		return fmt.Errorf("设置域 %s vCPU 为 %d 失败: %w", name, count, err)
	}
	return nil
}

// setDomainMemoryFlagsRPC 设置内存大小（替代 virsh setmem/setmaxmem）
// flags 组合使用 libvirt.DomainMem* 常量
func setDomainMemoryFlagsRPC(name string, memKB uint64, flags libvirt.DomainMemoryModFlags) error {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return err
	}
	l, err := GetLibvirt()
	if err != nil {
		return fmt.Errorf("设置域 %s 内存失败: %w", name, err)
	}
	if err := l.DomainSetMemoryFlags(dom, memKB, uint32(flags)); err != nil {
		return fmt.Errorf("设置域 %s 内存为 %d KB 失败: %w", name, memKB, err)
	}
	return nil
}

// getDomainVcpuCountRPC 获取 vCPU 计数（替代 virsh vcpucount）
func getDomainVcpuCountRPC(name string, flags uint32) (int, error) {
	dom, err := lookupDomainByName(name)
	if err != nil {
		return 0, err
	}
	l, err := GetLibvirt()
	if err != nil {
		return 0, fmt.Errorf("获取域 %s vCPU 计数失败: %w", name, err)
	}
	count, err := l.DomainGetVcpusFlags(dom, flags)
	if err != nil {
		return 0, fmt.Errorf("获取域 %s vCPU 计数失败: %w", name, err)
	}
	return int(count), nil
}
