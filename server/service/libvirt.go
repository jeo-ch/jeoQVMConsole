package service

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/model"
	"kvm_console/utils"
)

// ==================== Libvirt 命令封装 ====================
// 所有虚拟机操作优先使用 virsh 命令，不使用数据库

const (
	vmConfigMetadataURI = "https://kvm-console.local/domain-config"
	vmConfigMetadataKey = "kvm-console"
)

// VmInfo 虚拟机基本信息
type VmInfo struct {
	Name                     string               `json:"name"`
	Remark                   string               `json:"remark"`
	Group                    string               `json:"group"`
	Status                   string               `json:"status"`             // running, shut off, paused, etc.
	VCPU                     int                  `json:"vcpu"`               // CPU 核心数
	Memory                   int                  `json:"memory"`             // 内存（MB）
	MaxMemory                int                  `json:"max_memory"`         // 最大内存（MB）
	IP                       string               `json:"ip"`                 // IP 地址
	IPStatus                 string               `json:"ip_status"`          // IP 状态: ""=正常, "vlan_bridge"=VLAN桥接无法获取
	DiskSize                 string               `json:"disk_size"`          // 磁盘占用
	Template                 string               `json:"template"`           // 模板来源
	Network                  string               `json:"network"`            // 网络模式
	NicModel                 string               `json:"nic_model"`          // 网卡模型: virtio/e1000e/rtl8139
	Autostart                bool                 `json:"autostart"`          // 开机自启
	MacAddress               string               `json:"mac_address"`        // MAC 地址
	VNCPort                  string               `json:"vnc_port"`           // VNC 端口
	VideoModel               string               `json:"video_model"`        // 视频模型: virtio/vga/vmvga/cirrus
	CPUTopologyMode          string               `json:"cpu_topology_mode"`  // CPU 拓扑模式
	CPULimitPercent          int                  `json:"cpu_limit_percent"`  // CPU 限制百分比，0 表示无限制
	CPUAffinity              string               `json:"cpu_affinity"`       // CPU 亲和性，如 "0,2,4"，空字符串表示未设置
	CPUPercent               float64              `json:"cpu_percent"`        // CPU 使用率（来自缓存）
	MemPercent               float64              `json:"mem_percent"`        // 内存使用率（来自缓存）
	MemoryInitial            int                  `json:"memory_initial"`     // 动态内存启动/保障内存（MB）
	MemoryMin                int                  `json:"memory_min"`         // 动态内存最小内存（MB）
	MemoryMaxDynamic         int                  `json:"memory_max_dynamic"` // 动态内存最大内存（MB）
	MemoryBackend            string               `json:"memory_backend"`     // 动态内存后端: balloon/virtio_mem
	MemoryVirtioMemCurrent   int                  `json:"memory_virtio_mem_current"`
	MemoryDynamicEnabled     bool                 `json:"memory_dynamic_enabled"`
	MemoryAutoBalloon        bool                 `json:"memory_auto_balloon"`
	MemoryPendingApply       bool                 `json:"memory_pending_apply"`
	MemoryCompatMode         string               `json:"memory_compat_mode"`
	MemoryBalloonSupported   bool                 `json:"memory_balloon_supported"`
	MemoryBalloonStatus      string               `json:"memory_balloon_status"`
	CreatedAt                string               `json:"created_at"`    // 创建时间
	BandwidthIn              int                  `json:"bandwidth_in"`  // 下行平峰速率 Mbps
	BandwidthOut             int                  `json:"bandwidth_out"` // 上行平峰速率 Mbps
	PublicIPs                []PublicIPAttachment `json:"public_ips"`    // 已绑定公网 IP
	InRescue                 bool                 `json:"in_rescue"`     // 是否处于救援模式
	Locked                   bool                 `json:"locked"`        // 是否已锁定
	ContinuousRuntimeSeconds int64                `json:"continuous_runtime_seconds"`
	ContinuousRunningSince   string               `json:"continuous_running_since"`
}

// BootDevice 可引导设备信息（类似 Cockpit 的引导顺序展示）
type BootDevice struct {
	Type    string `json:"type"`    // 设备类型: disk / cdrom / network
	Device  string `json:"device"`  // 设备标识: vda, sda, sdb 等
	File    string `json:"file"`    // 文件路径或 MAC 地址
	Bus     string `json:"bus"`     // 总线类型: virtio, sata, ide, scsi
	Enabled bool   `json:"enabled"` // 是否在引导列表中启用
	Order   int    `json:"order"`   // 引导顺序（0 表示未设置）
}

// VmDetail 虚拟机详细信息
type VmDetail struct {
	VmInfo
	DiskPath               string                    `json:"disk_path"`    // 磁盘路径
	DiskHealthy            *bool                     `json:"disk_healthy"`  // 磁盘完整性标记: nil=未检查, true=正常, false=磁盘文件缺失
	UUID                   string                    `json:"uuid"`         // 虚拟机 UUID
	VNCPort                string                    `json:"vnc_port"`     // VNC 端口
	Snapshots              []string                  `json:"snapshots"`    // 快照列表
	OSType                 string                    `json:"os_type"`      // 系统类型
	BootType               string                    `json:"boot_type"`    // 引导方式: bios/uefi/uefi-secure
	BootOrder              []string                  `json:"boot_order"`   // 引导顺序（OS 级别: hd, cdrom, network）
	BootDevices            []BootDevice              `json:"boot_devices"` // 所有可引导设备列表
	Arch                   string                    `json:"arch"`         // 来宾架构
	MachineType            string                    `json:"machine_type"` // 机器类型: q35/i440fx/virt
	Bandwidth              *BandwidthDetail          `json:"bandwidth"`    // 带宽详情
	LightweightQuota       *model.LightweightVMQuota `json:"lightweight_quota"`
	Stats                  *VmStats                  `json:"stats"`         // 实时资源使用（缓存数据，SSE 推送用）
	Credential             *VMCredentialInfo         `json:"credential"`    // 保存的登录凭据
	Freeze                 bool                      `json:"freeze"`        // 启动时冻结 CPU
	APIC                   bool                      `json:"apic"`          // APIC 开关
	PAE                    bool                      `json:"pae"`           // PAE 开关
	RTCOffset              string                    `json:"rtc_offset"`    // RTC 偏移值: utc/localtime
	RTCStartDate           string                    `json:"rtc_startdate"` // RTC 开始日期
	GuestAgent             *VMGuestAgentConfig       `json:"guest_agent"`   // QEMU Guest Agent 配置
	SMBIOS1                *VMSMBIOS1Config          `json:"smbios1"`       // SMBIOS 类型 1 信息
	MemoryObservationUntil int64                     `json:"memory_observation_until"`
	MemoryManualPauseUntil int64                     `json:"memory_manual_pause_until"`
	PCIERootPorts          int                       `json:"pcie_root_ports"` // pcie-root-port 数量（仅 q35/virt 机型）
}

// VmStats 虚拟机资源使用统计
type VmStats struct {
	CPUPercent  float64 `json:"cpu_percent"` // CPU 使用率
	MemUsed     int64   `json:"mem_used"`    // 已用内存（KB）
	MemTotal    int64   `json:"mem_total"`   // 总内存（KB）
	NetRxBytes  int64   `json:"net_rx_bytes"`
	NetTxBytes  int64   `json:"net_tx_bytes"`
	DiskRdBytes int64   `json:"disk_rd_bytes"`
	DiskWrBytes int64   `json:"disk_wr_bytes"`
}

// VMListOptions 虚拟机列表查询选项
type VMListOptions struct {
	IncludeResourceUsage bool
	IncludeIP            bool
	IncludeNetworkInfo   bool
	IncludeBandwidth     bool
}

// HostStats 宿主机资源信息
type HostStats struct {
	CPUCount        int     `json:"cpu_count"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemTotal        int64   `json:"mem_total"`  // KB
	MemFree         int64   `json:"mem_free"`   // KB
	MemUsed         int64   `json:"mem_used"`   // KB
	SwapTotal       int64   `json:"swap_total"` // KB
	SwapFree        int64   `json:"swap_free"`  // KB
	SwapUsed        int64   `json:"swap_used"`  // KB
	DiskTotal       int64   `json:"disk_total"` // KB
	DiskUsed        int64   `json:"disk_used"`  // KB
	DiskFree        int64   `json:"disk_free"`  // KB
	NetRxBytes      int64   `json:"net_rx_bytes"`
	NetTxBytes      int64   `json:"net_tx_bytes"`
	DiskRdBytes     int64   `json:"disk_rd_bytes"`
	DiskWrBytes     int64   `json:"disk_wr_bytes"`
	Hostname        string  `json:"hostname"`
	Uptime          string  `json:"uptime"`
	VMRunning       int     `json:"vm_running"`
	VMTotal         int     `json:"vm_total"`
	KSMPagesShared  int64   `json:"ksm_pages_shared"`
	KSMPagesSharing int64   `json:"ksm_pages_sharing"`
	DiskIOLatencyMs float64 `json:"disk_io_latency_ms"` // ms (avg)
}

// XML 解析用结构体
type domainXML struct {
	XMLName xml.Name      `xml:"domain"`
	Name    string        `xml:"name"`
	Memory  domainMemory  `xml:"memory"`
	VCPU    int           `xml:"vcpu"`
	OS      domainOS      `xml:"os"`
	Devices domainDevices `xml:"devices"`
}

type domainMemory struct {
	Unit  string `xml:"unit,attr"`
	Value int    `xml:",chardata"`
}

type domainOS struct {
	Type struct {
		Arch string `xml:"arch,attr"`
	} `xml:"type"`
}

type domainDevices struct {
	Disks      []domainDisk      `xml:"disk"`
	Interfaces []domainInterface `xml:"interface"`
	Graphics   []domainGraphics  `xml:"graphics"`
}

type domainDisk struct {
	Type   string `xml:"type,attr"`
	Device string `xml:"device,attr"`
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
	Driver struct {
		Name string `xml:"name,attr"`
		Type string `xml:"type,attr"`
	} `xml:"driver"`
}

type domainInterface struct {
	Type   string `xml:"type,attr"`
	Source struct {
		Network string `xml:"network,attr"`
		Bridge  string `xml:"bridge,attr"`
	} `xml:"source"`
	Mac struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
}

type domainGraphics struct {
	Type   string `xml:"type,attr"`
	Port   string `xml:"port,attr"`
	Listen string `xml:"listen,attr"`
}

// ==================== 核心命令封装 ====================

// ListVMs 列出所有虚拟机
func ListVMs(options ...VMListOptions) ([]VmInfo, error) {
	listOptions := VMListOptions{}
	if len(options) > 0 {
		listOptions = options[0]
	}

	// 获取所有虚拟机名称
	var names []string
	if IsLibvirtRPCAvailable() {
		if domains, err := listAllDomainsRPC(); err == nil {
			for _, dom := range domains {
				names = append(names, dom.Name)
			}
		} else {
			log.Printf("[go-libvirt] listAllDomainsRPC 失败，降级为 virsh: %v", err)
		}
	}
	if len(names) == 0 {
		result := utils.ExecCommand("virsh", "list", "--all", "--name")
		if result.Error != nil {
			// 维护模式下 libvirtd 可能已被主动停用，此时列表接口降级为空列表，
			// 避免前端不断弹出连接 hypervisor 失败的错误。
			if IsMaintenanceModeEnabled() && (isLibvirtUnavailableText(result.Stderr) || IsLibvirtUnavailableError(result.Error)) {
				return []VmInfo{}, nil
			}
			return nil, result.Error
		}
		names = strings.Split(result.Stdout, "\n")
	}

	// 提前批量获取所有虚拟机的创建时间
	statMap := make(map[string]string)
	bulkStatResult := utils.ExecShell("stat -c '%n|%W|%Y' /etc/libvirt/qemu/*.xml 2>/dev/null")
	if bulkStatResult.Error == nil {
		lines := strings.Split(bulkStatResult.Stdout, "\n")
		for _, line := range lines {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) >= 3 {
				pathParts := strings.Split(parts[0], "/")
				fileName := pathParts[len(pathParts)-1]
				vmName := strings.TrimSuffix(fileName, ".xml")
				w, _ := strconv.ParseInt(parts[1], 10, 64)
				y, _ := strconv.ParseInt(parts[2], 10, 64)
				ts := w
				if ts <= 0 {
					ts = y
				}
				if ts > 0 {
					statMap[vmName] = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
				}
			}
		}
	}

	var vms []VmInfo

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		vm := VmInfo{
			Name:      name,
			CreatedAt: statMap[name],
		}

		// 获取状态
		if IsLibvirtRPCAvailable() {
			if state, err := getDomainStateRPC(name); err == nil {
				vm.Status = state
			} else {
				log.Printf("[go-libvirt] getDomainStateRPC(%s) 失败，降级为 virsh: %v", name, err)
			}
		}
		if vm.Status == "" {
			stateResult := utils.ExecCommand("virsh", "domstate", name)
			if stateResult.Error == nil {
				vm.Status = strings.TrimSpace(stateResult.Stdout)
			}
		}
		UpdateVMRuntimeState(name, vm.Status, time.Now())

		// 获取基本信息（从 dominfo）
		if IsLibvirtRPCAvailable() {
			if vcpu, maxMemKB, usedMemKB, autostart, err := getDomainInfoRPC(name); err == nil {
				vm.VCPU = vcpu
				vm.MaxMemory = int(maxMemKB) / 1024 // KiB -> MB
				vm.Memory = int(usedMemKB) / 1024   // KiB -> MB
				vm.Autostart = autostart
			} else {
				log.Printf("[go-libvirt] getDomainInfoRPC(%s) 失败，降级为 virsh: %v", name, err)
			}
		}
		if vm.VCPU == 0 {
			infoResult := utils.ExecCommand("virsh", "dominfo", name)
			if infoResult.Error == nil {
				vm.VCPU = parseInfoInt(infoResult.Stdout, "CPU(s):")
				maxMem := parseInfoInt(infoResult.Stdout, "Max memory:")
				vm.MaxMemory = maxMem / 1024 // KiB -> MB
				usedMem := parseInfoInt(infoResult.Stdout, "Used memory:")
				vm.Memory = usedMem / 1024 // KiB -> MB
				vm.Autostart = strings.Contains(infoResult.Stdout, "Autostart:      enable")
			}
		}
		var xmlStr string
		if IsLibvirtRPCAvailable() {
			if xmlRPC, err := getDomainXMLRPC(name, libvirt.DomainXMLInactive); err == nil {
				xmlStr = xmlRPC
			} else {
				log.Printf("[go-libvirt] getDomainXMLRPC(%s) 失败，降级为 virsh: %v", name, err)
			}
		}
		if xmlStr == "" {
			if xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive"); xmlResult.Error == nil {
				xmlStr = xmlResult.Stdout
			}
		}
		if xmlStr != "" {
			// 使用持久化配置的 vCPU 覆盖在线值，确保界面显示的 vCPU 与用户配置一致
			// (libvirt 不支持在线修改 vCPU 最大值，热添加超限时持久化已更新但在线未变)
			if configVCPU := ParseVCPUCountFromDomainXML(xmlStr); configVCPU > 0 {
				vm.VCPU = configVCPU
			}
			applyMemoryDynamicInfoToVMInfo(&vm, GetVMMemoryDynamicInfo(name, xmlStr, vm.Status))
			vm.CPULimitPercent = ParseVMCPULimitPercentFromDomainXML(xmlStr, vm.VCPU)
			vm.CPUAffinity = ParseCPUAffinityFromDomainXML(xmlStr)
		}
		if remark, err := GetVMRemark(name); err == nil {
			vm.Remark = remark
		}

		if listOptions.IncludeIP {
			vm.IP = getVMIP(name, vm.Status == "running")
		}
		vm.IPStatus = getVMIPStatus(name, vm.Status == "running")

		// 获取磁盘信息和模板来源
		diskInfo := getVMDiskInfo(name)
		vm.DiskSize = diskInfo.size
		vm.Template = diskInfo.template

		// 仅在需要时查询列表页未直接使用的网卡 / 带宽信息，避免每台 VM 额外触发多次 virsh 调用。
		if listOptions.IncludeNetworkInfo {
			netInfo := getVMNetworkInfo(name)
			vm.Network = netInfo.network
			vm.NicModel = netInfo.nicModel
			vm.MacAddress = netInfo.mac
		}
		if listOptions.IncludeBandwidth {
			vm.BandwidthIn, vm.BandwidthOut = GetVMBandwidthMbps(name)
		}

		// 仅在明确请求时，从缓存补充资源使用率
		if listOptions.IncludeResourceUsage && vm.Status == "running" {
			if cached := GetCachedStats(name); cached != nil {
				vm.CPUPercent = cached.CPUPercent
				if cached.MemTotal > 0 {
					vm.MemPercent = float64(cached.MemUsed) / float64(cached.MemTotal) * 100
				}
			}
		}

		// 检查是否处于救援模式
		vm.InRescue = IsInRescueMode(name)
		runtimeInfo := GetVMRuntimeInfo(name, vm.Status)
		vm.ContinuousRuntimeSeconds = runtimeInfo.ContinuousRuntimeSeconds
		vm.ContinuousRunningSince = runtimeInfo.ContinuousRunningSince
		applyVMUnderMigrationStatus(&vm)

		vms = append(vms, vm)
	}

	return vms, nil
}

// GetVM 获取单个虚拟机详情
func GetVM(name string) (*VmDetail, error) {
	// 检查虚拟机是否存在并获取基本信息
	var dominfoStdout string
	domExists := false
	if IsLibvirtRPCAvailable() {
		if _, _, _, _, err := getDomainInfoRPC(name); err == nil {
			domExists = true
		} else {
			log.Printf("[go-libvirt] getDomainInfoRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if !domExists {
		result := utils.ExecCommand("virsh", "dominfo", name)
		if result.Error != nil {
			return nil, fmt.Errorf("虚拟机不存在: %s", name)
		}
		dominfoStdout = result.Stdout
	}

	vm := &VmDetail{}
	vm.Name = name
	if remark, err := GetVMRemark(name); err == nil {
		vm.Remark = remark
	}

	// 状态
	if IsLibvirtRPCAvailable() {
		if state, err := getDomainStateRPC(name); err == nil {
			vm.Status = state
		} else {
			log.Printf("[go-libvirt] getDomainStateRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if vm.Status == "" {
		stateResult := utils.ExecCommand("virsh", "domstate", name)
		if stateResult.Error == nil {
			vm.Status = strings.TrimSpace(stateResult.Stdout)
		}
	}
	UpdateVMRuntimeState(name, vm.Status, time.Now())

	// 基本信息
	if IsLibvirtRPCAvailable() {
		if vcpu, maxMemKB, usedMemKB, autostart, err := getDomainInfoRPC(name); err == nil {
			vm.VCPU = vcpu
			vm.MaxMemory = int(maxMemKB) / 1024
			vm.Memory = int(usedMemKB) / 1024
			vm.Autostart = autostart
		} else {
			log.Printf("[go-libvirt] getDomainInfoRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if dominfoStdout != "" {
		vm.VCPU = parseInfoInt(dominfoStdout, "CPU(s):")
		maxMem := parseInfoInt(dominfoStdout, "Max memory:")
		vm.MaxMemory = maxMem / 1024
		usedMem := parseInfoInt(dominfoStdout, "Used memory:")
		vm.Memory = usedMem / 1024
		vm.Autostart = strings.Contains(dominfoStdout, "Autostart:      enable")
		vm.UUID = parseInfoValue(dominfoStdout, "UUID:")
	}

	// 创建时间
	xmlPath := fmt.Sprintf("/etc/libvirt/qemu/%s.xml", name)
	statRes := utils.ExecShell(fmt.Sprintf("stat -c %%W|%%Y %s 2>/dev/null", utils.ShellSingleQuote(xmlPath)))
	if statRes.Error == nil {
		parts := strings.Split(strings.TrimSpace(statRes.Stdout), "|")
		if len(parts) >= 2 {
			w, _ := strconv.ParseInt(parts[0], 10, 64)
			y, _ := strconv.ParseInt(parts[1], 10, 64)
			ts := w
			if ts <= 0 {
				ts = y
			}
			if ts > 0 {
				vm.CreatedAt = time.Unix(ts, 0).Format("2006-01-02 15:04:05")
			}
		}
	}

	// IP（无论是否运行都尝试获取，可从静态绑定中兜底获取）
	vm.IP = getVMIP(name, vm.Status == "running")
	vm.IPStatus = getVMIPStatus(name, vm.Status == "running")
	vm.PublicIPs = ListPublicIPAttachmentsForVM(name)

	// 磁盘
	diskInfo := getVMDiskInfo(name)
	vm.DiskPath = diskInfo.path
	vm.DiskSize = diskInfo.size
	vm.Template = diskInfo.template

	// 检查系统盘完整性（仅检查第一块非 cdrom 磁盘）
	if diskInfo.path != "" {
		if _, err := os.Stat(diskInfo.path); err != nil {
			unhealthy := false
			vm.DiskHealthy = &unhealthy
			log.Printf("[警告] 虚拟机 %s 磁盘文件缺失: %s", name, diskInfo.path)
		} else {
			healthy := true
			vm.DiskHealthy = &healthy
		}
	}

	// 网络
	netInfo := getVMNetworkInfo(name)
	vm.Network = netInfo.network
	vm.NicModel = netInfo.nicModel
	vm.MacAddress = netInfo.mac

	// VNC 信息
	if vm.Status == "running" || vm.Status == "paused" {
		vncResult := utils.ExecCommand("virsh", "vncdisplay", name)
		if vncResult.Error == nil {
			vm.VNCPort = strings.TrimSpace(vncResult.Stdout)
		}
	}

	// 获取 XML 判断系统类型、引导顺序和可引导设备
	var xmlStr string
	if IsLibvirtRPCAvailable() {
		if xmlRPC, err := getDomainXMLRPC(name, libvirt.DomainXMLInactive); err == nil {
			xmlStr = xmlRPC
		} else {
			log.Printf("[go-libvirt] getDomainXMLRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if xmlStr == "" {
		xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
		if xmlResult.Error == nil {
			xmlStr = xmlResult.Stdout
		}
	}
	if xmlStr != "" {
		// 使用持久化配置的 vCPU 覆盖在线值，确保界面显示的 vCPU 与用户配置一致
		// (libvirt 不支持在线修改 vCPU 最大值，热添加超限时持久化已更新但在线未变)
		if configVCPU := ParseVCPUCountFromDomainXML(xmlStr); configVCPU > 0 {
			vm.VCPU = configVCPU
		}
		vm.OSType = detectVMOSType(vm.Template, xmlStr)
		vm.BootType = ParseVMBootTypeFromDomainXML(xmlStr)
		vm.Arch = ParseVMArchFromDomainXML(xmlStr)
		vm.MachineType = ParseVMMachineTypeFromDomainXML(xmlStr)
		if count, err := GetVMPCIERootPorts(name); err == nil {
			vm.PCIERootPorts = count
		}
		vm.VideoModel = ParseVMVideoModelFromDomainXML(xmlStr)
		vm.CPUTopologyMode = ParseVMCPUTopologyModeFromDomainXML(xmlStr)
		vm.CPULimitPercent = ParseVMCPULimitPercentFromDomainXML(xmlStr, vm.VCPU)
		vm.CPUAffinity = ParseCPUAffinityFromDomainXML(xmlStr)
		vm.APIC = ParseVMAPICFromDomainXML(xmlStr)
		vm.PAE = ParseVMPAEFromDomainXML(xmlStr)
		vm.RTCOffset = ParseRTCOffsetFromDomainXML(xmlStr)
		vm.RTCStartDate = ParseRTCStartDateFromDomainXML(xmlStr)
		vm.GuestAgent = ParseVMGuestAgentConfigFromDomainXML(xmlStr)
		vm.SMBIOS1 = ParseSMBIOS1ConfigFromDomainXML(xmlStr)
		memInfo := GetVMMemoryDynamicInfo(name, xmlStr, vm.Status)
		applyMemoryDynamicInfoToVMInfo(&vm.VmInfo, memInfo)
		if memInfo != nil {
			vm.MemoryObservationUntil = memInfo.ObservationUntil
			vm.MemoryManualPauseUntil = memInfo.ManualPauseUntil
		}

		// 解析引导顺序（OS 级别 <boot dev='xxx'/>）
		bootDevRe := regexp.MustCompile(`<boot dev='([^']+)'/>`)
		bootMatches := bootDevRe.FindAllStringSubmatch(xmlStr, -1)
		for _, m := range bootMatches {
			vm.BootOrder = append(vm.BootOrder, m[1])
		}
		if len(vm.BootOrder) == 0 {
			vm.BootOrder = []string{"hd"}
		}

		// 解析所有可引导设备
		vm.BootDevices = parseBootDevices(xmlStr, vm.BootOrder)
		vm.Freeze = isVMFreezeEnabled(xmlStr)
	}

	// 获取带宽详情
	vm.BandwidthIn, vm.BandwidthOut = GetVMBandwidthMbps(name)
	if bwDetail, err := GetVMBandwidth(name); err == nil {
		vm.Bandwidth = bwDetail
	}
	if quota, err := GetLightweightVMQuota(name); err == nil {
		vm.LightweightQuota = quota
	}

	// 检查是否处于救援模式
	vm.InRescue = IsInRescueMode(name)
	runtimeInfo := GetVMRuntimeInfo(name, vm.Status)
	vm.ContinuousRuntimeSeconds = runtimeInfo.ContinuousRuntimeSeconds
	vm.ContinuousRunningSince = runtimeInfo.ContinuousRunningSince

	// 从缓存获取实时资源数据（后台采集器每10秒更新，不阻塞SSE推送）
	if vm.Status == "running" {
		vm.Stats = GetCachedStats(name)
	}

	// 读取已保存的虚拟机登录凭据
	if credential, err := GetVMCredential(name); err == nil {
		vm.Credential = credential
	}
	vm.Locked = IsVMLocked(name)
	applyVMUnderMigrationStatus(&vm.VmInfo)

	return vm, nil
}

// GetVMIPInfo 获取单个虚拟机 IP 信息
func GetVMIPInfo(name string) (string, string, error) {
	result := utils.ExecCommand("virsh", "dominfo", name)
	if result.Error != nil {
		return "", "", fmt.Errorf("虚拟机不存在: %s", name)
	}

	isRunning := false
	if IsLibvirtRPCAvailable() {
		if state, err := getDomainStateRPC(name); err == nil {
			isRunning = state == "running"
		} else {
			log.Printf("[go-libvirt] getDomainStateRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if !isRunning {
		stateResult := utils.ExecCommand("virsh", "domstate", name)
		isRunning = stateResult.Error == nil && strings.TrimSpace(stateResult.Stdout) == "running"
	}

	return getVMIP(name, isRunning), getVMIPStatus(name, isRunning), nil
}

func detectVMOSType(templateName, xmlStr string) string {
	if templateName != "" {
		if meta := GetTemplateMeta(templateName); meta != nil {
			switch strings.ToLower(strings.TrimSpace(meta.Type)) {
			case "fnos":
				return "fnos"
			case "windows":
				return "windows"
			case "linux":
				return "linux"
			}
		}
	}

	if strings.Contains(xmlStr, "firmware='efi'") &&
		strings.Contains(xmlStr, "hyperv") {
		return "windows"
	}
	return "linux"
}

func applyMemoryDynamicInfoToVMInfo(vm *VmInfo, info *VMMemoryDynamicInfo) {
	if vm == nil || info == nil {
		return
	}
	vm.MemoryInitial = info.MemoryInitial
	vm.MemoryMin = info.MemoryMin
	vm.MemoryMaxDynamic = info.MemoryMax
	vm.MemoryBackend = info.MemoryBackend
	vm.MemoryVirtioMemCurrent = info.VirtioMemCurrent
	vm.MemoryDynamicEnabled = info.DynamicEnabled
	vm.MemoryAutoBalloon = info.AutoBalloon
	vm.MemoryPendingApply = info.PendingApply
	vm.MemoryCompatMode = info.CompatMode
	vm.MemoryBalloonSupported = info.BalloonSupported
	vm.MemoryBalloonStatus = info.BalloonStatus
}

func isVMFreezeEnabled(content string) bool {
	content = strings.ToLower(content)
	return strings.Contains(content, `freeze="yes"`) ||
		strings.Contains(content, `freeze="true"`) ||
		strings.Contains(content, `freeze='yes'`) ||
		strings.Contains(content, `freeze='true'`)
}

// GetVMFreeze 获取虚拟机是否启用启动冻结
func GetVMFreeze(name string) (bool, error) {
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return false, err
	}
	return metadataFreezeEnabled(metadata), nil
}

// SetVMFreeze 设置虚拟机启动时冻结 CPU
func SetVMFreeze(name string, freeze bool) error {
	if err := EnsureVMNotMigrating(name, "设置启动冻结"); err != nil {
		return err
	}
	metadata, err := readVMConfigMetadata(name)
	if err != nil {
		return err
	}
	if freeze {
		metadata.Freeze = "yes"
	} else {
		metadata.Freeze = ""
	}
	if err := writeVMConfigMetadata(name, metadata); err != nil {
		return err
	}
	RefreshVMCacheByNameAsync(name)
	return nil
}

// StartVM 启动虚拟机
func StartVM(name string) error {
	return startVM(name, true)
}

// StartVMPreserveRebootAction 启动虚拟机，但保留当前 on_reboot 策略。
func StartVMPreserveRebootAction(name string) error {
	return startVM(name, false)
}

func applyVMRuntimeNetworkState(name string) error {
	if err := ApplyVPCBindingRuntime(name); err != nil {
		return fmt.Errorf("应用 VPC 网络失败: %w", err)
	}
	if IsLightweightCloudVM(name) {
		if err := ApplyLightweightVMBandwidth(name); err != nil {
			return fmt.Errorf("应用轻量云带宽失败: %w", err)
		}
		return nil
	}
	if err := ReapplyConfiguredVMBandwidth(name); err != nil {
		return fmt.Errorf("刷新虚拟机带宽失败: %w", err)
	}
	return nil
}

func startVM(name string, fixOnReboot bool) error {
	if err := EnsureVMNotMigrating(name, "开机"); err != nil {
		return err
	}
	if owner := FindVMOwner(name); owner != "" && owner != "admin" {
		if IsLightweightCloudUser(owner) {
			if err := CheckLightweightVMRuntimeQuotaAvailable(name); err != nil {
				return err
			}
		} else {
			if err := CheckQuotaForStart(owner, name); err != nil {
				return err
			}
		}
	}
	if err := EnsureMaintenanceModeDisabled("启动虚拟机"); err != nil {
		return err
	}
	if err := EnsureOVSNetworkReady(); err != nil {
		return err
	}

	if fixOnReboot {
		// 启动前自动修复 on_reboot 配置
		FixOnReboot(name)
	}

	// 检测当前状态
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	if stateResult.Error == nil {
		state := strings.TrimSpace(stateResult.Stdout)
		switch state {
		case "running":
			return fmt.Errorf("虚拟机 %s 已在运行中", name)
		case "paused":
			if isQEMUInternalErrorPaused(name) {
				return fmt.Errorf("虚拟机处于 QEMU 内部错误暂停，当前状态不能继续启动；请先执行重置或强制断电后重新开机。如果重置后仍反复进入该状态，请检查宿主机 KVM/嵌套虚拟化能力和 QEMU 日志")
			}
			if IsLibvirtRPCAvailable() {
				err := resumeDomainRPC(name)
				if err == nil {
					UpdateVMRuntimeState(name, "running", time.Now())
					if err := applyVMRuntimeNetworkState(name); err != nil {
						return fmt.Errorf("恢复运行成功，但%w", err)
					}
					return nil
				}
				log.Printf("[go-libvirt] 恢复 %s 失败，降级为 virsh: %v", name, err)
			}
			result := utils.ExecCommand("virsh", "resume", name)
			if result.Error != nil {
				return formatResumeError(name, result.Stderr)
			}
			UpdateVMRuntimeState(name, "running", time.Now())
			if err := applyVMRuntimeNetworkState(name); err != nil {
				return fmt.Errorf("恢复运行成功，但%w", err)
			}
			return nil
		case "crashed", "pmsuspended":
			log.Printf("[警告] 虚拟机 %s 处于异常状态(%s)，尝试强制关闭后重启", name, state)
			utils.ExecCommand("virsh", "destroy", name)
		}
	}

	// 启动前清理不完整的 backingStore XML（防止 AppArmor 拦截 backing chain 访问）
	fixBackingStoreXML(name)
	if err := ApplyPendingVMMemoryConfig(name); err != nil {
		return fmt.Errorf("应用动态内存待迁移配置失败: %w", err)
	}

	freeze, err := GetVMFreeze(name)
	if err != nil {
		return err
	}

	startArgs := []string{"start", name}
	statusAfterStart := "running"
	if freeze {
		startArgs = append(startArgs, "--paused")
		statusAfterStart = "paused"
	}

	started := false
	if IsLibvirtRPCAvailable() {
		var startErr error
		if freeze {
			startErr = startDomainPausedRPC(name)
		} else {
			startErr = startDomainRPC(name)
		}
		if startErr == nil {
			started = true
		} else {
			log.Printf("[go-libvirt] 启动 %s 失败，降级为 virsh: %v", name, startErr)
		}
	}
	if !started {
		result := utils.ExecCommand("virsh", startArgs...)
		if result.Error != nil {
			// 检查是否是权限问题，自动修复后重试一次
			if strings.Contains(result.Stderr, "Permission denied") {
				fixSnapshotDiskPermissions(name)
				retryResult := utils.ExecCommand("virsh", startArgs...)
				if retryResult.Error != nil {
					return fmt.Errorf("启动虚拟机失败: %s", retryResult.Stderr)
				}
			} else {
				return fmt.Errorf("启动虚拟机失败: %s", result.Stderr)
			}
		}
	}
	UpdateVMRuntimeState(name, statusAfterStart, time.Now())
	if err := applyVMRuntimeNetworkState(name); err != nil {
		return fmt.Errorf("启动成功，但%w", err)
	}
	return nil
}

func isQEMUInternalErrorPaused(name string) bool {
	status := getQEMUMonitorStatus(name)
	return strings.Contains(strings.ToLower(status), "internal-error")
}

func getQEMUMonitorStatus(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	result := utils.ExecCommand("virsh", "qemu-monitor-command", name, "--hmp", "info status")
	return strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
}

func formatResumeError(name, stderr string) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		message = "未知错误"
	}
	lower := strings.ToLower(message + "\n" + getQEMUMonitorStatus(name))
	if strings.Contains(lower, "resetting the virtual machine is required") || strings.Contains(lower, "internal-error") {
		return fmt.Errorf("恢复运行失败: 虚拟机处于 QEMU 内部错误暂停，当前状态不能继续启动；请先执行重置或强制断电后重新开机。如果重置后仍反复进入该状态，请检查宿主机 KVM/嵌套虚拟化能力和 QEMU 日志。原始错误: %s", message)
	}
	return fmt.Errorf("恢复运行失败: %s", message)
}

// fixBackingStoreXML 清理 VM XML 中不完整的 backingStore 标签
// 外部快照创建后 libvirt 会写入部分 backingStore 信息，但 backing chain 可能更深，
// 导致 virt-aa-helper 无法为完整 backing chain 生成 AppArmor 权限，开机时报 Permission denied
func fixBackingStoreXML(vmName string) {
	dumpResult := utils.ExecCommand("virsh", "dumpxml", vmName, "--inactive")
	if dumpResult.Error != nil {
		return
	}
	if !strings.Contains(dumpResult.Stdout, "<backingStore") {
		return
	}
	shellCmd := fmt.Sprintf("EDITOR=\"sed -i '/<backingStore type/,/<\\/backingStore>/d'\" virsh edit %s", utils.ShellSingleQuote(vmName))
	utils.ExecShell(shellCmd)
}

// ShutdownVM 正常关机
func ShutdownVM(name string) error {
	if err := EnsureVMNotMigrating(name, "关机"); err != nil {
		return err
	}
	if IsLibvirtRPCAvailable() {
		err := shutdownDomainRPC(name)
		if err == nil {
			return nil
		}
		log.Printf("[go-libvirt] 关机 %s 失败，降级为 virsh: %v", name, err)
	}
	result := utils.ExecCommand("virsh", "shutdown", name)
	if result.Error != nil {
		return fmt.Errorf("关机失败: %s", result.Stderr)
	}
	return nil
}

// DestroyVM 强制断电
func DestroyVM(name string) error {
	if err := EnsureVMNotMigrating(name, "强制断电"); err != nil {
		return err
	}
	if IsLibvirtRPCAvailable() {
		err := destroyDomainRPC(name)
		if err == nil {
			UpdateVMRuntimeState(name, "shut off", time.Now())
			return nil
		}
		log.Printf("[go-libvirt] 强制断电 %s 失败，降级为 virsh: %v", name, err)
	}
	result := utils.ExecCommand("virsh", "destroy", name)
	if result.Error != nil {
		return fmt.Errorf("强制断电失败: %s", result.Stderr)
	}
	UpdateVMRuntimeState(name, "shut off", time.Now())
	return nil
}

// RebootVM 重启虚拟机
func RebootVM(name string) error {
	if err := EnsureVMNotMigrating(name, "重启"); err != nil {
		return err
	}
	if err := EnsureMaintenanceModeDisabled("重启虚拟机"); err != nil {
		return err
	}

	// 先修复 on_reboot 配置（Cockpit/virt-install 默认 destroy 导致重启变关机）
	FixOnReboot(name)

	if IsLibvirtRPCAvailable() {
		err := rebootDomainRPC(name)
		if err == nil {
			ResetVMContinuousRuntime(name, time.Now())
			return nil
		}
		log.Printf("[go-libvirt] 重启 %s 失败，降级为 virsh: %v", name, err)
	}
	result := utils.ExecCommand("virsh", "reboot", name)
	if result.Error != nil {
		return fmt.Errorf("重启失败: %s", result.Stderr)
	}
	ResetVMContinuousRuntime(name, time.Now())
	return nil
}

// ResetVM 硬重置虚拟机，适用于 QEMU internal-error 暂停等无法 resume 的状态。
func ResetVM(name string) error {
	if err := EnsureVMNotMigrating(name, "重置"); err != nil {
		return err
	}
	if err := EnsureMaintenanceModeDisabled("重置虚拟机"); err != nil {
		return err
	}
	FixOnReboot(name)
	if IsLibvirtRPCAvailable() {
		err := resetDomainRPC(name)
		if err == nil {
			ResetVMContinuousRuntime(name, time.Now())
			if err := ApplyVPCBindingRuntime(name); err != nil {
				return fmt.Errorf("重置成功，但应用 VPC 网络失败: %w", err)
			}
			return nil
		}
		log.Printf("[go-libvirt] 重置 %s 失败，降级为 virsh: %v", name, err)
	}
	result := utils.ExecCommand("virsh", "reset", name)
	if result.Error != nil {
		return fmt.Errorf("重置失败: %s", result.Stderr)
	}
	ResetVMContinuousRuntime(name, time.Now())
	if err := ApplyVPCBindingRuntime(name); err != nil {
		return fmt.Errorf("重置成功，但应用 VPC 网络失败: %w", err)
	}
	return nil
}

// FixOnReboot 修复虚拟机的 on_reboot 配置（destroy → restart）
func FixOnReboot(name string) {
	xmlPath := fmt.Sprintf("/etc/libvirt/qemu/%s.xml", name)
	// 检查是否需要修复
	checkResult := utils.ExecShell(fmt.Sprintf("grep '<on_reboot>destroy</on_reboot>' %s 2>/dev/null", utils.ShellSingleQuote(xmlPath)))
	if checkResult.Error != nil || strings.TrimSpace(checkResult.Stdout) == "" {
		return // 不需要修复
	}
	// 直接 sed 修改 XML 文件
	utils.ExecShell(fmt.Sprintf(
		"sed -i 's|<on_reboot>destroy</on_reboot>|<on_reboot>restart</on_reboot>|' %s", utils.ShellSingleQuote(xmlPath)))
	// 重载 libvirtd 使配置生效
	utils.ExecCommand("systemctl", "reload", "libvirtd")
}

// EditVMConfig 编辑虚拟机配置（CPU/内存）
func EditVMConfig(name string, vcpu, maxVCPU, memoryMB int) error {
	if err := EnsureVMNotMigrating(name, "编辑配置"); err != nil {
		return err
	}
	// 检查虚拟机状态
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	if stateResult.Error != nil {
		return fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
	}
	state := strings.TrimSpace(stateResult.Stdout)

	// 修改 CPU
	if vcpu > 0 {
		if state == "running" {
			// 先获取在线最大 vCPU：libvirt 不支持在线修改最大值，新 vCPU 超过在线最大值时热添加必然失败
			liveMaxResult := utils.ExecCommand("virsh", "vcpucount", name, "--maximum", "--live")
			liveMax, _ := strconv.Atoi(strings.TrimSpace(liveMaxResult.Stdout))
			if liveMax <= 0 {
				liveMaxResult2 := utils.ExecCommand("virsh", "vcpucount", name, "--maximum")
				liveMax, _ = strconv.Atoi(strings.TrimSpace(liveMaxResult2.Stdout))
			}
			if vcpu <= liveMax {
				if IsLibvirtRPCAvailable() {
					if err := setDomainVcpusFlagsRPC(name, uint32(vcpu), domainVcpuLive); err != nil {
						log.Printf("[go-libvirt] 设置 %s live vCPU 失败，降级为 virsh: %v", name, err)
						utils.ExecCommand("virsh", "setvcpus", name, strconv.Itoa(vcpu), "--live")
					}
				} else {
					utils.ExecCommand("virsh", "setvcpus", name, strconv.Itoa(vcpu), "--live")
				}
			}
			// vcpu > liveMax 时仅更新持久化配置，需重启后生效
		}

		// 当 domain XML 中存在 CPU topology 时，virsh setvcpus --config --maximum
		// 会校验 sockets × dies × cores × threads == 目标 vcpu，不匹配则报错。
		// virsh define 同样会校验。因此当存在 topology 时，必须同时修改 vcpu 和 topology
		// 后 define 回去；不存在 topology 时仍用 virsh setvcpus 命令。
		if err := setVMCPUWithTopologySync(name, vcpu, maxVCPU); err != nil {
			return err
		}
	}

	// 修改内存
	if memoryMB > 0 {
		memKB := strconv.Itoa(memoryMB * 1024)
		if state == "running" {
			if IsLibvirtRPCAvailable() {
				if err := setDomainMemoryFlagsRPC(name, uint64(memoryMB*1024), domainMemLive); err != nil {
					log.Printf("[go-libvirt] 设置 %s live mem 失败，降级为 virsh: %v", name, err)
					utils.ExecCommand("virsh", "setmem", name, memKB, "--live")
				}
			} else {
				utils.ExecCommand("virsh", "setmem", name, memKB, "--live")
			}
		}
		var result *utils.CmdResult
		if IsLibvirtRPCAvailable() {
			err := setDomainMemoryFlagsRPC(name, uint64(memoryMB*1024), domainMemConfig|domainMemMaximum)
			if err != nil {
				log.Printf("[go-libvirt] 设置 %s maxmem 失败，降级为 virsh: %v", name, err)
				result = utils.ExecCommand("virsh", "setmaxmem", name, memKB, "--config")
				if result.Error != nil {
					return fmt.Errorf("设置最大内存失败: %s", result.Stderr)
				}
			}
		} else {
			result = utils.ExecCommand("virsh", "setmaxmem", name, memKB, "--config")
			if result.Error != nil {
				return fmt.Errorf("设置最大内存失败: %s", result.Stderr)
			}
		}
		if IsLibvirtRPCAvailable() {
			err := setDomainMemoryFlagsRPC(name, uint64(memoryMB*1024), domainMemConfig)
			if err != nil {
				log.Printf("[go-libvirt] 设置 %s config mem 失败，降级为 virsh: %v", name, err)
				result = utils.ExecCommand("virsh", "setmem", name, memKB, "--config")
				if result.Error != nil {
					return fmt.Errorf("设置内存失败: %s", result.Stderr)
				}
			}
		} else {
			result = utils.ExecCommand("virsh", "setmem", name, memKB, "--config")
			if result.Error != nil {
				return fmt.Errorf("设置内存失败: %s", result.Stderr)
			}
		}
	}

	return nil
}

// SetVMAutostart 设置虚拟机自动启动
func SetVMAutostart(name string, autostart bool) error {
	if err := EnsureVMNotMigrating(name, "设置开机自启"); err != nil {
		return err
	}
	if IsLibvirtRPCAvailable() {
		err := setDomainAutostartRPC(name, autostart)
		if err == nil {
			RefreshVMCacheByNameAsync(name)
			return nil
		}
		log.Printf("[go-libvirt] 设置 %s 自动启动失败，降级为 virsh: %v", name, err)
	}
	var args []string
	if autostart {
		args = []string{"autostart", name}
	} else {
		args = []string{"autostart", name, "--disable"}
	}
	result := utils.ExecCommand("virsh", args...)
	if result.Error != nil {
		return fmt.Errorf("设置自动启动失败: %s", result.Stderr)
	}
	RefreshVMCacheByNameAsync(name)
	return nil
}

// SetVMBootOrder 设置虚拟机启动顺序
func SetVMBootOrder(name string, bootOrder []string) error {
	if err := EnsureVMNotMigrating(name, "设置启动顺序"); err != nil {
		return err
	}
	// 先尝试 virt-xml 方式
	bootStr := strings.Join(bootOrder, ",")
	result := utils.ExecCommand("virt-xml", name, "--edit", "--boot", bootStr)
	if result.Error == nil {
		return nil
	}

	// 回退到直接编辑 XML 方式
	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	xmlContent := xmlResult.Stdout
	lines := strings.Split(xmlContent, "\n")
	var newLines []string
	inOS := false
	osEndInserted := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "<os>") || strings.Contains(trimmed, "<os ") {
			inOS = true
		}
		if inOS && strings.Contains(trimmed, "<boot dev=") {
			continue // 跳过旧的 boot 行
		}
		if inOS && strings.Contains(trimmed, "</os>") {
			if !osEndInserted {
				for _, dev := range bootOrder {
					newLines = append(newLines, fmt.Sprintf("    <boot dev='%s'/>", dev))
				}
				osEndInserted = true
			}
			inOS = false
		}
		newLines = append(newLines, line)
	}

	newXML := strings.Join(newLines, "\n")
	xmlPath := fmt.Sprintf("/tmp/_boot-%s.xml", name)
	utils.ExecShell(fmt.Sprintf("cat > %s << 'XMLEOF'\n%s\nXMLEOF", utils.ShellSingleQuote(xmlPath), newXML))
	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	utils.ExecShell(fmt.Sprintf("rm -f %s", utils.ShellSingleQuote(xmlPath)))
	if defineResult.Error != nil {
		return fmt.Errorf("设置启动顺序失败: %s", defineResult.Stderr)
	}
	return nil
}

// parseBootDevices 从 XML 中解析所有可引导设备
func parseBootDevices(xmlStr string, bootOrder []string) []BootDevice {
	var devices []BootDevice

	// 构建 boot order set（用于标记哪些设备类型被启用）
	bootOrderSet := make(map[string]int) // dev_type -> order
	for i, dev := range bootOrder {
		bootOrderSet[dev] = i + 1
	}

	// 解析磁盘设备
	diskRe := regexp.MustCompile(`(?s)<disk type='[^']*' device='([^']*)'>(.*?)</disk>`)
	sourceFileRe := regexp.MustCompile(`<source file='([^']*)'`)
	targetRe := regexp.MustCompile(`<target dev='([^']*)' bus='([^']*)'`)

	diskMatches := diskRe.FindAllStringSubmatch(xmlStr, -1)
	for _, m := range diskMatches {
		deviceType := m[1] // disk 或 cdrom
		deviceContent := m[2]

		bd := BootDevice{}
		if deviceType == "cdrom" {
			bd.Type = "cdrom"
		} else {
			bd.Type = "disk"
		}

		// 获取文件路径
		if sm := sourceFileRe.FindStringSubmatch(deviceContent); len(sm) > 1 {
			bd.File = sm[1]
		}

		// 获取设备名和总线
		if tm := targetRe.FindStringSubmatch(deviceContent); len(tm) > 2 {
			bd.Device = tm[1]
			bd.Bus = tm[2]
		}

		// 根据 OS 级别 boot order 判断是否启用及顺序
		// disk -> hd, cdrom -> cdrom
		bootKey := "hd"
		if bd.Type == "cdrom" {
			bootKey = "cdrom"
		}
		if order, ok := bootOrderSet[bootKey]; ok {
			bd.Enabled = true
			bd.Order = order
		}

		devices = append(devices, bd)
	}

	// 解析网络接口
	ifRe := regexp.MustCompile(`(?s)<interface type='[^']*'>(.*?)</interface>`)
	macRe := regexp.MustCompile(`<mac address='([^']*)'`)

	ifMatches := ifRe.FindAllStringSubmatch(xmlStr, -1)
	for _, m := range ifMatches {
		ifContent := m[1]
		bd := BootDevice{
			Type: "network",
		}
		if mm := macRe.FindStringSubmatch(ifContent); len(mm) > 1 {
			bd.File = mm[1]
		}

		if order, ok := bootOrderSet["network"]; ok {
			bd.Enabled = true
			bd.Order = order
		}

		devices = append(devices, bd)
	}

	return devices
}

// GetVMStats 获取虚拟机实时资源使用（用于监控图表）
func GetVMStats(name string) (*VmStats, error) {
	stats := &VmStats{}

	// CPU 使用率（两次采样计算差值）
	var cpuTime1 float64
	if IsLibvirtRPCAvailable() {
		if cput, err := getDomainCPUStatsRPC(name); err == nil {
			cpuTime1 = float64(cput) / 1e9
		} else {
			log.Printf("[go-libvirt] getDomainCPUStatsRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if cpuTime1 == 0 {
		re := regexp.MustCompile(`cpu_time\s+([\d.]+)\s+seconds`)
		cpuResult1 := utils.ExecCommand("virsh", "cpu-stats", name, "--total")
		if cpuResult1.Error == nil {
			if matches := re.FindStringSubmatch(cpuResult1.Stdout); len(matches) > 1 {
				cpuTime1, _ = strconv.ParseFloat(matches[1], 64)
			}
		}
	}

	// 获取 vCPU 数量
	vcpuCount := 1
	if IsLibvirtRPCAvailable() {
		if vcpu, _, _, _, err := getDomainInfoRPC(name); err == nil {
			vcpuCount = vcpu
			if vcpuCount <= 0 {
				vcpuCount = 1
			}
		} else {
			log.Printf("[go-libvirt] getDomainInfoRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if vcpuCount == 1 {
		infoResult := utils.ExecCommand("virsh", "dominfo", name)
		if infoResult.Error == nil {
			vcpuCount = parseInfoInt(infoResult.Stdout, "CPU(s):")
			if vcpuCount <= 0 {
				vcpuCount = 1
			}
		}
	}

	// 等待 1 秒再采样
	time.Sleep(time.Second)

	var cpuTime2 float64
	if IsLibvirtRPCAvailable() {
		if cput, err := getDomainCPUStatsRPC(name); err == nil {
			cpuTime2 = float64(cput) / 1e9
		} else {
			log.Printf("[go-libvirt] getDomainCPUStatsRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if cpuTime2 == 0 {
		re := regexp.MustCompile(`cpu_time\s+([\d.]+)\s+seconds`)
		cpuResult2 := utils.ExecCommand("virsh", "cpu-stats", name, "--total")
		if cpuResult2.Error == nil {
			if matches := re.FindStringSubmatch(cpuResult2.Stdout); len(matches) > 1 {
				cpuTime2, _ = strconv.ParseFloat(matches[1], 64)
			}
		}
	}
	if cpuTime2 > cpuTime1 {
		// CPU 使用率 = (差值 / 采样间隔 / vCPU数) * 100
		delta := cpuTime2 - cpuTime1
		if delta >= 0 {
			stats.CPUPercent = (delta / 1.0 / float64(vcpuCount)) * 100
			if stats.CPUPercent > 100 {
				stats.CPUPercent = 100
			}
		}
	}

	// 内存统计（通过 dommemstat）
	if IsLibvirtRPCAvailable() {
		if memStats, err := getDomainMemoryStatsRPC(name); err == nil {
			stats.MemTotal = int64(memStats["actual"])
			stats.MemUsed = stats.MemTotal - int64(memStats["unused"])
			if memStats["available"] > 0 {
				stats.MemUsed = stats.MemTotal - int64(memStats["usable"])
			}
		} else {
			log.Printf("[go-libvirt] getDomainMemoryStatsRPC(%s) 失败，降级为 virsh: %v", name, err)
		}
	}
	if stats.MemTotal == 0 {
		memResult := utils.ExecCommand("virsh", "dommemstat", name)
		if memResult.Error == nil {
			stats.MemTotal = parseMemStat(memResult.Stdout, "actual")
			stats.MemUsed = stats.MemTotal - parseMemStat(memResult.Stdout, "unused")
			if parseMemStat(memResult.Stdout, "available") > 0 {
				stats.MemUsed = stats.MemTotal - parseMemStat(memResult.Stdout, "usable")
			}
		}
	}

	// 网络统计 - 动态获取网卡接口名
	ifListResult := utils.ExecCommand("virsh", "domiflist", name)
	if ifListResult.Error == nil {
		ifLines := strings.Split(ifListResult.Stdout, "\n")
		for _, ifLine := range ifLines {
			fields := strings.Fields(ifLine)
			if len(fields) >= 2 && fields[0] != "Interface" && !strings.HasPrefix(ifLine, "-") {
				ifName := fields[0]
				if ifName == "-" || ifName == "" {
					continue
				}
				var netRxBytes, netTxBytes int64
				if IsLibvirtRPCAvailable() {
					if rx, tx, err := getDomainInterfaceStatsRPC(name, ifName); err == nil {
						netRxBytes = rx
						netTxBytes = tx
					} else {
						log.Printf("[go-libvirt] getDomainInterfaceStatsRPC(%s, %s) 失败，降级为 virsh: %v", name, ifName, err)
					}
				}
				if netRxBytes == 0 && netTxBytes == 0 {
					netResult := utils.ExecCommand("virsh", "domifstat", name, ifName)
					if netResult.Error == nil {
						netRxBytes = parseIfStat(netResult.Stdout, "rx_bytes")
						netTxBytes = parseIfStat(netResult.Stdout, "tx_bytes")
					}
				}
				stats.NetRxBytes += netRxBytes
				stats.NetTxBytes += netTxBytes
			}
		}
	}

	// 磁盘 I/O - 动态获取第一个磁盘设备
	blkListResult := utils.ExecCommand("virsh", "domblklist", name)
	if blkListResult.Error == nil {
		blkLines := strings.Split(blkListResult.Stdout, "\n")
		for _, blkLine := range blkLines {
			fields := strings.Fields(blkLine)
			if len(fields) >= 2 && fields[0] != "Target" && !strings.HasPrefix(blkLine, "-") {
				dev := fields[0]
				// 跳过 cdrom 设备（通常是 sda/hdc 且路径为 iso）
				if strings.HasSuffix(fields[1], ".iso") {
					continue
				}
				var diskRdBytes, diskWrBytes int64
				if IsLibvirtRPCAvailable() {
					if rd, wr, err := getDomainBlockStatsRPC(name, dev); err == nil {
						diskRdBytes = rd
						diskWrBytes = wr
					} else {
						log.Printf("[go-libvirt] getDomainBlockStatsRPC(%s, %s) 失败，降级为 virsh: %v", name, dev, err)
					}
				}
				if diskRdBytes == 0 && diskWrBytes == 0 {
					blkResult := utils.ExecCommand("virsh", "domblkstat", name, dev)
					if blkResult.Error == nil {
						diskRdBytes = parseBlkStat(blkResult.Stdout, "rd_bytes")
						diskWrBytes = parseBlkStat(blkResult.Stdout, "wr_bytes")
					}
				}
				stats.DiskRdBytes += diskRdBytes
				stats.DiskWrBytes += diskWrBytes
				break // 只取第一个磁盘
			}
		}
	}

	return stats, nil
}

// GetHostStats 获取宿主机资源信息
func GetHostStats() (*HostStats, error) {
	stats := &HostStats{}

	// CPU 核心数
	cpuResult := utils.ExecShell("nproc")
	if cpuResult.Error == nil {
		stats.CPUCount, _ = strconv.Atoi(strings.TrimSpace(cpuResult.Stdout))
	}

	// CPU 使用率
	cpuUsageResult := utils.ExecShell(`top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1`)
	if cpuUsageResult.Error == nil {
		stats.CPUPercent, _ = strconv.ParseFloat(strings.TrimSpace(cpuUsageResult.Stdout), 64)
	}

	// 内存信息
	memResult := utils.ExecShell(`free -k | awk 'NR==2{print $2,$3,$4}'`)
	if memResult.Error == nil {
		parts := strings.Fields(memResult.Stdout)
		if len(parts) >= 3 {
			stats.MemTotal, _ = strconv.ParseInt(parts[0], 10, 64)
			stats.MemUsed, _ = strconv.ParseInt(parts[1], 10, 64)
			stats.MemFree, _ = strconv.ParseInt(parts[2], 10, 64)
		}
	}

	// Swap 信息
	swapResult := utils.ExecShell(`free -k | awk 'NR==3{print $2,$3,$4}'`)
	if swapResult.Error == nil {
		parts := strings.Fields(swapResult.Stdout)
		if len(parts) >= 3 {
			stats.SwapTotal, _ = strconv.ParseInt(parts[0], 10, 64)
			stats.SwapUsed, _ = strconv.ParseInt(parts[1], 10, 64)
			stats.SwapFree, _ = strconv.ParseInt(parts[2], 10, 64)
		}
	}

	// KSM 内存合并页数
	ksmSharedResult := utils.ExecShell(`cat /sys/kernel/mm/ksm/pages_shared 2>/dev/null`)
	if ksmSharedResult.Error == nil {
		stats.KSMPagesShared, _ = strconv.ParseInt(strings.TrimSpace(ksmSharedResult.Stdout), 10, 64)
	}
	ksmSharingResult := utils.ExecShell(`cat /sys/kernel/mm/ksm/pages_sharing 2>/dev/null`)
	if ksmSharingResult.Error == nil {
		stats.KSMPagesSharing, _ = strconv.ParseInt(strings.TrimSpace(ksmSharingResult.Stdout), 10, 64)
	}

	// 磁盘 IO 延迟（毫秒），通过 iostat 1 秒间隔采样获取实时 r_await
	ioLatencyResult := utils.ExecShell(`iostat -d -x 1 2 2>/dev/null | awk 'BEGIN{sec=0} /^Device/{sec++; next} sec==2 && $1 ~ /^(sd|vd|nvme|dm-)/ && $6 != "" {s+=$6; c++} END{if(c>0) printf "%.1f", s/c; else print "0"}'`)
	if ioLatencyResult.Error == nil {
		stats.DiskIOLatencyMs, _ = strconv.ParseFloat(strings.TrimSpace(ioLatencyResult.Stdout), 64)
	}

	// 磁盘信息（根分区）
	diskResult := utils.ExecShell(`df -k / | awk 'NR==2{print $2,$3,$4}'`)
	if diskResult.Error == nil {
		parts := strings.Fields(diskResult.Stdout)
		if len(parts) >= 3 {
			stats.DiskTotal, _ = strconv.ParseInt(parts[0], 10, 64)
			stats.DiskUsed, _ = strconv.ParseInt(parts[1], 10, 64)
			stats.DiskFree, _ = strconv.ParseInt(parts[2], 10, 64)
		}
	}

	// 宿主机网络 IO (累加常见物理网卡，排除 virbr, vnet, docker, lo)
	netIOResult := utils.ExecShell(`awk 'NR>2 {if ($1 !~ /lo|virbr|vnet|docker/) {rx+=$2; tx+=$10}} END {print rx, tx}' /proc/net/dev`)
	if netIOResult.Error == nil {
		parts := strings.Fields(netIOResult.Stdout)
		if len(parts) >= 2 {
			stats.NetRxBytes, _ = strconv.ParseInt(parts[0], 10, 64)
			stats.NetTxBytes, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}

	// 宿主机磁盘 IO
	// 优先通过 lsblk 获取 type=disk 的顶层设备，再从 /proc/diskstats 汇总累计扇区数。
	if diskRdBytes, diskWrBytes, err := collectHostDiskIOBytes(); err == nil {
		stats.DiskRdBytes = diskRdBytes
		stats.DiskWrBytes = diskWrBytes
	}

	// 主机名
	hostnameResult := utils.ExecCommand("hostname")
	if hostnameResult.Error == nil {
		stats.Hostname = strings.TrimSpace(hostnameResult.Stdout)
	}

	// 运行时间
	uptimeResult := utils.ExecShell(`uptime -p`)
	if uptimeResult.Error == nil {
		stats.Uptime = strings.TrimSpace(uptimeResult.Stdout)
	}

	// 虚拟机数量
	runningResult := utils.ExecShell(`virsh list --name 2>/dev/null | grep -v '^$' | wc -l`)
	if runningResult.Error == nil {
		stats.VMRunning, _ = strconv.Atoi(strings.TrimSpace(runningResult.Stdout))
	}
	totalResult := utils.ExecShell(`virsh list --all --name 2>/dev/null | grep -v '^$' | wc -l`)
	if totalResult.Error == nil {
		stats.VMTotal, _ = strconv.Atoi(strings.TrimSpace(totalResult.Stdout))
	}

	return stats, nil
}

// ==================== 辅助函数 ====================

type diskInfoResult struct {
	device   string
	path     string
	size     string
	template string
}

type netInfoResult struct {
	network  string
	mac      string
	nicModel string
}

// getVMIP 获取虚拟机 IP 地址
// VPC VM 优先使用 VPC 静态绑定或租约；普通 VM 依次尝试 Guest Agent → ARP 表 → DHCP 租约 → 静态绑定
// isRunning 表示虚拟机是否在运行，关机状态跳过前三种方式（避免无效命令调用）
func getVMIP(name string, isRunning bool) string {
	ipRe := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)`)

	if sw, ok := getVPCSwitchForVM(name); ok {
		if ip := GetVPCLeaseIPForVM(name); ip != "" {
			return ip
		}
		if isRunning {
			if ip := getVMIPFromDomifaddrSource(name, "agent", ipRe, sw.CIDR, false); ip != "" {
				return ip
			}
			if ip := getVMIPFromDomifaddrSource(name, "arp", ipRe, sw.CIDR, true); ip != "" {
				return ip
			}
			if mac := getFirstVMMAC(name); mac != "" {
				if ip := GetOVSLeaseIPByMAC(mac); ip != "" && IPInCIDR(ip, sw.CIDR) {
					return ip
				}
				if ip := getVPCStaticIPFromAllHostsByMAC(mac, sw.CIDR); ip != "" {
					return ip
				}
				if ip := getHostNeighborIPByMAC(mac, sw.CIDR, true); ip != "" {
					return ip
				}
			}
			// VPC 所有方式均失败时，尝试主动 ARP 扫描（桥接直通模式兜底）
			if ip := getVMIPByActiveScan(name); ip != "" {
				return ip
			}
			// 桥接/直通模式兜底：ARP 不加 CIDR 过滤
			// VPC CIDR 内未命中时，VM 的实际 IP 可能由上游路由器分配（桥接模式），CIDR 范围外仍可获取
			if ip := execDomifaddrARP(name, ipRe); ip != "" {
				return ip
			}
		}
		return ""
	}

	if isRunning {
		// 方式1: Guest Agent（最准确，但需要虚拟机安装 qemu-guest-agent）
		result := utils.ExecCommand("virsh", "domifaddr", name, "--source", "agent")
		if result.Error == nil {
			// 过滤掉 127.0.0.1 环回地址，取第一个有效 IP
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			for _, m := range allMatches {
				if m[1] != "127.0.0.1" {
					return m[1]
				}
			}
		}

		// 方式2: ARP 表（反映当前实际网络通信状态，比 DHCP 租约更可靠）
		result = utils.ExecCommand("virsh", "domifaddr", name, "--source", "arp")
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) == 1 {
				return allMatches[0][1]
			} else if len(allMatches) > 1 {
				// ARP 表有多个 IP（可能存在残留），用 ping 验证存活性
				for i := len(allMatches) - 1; i >= 0; i-- {
					ip := allMatches[i][1]
					pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", ip)
					if pingResult.ExitCode == 0 {
						return ip
					}
				}
				// 都不通则返回最后一个
				return allMatches[len(allMatches)-1][1]
			}
		}

		// 方式3: VPC dnsmasq 租约（VM 已绑定 VPC 时优先读取对应交换机租约）
		if ip := GetVPCLeaseIPForVM(name); ip != "" {
			return ip
		}

		// 方式4: OVS dnsmasq 租约（OVS bridge 不属于 libvirt managed network）
		if mac := getFirstVMMAC(name); mac != "" {
			if ip := GetOVSLeaseIPByMAC(mac); ip != "" {
				return ip
			}
			if ip := getHostNeighborIPByMAC(mac, "", true); ip != "" {
				return ip
			}
		}

		// 方式5: libvirt DHCP 租约（取最后一条，即最新租约，避免旧租约干扰）
		result = utils.ExecCommand("virsh", "domifaddr", name, "--source", "lease")
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) > 0 {
				return allMatches[len(allMatches)-1][1]
			}
		}

		// 方式6: 默认模式（取最后一条匹配，避免旧租约从第一条返回）
		result = utils.ExecCommand("virsh", "domifaddr", name)
		if result.Error == nil {
			allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
			if len(allMatches) > 0 {
				return allMatches[len(allMatches)-1][1]
			}
		}
	}

	// 方式7: 从 OVS 静态绑定中查找（兜底方案，不依赖虚拟机运行状态）
	mac := getFirstVMMAC(name)
	if mac != "" {
		if ip := GetOVSStaticIPByMAC(mac); ip != "" {
			return ip + " (静态)"
		}
	}

	// 方式8: 桥接模式主动 ARP 扫描（兜底，有频率限制）
	if isRunning {
		if ip := getVMIPByActiveScan(name); ip != "" {
			return ip
		}
	}

	return ""
}

// getVMIPStatus 获取虚拟机 IP 状态（用于前端展示"无法获取"提示）
// 返回 "" 表示正常，返回非空表示无法获取的原因
func getVMIPStatus(name string, isRunning bool) string {
	if !isRunning {
		return "shut_off"
	}
	if hasVMBridgeVLANTag(name) {
		return "vlan_bridge"
	}
	return ""
}

func getVPCStaticIPFromAllHostsByMAC(mac, cidr string) string {
	hosts, err := ListAllVPCStaticHosts()
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if strings.EqualFold(host.MAC, mac) && strings.TrimSpace(host.IP) != "" {
			if cidr == "" || IPInCIDR(host.IP, cidr) {
				return host.IP
			}
		}
	}
	return ""
}

func getHostNeighborIPByMAC(mac, cidr string, verifyPing bool) string {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return ""
	}
	result := utils.ExecCommand("ip", "neigh", "show")
	if result.Error != nil {
		return ""
	}
	candidates := parseHostNeighborIPsByMAC(result.Stdout, mac, cidr)
	if len(candidates) == 0 {
		return ""
	}
	if verifyPing {
		for i := len(candidates) - 1; i >= 0; i-- {
			pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", candidates[i])
			if pingResult.ExitCode == 0 {
				return candidates[i]
			}
		}
	}
	return candidates[len(candidates)-1]
}

func parseHostNeighborIPsByMAC(text, mac, cidr string) []string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	var ips []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		ip := fields[0]
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "lladdr" && strings.EqualFold(fields[i+1], mac) {
				if cidr == "" || IPInCIDR(ip, cidr) {
					ips = append(ips, ip)
				}
				break
			}
		}
	}
	return ips
}

// ==================== 主动 ARP 扫描（桥接模式 IP 发现） ====================

// 主动扫描子网级缓存
var (
	activeScanMu         sync.Mutex
	lastActiveScanTime   time.Time
	lastActiveScanBridge string
	lastActiveScanMacIPs map[string]string // MAC -> IP 映射缓存
)

// getVMBridgeInterface 获取虚拟机第一个网桥类型接口的桥名称
func getVMBridgeInterface(vmName string) string {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == "bridge" && fields[2] != "Source" {
			return fields[2]
		}
	}
	return ""
}

// getVMBridgeVnetInterface 获取虚拟机第一个网桥接口的 vnet 接口名
func getVMBridgeVnetInterface(vmName string) string {
	result := utils.ExecCommand("virsh", "domiflist", vmName)
	if result.Error != nil {
		return ""
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[1] == "bridge" && strings.HasPrefix(fields[0], "vnet") {
			return fields[0]
		}
	}
	return ""
}

// hasVMBridgeVLANTag 检测虚拟机桥接接口是否带有 OVS VLAN tag
func hasVMBridgeVLANTag(vmName string) bool {
	// 通过 domiflist 获取 vnet 接口名
	vnet := getVMBridgeVnetInterface(vmName)
	if vnet == "" {
		return false
	}
	// 通过 OVS 查询该端口是否有 VLAN tag
	result := utils.ExecShell(fmt.Sprintf(
		"ovs-vsctl list port %s 2>/dev/null | awk '$1==\"tag\" {print $3}'",
		utils.ShellSingleQuote(vnet)))
	tag := strings.TrimSpace(result.Stdout)
	if tag == "" || tag == "[]" {
		return false
	}
	return true
}

// getInterfaceCIDR 获取网络接口的 IPv4 CIDR 子网
func getInterfaceCIDR(iface string) string {
	result := utils.ExecShell(fmt.Sprintf("ip -4 -o addr show %s 2>/dev/null | awk '{print $4}' | head -1", utils.ShellSingleQuote(iface)))
	cidr := strings.TrimSpace(result.Stdout)
	if cidr == "" {
		return ""
	}
	return cidr
}

// parseARPScanMacIPs 从 arp-scan 输出中解析 MAC -> IP 映射表
func parseARPScanMacIPs(output string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			mac := strings.ToLower(fields[1])
			if matched, _ := regexp.MatchString(`^([0-9a-f]{2}:){5}[0-9a-f]{2}$`, mac); matched {
				m[mac] = ip
			}
		}
	}
	return m
}

// getAllHostNeighborMacIPs 从 ip neigh 中获取指定网桥的所有 MAC -> IP 映射
func getAllHostNeighborMacIPs(bridge string) map[string]string {
	result := utils.ExecShell(fmt.Sprintf("ip neigh show dev %s 2>/dev/null", utils.ShellSingleQuote(bridge)))
	if result.Error != nil {
		return nil
	}
	m := make(map[string]string)
	macRe := regexp.MustCompile(`([0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2})`)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		macMatches := macRe.FindAllString(strings.ToLower(line), 1)
		if len(macMatches) > 0 {
			m[macMatches[0]] = ip
		}
	}
	return m
}

// getVMIPByActiveScan 通过主动 ARP 扫描获取桥接模式虚拟机的 IP
// 子网级缓存：同一网桥 12 秒内共享一次扫描结果，所有 VM 命中缓存无需重新扫描
// 桥接 VLAN 模式不扫描，直接返回空
func getVMIPByActiveScan(vmName string) string {
	// 桥接 VLAN 模式：arp-scan/nmap 均无法穿透 VLAN，不浪费扫描资源
	if hasVMBridgeVLANTag(vmName) {
		return ""
	}

	mac := getFirstVMMAC(vmName)
	if mac == "" {
		return ""
	}
	mac = strings.ToLower(strings.TrimSpace(mac))

	bridge := getVMBridgeInterface(vmName)
	if bridge == "" {
		return ""
	}

	activeScanMu.Lock()
	if lastActiveScanBridge == bridge && time.Since(lastActiveScanTime) < 12*time.Second && lastActiveScanMacIPs != nil {
		cached := lastActiveScanMacIPs[mac]
		activeScanMu.Unlock()
		return cached
	}
	activeScanMu.Unlock()

	cidr := getInterfaceCIDR(bridge)
	if cidr == "" {
		return ""
	}

	// 同网段内只需要一个机器来 ping 验证 VM 是否已分配到 IP
	// 另外注意首次扫描到的 IP 可能比 VM 真实 IP 晚几秒
	// 所以如果没找到结果，也不缓存空结果，允许 3 秒后 SSE 重试
	var macIPs map[string]string

	// 方式1: arp-scan（最快，直接输出完整 MAC-IP 映射）
	result := utils.ExecCommandWithTimeout("arp-scan",
		20*time.Second,
		"--interface="+bridge,
		"--localnet",
		"--quiet",
		"--ignoredups")
	if result.Error == nil && result.Stdout != "" {
		macIPs = parseARPScanMacIPs(result.Stdout)
		if len(macIPs) > 0 {
			activeScanMu.Lock()
			lastActiveScanTime = time.Now()
			lastActiveScanBridge = bridge
			lastActiveScanMacIPs = macIPs
			activeScanMu.Unlock()
			return macIPs[mac]
		}
	}

	// 方式2: nmap -sn -PR 强制 ARP 探测，再用 ip neigh 构建映射
	_ = utils.ExecCommandWithTimeout("nmap", 15*time.Second,
		"-sn", "-PR", cidr,
		"--max-retries", "1",
		"--host-timeout", "1s")
	time.Sleep(500 * time.Millisecond)
	macIPs = getAllHostNeighborMacIPs(bridge)
	if len(macIPs) > 0 {
		activeScanMu.Lock()
		lastActiveScanTime = time.Now()
		lastActiveScanBridge = bridge
		lastActiveScanMacIPs = macIPs
		activeScanMu.Unlock()
		return macIPs[mac]
	}

	// 没扫到任何结果，不缓存（下次 SSE 再试）
	return ""
}

// execDomifaddrARP 通过 virsh domifaddr --source arp 获取 VM IP（不限制 CIDR）
// 作为 VPC 桥接模式的兜底，用于 VM 实际 IP 不在 VPC CIDR 内的情况
func execDomifaddrARP(name string, ipRe *regexp.Regexp) string {
	result := utils.ExecCommand("virsh", "domifaddr", name, "--source", "arp")
	if result.Error != nil {
		return ""
	}
	allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
	if len(allMatches) == 0 {
		return ""
	}
	// 单 IP 直接返回
	if len(allMatches) == 1 {
		return allMatches[0][1]
	}
	// 多 IP 用 ping 验证存活性
	for i := len(allMatches) - 1; i >= 0; i-- {
		ip := allMatches[i][1]
		pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", ip)
		if pingResult.ExitCode == 0 {
			return ip
		}
	}
	// 都不通则返回最后一个
	return allMatches[len(allMatches)-1][1]
}

func getVMIPFromDomifaddrSource(name, source string, ipRe *regexp.Regexp, cidr string, verifyPing bool) string {
	result := utils.ExecCommand("virsh", "domifaddr", name, "--source", source)
	if result.Error != nil {
		return ""
	}
	allMatches := ipRe.FindAllStringSubmatch(result.Stdout, -1)
	var candidates []string
	for _, match := range allMatches {
		if len(match) < 2 || match[1] == "127.0.0.1" {
			continue
		}
		if cidr != "" && !IPInCIDR(match[1], cidr) {
			continue
		}
		candidates = append(candidates, match[1])
	}
	if len(candidates) == 0 {
		return ""
	}
	if verifyPing {
		for i := len(candidates) - 1; i >= 0; i-- {
			pingResult := utils.ExecCommandWithTimeout("ping", 2*time.Second, "-c", "1", "-W", "1", candidates[i])
			if pingResult.ExitCode == 0 {
				return candidates[i]
			}
		}
	}
	return candidates[0]
}

// getVMDiskInfo 获取虚拟机磁盘信息
func getVMDiskInfo(name string) diskInfoResult {
	info := diskInfoResult{}

	// 获取磁盘路径
	blkResult := utils.ExecCommand("virsh", "domblklist", name)
	if blkResult.Error != nil {
		return info
	}

	lines := strings.Split(blkResult.Stdout, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] != "-" && fields[1] != "Source" && !strings.HasPrefix(line, "-") {
			info.device = fields[0]
			info.path = fields[1]
			break
		}
	}

	if info.path == "" {
		return info
	}

	// 获取磁盘配置容量（默认展示虚拟机设置大小，而非实际占用）
	qemuInfoResult := utils.ExecShell(fmt.Sprintf("qemu-img info --output=json -U %s 2>/dev/null", utils.ShellSingleQuote(info.path)))
	if qemuInfoResult.Error == nil {
		info.size = parseQemuInfoGB(qemuInfoResult.Stdout, "virtual-size")
		if info.size != "-" && info.size != "" {
			info.size += " GB"
		}
		backing := strings.TrimSpace(parseQemuInfoStr(qemuInfoResult.Stdout, "backing-filename"))
		if backing != "" {
			parts := strings.Split(backing, "/")
			templateFile := parts[len(parts)-1]
			info.template = strings.TrimSuffix(templateFile, ".qcow2")
		}
	}
	if (info.size == "" || info.size == "-") && info.device != "" {
		blkInfoResult := utils.ExecCommand("virsh", "domblkinfo", name, info.device)
		if blkInfoResult.Error == nil {
			size := parseBlkInfoGB(blkInfoResult.Stdout, "Capacity:")
			if size != "-" && size != "" {
				info.size = size + " GB"
			}
		}
	}

	// 获取 backing file（模板来源）
	if info.template == "" {
		backingResult := utils.ExecShell(fmt.Sprintf("qemu-img info -U %s 2>/dev/null | grep 'backing file:' | awk '{print $3}'", utils.ShellSingleQuote(info.path)))
		if backingResult.Error == nil {
			backing := strings.TrimSpace(backingResult.Stdout)
			if backing != "" {
				parts := strings.Split(backing, "/")
				templateFile := parts[len(parts)-1]
				info.template = strings.TrimSuffix(templateFile, ".qcow2")
			}
		}
	}

	return info
}

// getVMNetworkInfo 获取虚拟机网络信息
func getVMNetworkInfo(name string) netInfoResult {
	info := netInfoResult{network: "unknown"}

	// 从 domiflist 获取
	result := utils.ExecCommand("virsh", "domiflist", name)
	if result.Error != nil {
		return info
	}

	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] != "Interface" && !strings.HasPrefix(line, "-") {
			// fields: Interface Type Source Model MAC
			switch fields[1] {
			case "network":
				info.network = "nat"
			case "bridge":
				info.network = "bridge"
			default:
				info.network = fields[1]
			}
			info.nicModel = fields[3] // Model 列
			info.mac = fields[4]
			break
		}
	}

	return info
}

// SetVMNicModel 修改虚拟机网卡模型（通过编辑 XML）
func SetVMNicModel(name, nicModel string) error {
	state := strings.TrimSpace(utils.ExecCommand("virsh", "domstate", name).Stdout)
	if state == "running" {
		return fmt.Errorf("修改网卡类型需要先关机")
	}

	// 获取当前 XML
	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	// 替换 model type
	xmlStr := xmlResult.Stdout
	lines := strings.Split(xmlStr, "\n")
	var newLines []string
	inInterface := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "<interface ") {
			inInterface = true
		}
		if inInterface && strings.Contains(trimmed, "<model type='") {
			// 替换为新的网卡模型
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			line = fmt.Sprintf("%s<model type='%s'/>", indent, nicModel)
			inInterface = false // 只修改第一个网卡
		}
		if inInterface && strings.Contains(trimmed, "</interface>") {
			inInterface = false
		}
		newLines = append(newLines, line)
	}

	newXML := strings.Join(newLines, "\n")
	xmlPath := fmt.Sprintf("/tmp/_nic-%s.xml", name)
	writeResult := utils.ExecShell(fmt.Sprintf("cat > %s << 'XMLEOF'\n%s\nXMLEOF", utils.ShellSingleQuote(xmlPath), newXML))
	if writeResult.Error != nil {
		return fmt.Errorf("写入 XML 失败: %s", writeResult.Stderr)
	}

	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	utils.ExecShell(fmt.Sprintf("rm -f %s", utils.ShellSingleQuote(xmlPath)))
	if defineResult.Error != nil {
		return fmt.Errorf("修改网卡类型失败: %s", defineResult.Stderr)
	}

	return nil
}

// parseInfoInt 从 virsh dominfo 输出中解析整数值
func parseInfoInt(output, key string) int {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// 取最后或倒数第二个字段（带 KiB 单位）
				valStr := parts[len(parts)-2]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
				// 尝试最后一个字段
				valStr = parts[len(parts)-1]
				if val, err := strconv.Atoi(valStr); err == nil {
					return val
				}
			}
		}
	}
	return 0
}

// parseInfoValue 从 virsh dominfo 输出中解析文本值
func parseInfoValue(output, key string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// parseMemStat 从 dommemstat 输出解析内存值（KB）
func parseMemStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			val, _ := strconv.ParseInt(fields[1], 10, 64)
			return val
		}
	}
	return 0
}

// parseIfStat 从 domifstat 输出解析网络值
func parseIfStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[len(fields)-1], 10, 64)
				return val
			}
		}
	}
	return 0
}

// parseBlkStat 从 domblkstat 输出解析磁盘值
func parseBlkStat(output, key string) int64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, key) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[len(fields)-1], 10, 64)
				return val
			}
		}
	}
	return 0
}
