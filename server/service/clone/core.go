package clone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/ip_resolver"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/taskqueue"
	"kvm_console/utils"

	"github.com/digitalocean/go-libvirt"
)

// CheckCanceled 检查任务是否已被取消，如果已取消执行清理并返回错误
func CheckCanceled(ctx context.Context, vmName, diskPath string) error {
	select {
	case <-ctx.Done():
		logger.App.Info("任务被取消，开始清理资源", "vm", vmName, "disk", diskPath)
		if vmName != "" {
			_ = libvirt_rpc.DestroyDomainRPC(vmName)
			_ = libvirt_rpc.UndefineDomainRPC(vmName, libvirt.DomainUndefineNvram|libvirt.DomainUndefineSnapshotsMetadata)
		}
		if diskPath != "" {
			if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
				logger.App.Warn("取消任务时删除磁盘失败", "error", err)
			}
		}
		return taskqueue.ErrTaskCanceled
	default:
		return nil
	}
}

// CloneVM 链式克隆虚拟机（主逻辑，对应 vm-linked-clone.sh clone）
func CloneVM(ctx context.Context, params *CloneParams, progressFn func(int, string)) (*CloneResult, error) {
	if err := D.ValidateVMName(params.Name); err != nil {
		return nil, err
	}
	templateDir := config.GlobalConfig.TemplateDir
	cloneDir, resolvedStoragePoolID, err := D.ResolveVMStorageDir(params.StoragePoolID, params.IsAdmin)
	if err != nil {
		return nil, err
	}
	params.StoragePoolID = resolvedStoragePoolID

	// 默认值
	if params.Network == "" {
		params.Network = config.GlobalConfig.DefaultNetwork
	}
	params.NicModel = D.NormalizeVMNicModel(params.NicModel)
	params.DiskBus = D.NormalizeVMDiskBus(params.DiskBus)
	params.Hostname = strings.TrimSpace(params.Hostname)
	params.User = strings.TrimSpace(params.User)
	if params.Hostname == "" {
		params.Hostname = GenerateRandomCloneHostname()
	}

	// 确定模板路径
	templatePath := filepath.Join(templateDir, params.Template+".qcow2")
	if !utils.FileExists(templatePath) {
		return nil, fmt.Errorf("模板不存在: %s", params.Template)
	}

	// 检查虚拟机是否已存在
	if _, _, _, _, err := libvirt_rpc.GetDomainInfoRPC(params.Name); err == nil {
		return nil, fmt.Errorf("虚拟机 '%s' 已存在", params.Name)
	}

	cloneDisk := filepath.Join(cloneDir, params.Name+".qcow2")

	// 从模板元数据获取类型和凭据
	meta := D.GetTemplateMeta(params.Template)
	if params.DiskBus == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.DiskBus) != "" {
		params.DiskBus = D.NormalizeVMDiskBus(meta.DefaultConfig.DiskBus)
	}
	if strings.TrimSpace(params.VideoModel) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.VideoModel) != "" {
		params.VideoModel = strings.TrimSpace(meta.DefaultConfig.VideoModel)
	}
	if !params.IsAdmin {
		params.CPULimitPercent = D.VMCPULimitUnlimited
	}
	if err := D.ValidateVMCPULimitPercent(params.CPULimitPercent); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.CPUTopologyMode) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.CPUTopologyMode) != "" {
		params.CPUTopologyMode = strings.TrimSpace(meta.DefaultConfig.CPUTopologyMode)
	}
	params.CPUTopologyMode = D.NormalizeVMCPUTopologyMode(params.CPUTopologyMode)
	if strings.TrimSpace(params.FirstBootRebootMode) == "" && meta.DefaultConfig != nil && strings.TrimSpace(meta.DefaultConfig.FirstBootRebootMode) != "" {
		params.FirstBootRebootMode = strings.TrimSpace(meta.DefaultConfig.FirstBootRebootMode)
	}
	params.FirstBootRebootMode = D.NormalizeVMFirstBootRebootMode(params.FirstBootRebootMode)

	// 确定模板类型（优先使用 CloneParams 中显式指定的，否则用元数据）
	tplType := strings.ToLower(params.TemplateType)
	if tplType == "" {
		tplType = meta.Type
	}
	if tplType == "" {
		tplType = "linux"
	}
	params.TemplateType = tplType
	if params.DiskBus == "" {
		params.DiskBus = "virtio"
	}

	// 凭据：优先用 CloneParams 中的，否则用元数据中的
	if params.TemplateRootPass == "" && meta.RootPassword != "" {
		params.TemplateRootPass = meta.RootPassword
	}
	if params.TemplateUser == "" && meta.TemplateUser != "" {
		params.TemplateUser = meta.TemplateUser
	}
	params.User = NormalizeCloneUsernameForTemplate(tplType, params.User)
	if err := ValidateCloneCredentialsForTemplate(tplType, params.Hostname, params.User, params.Password, false); err != nil {
		return nil, err
	}
	resolvedDiskSize, err := D.ResolveCloneDiskSizeGB(params.Template, params.DiskSize)
	if err != nil {
		return nil, err
	}
	params.DiskSize = resolvedDiskSize

	isWindows := tplType == "windows"
	isFnOS := tplType == "fnos"
	isOther := tplType == "other"

	// 克隆前存储空间预检查
	if err := D.CheckStorageSpace(filepath.Dir(cloneDisk), int64(params.DiskSize)*1024+1024); err != nil {
		return nil, err
	}

	if isOther {
		// ===== Other 类型：直接复制模板磁盘，不做任何初始化 =====
		progressFn(10, "复制模板磁盘（完整复制）...")
		result := utils.ExecCommandLongRunning("cp", "--sparse=always", templatePath, cloneDisk)
		if result.Error != nil {
			return nil, fmt.Errorf("复制模板磁盘失败: %s", result.Stderr)
		}

		// 如果指定了磁盘大小，进行扩容
		if params.DiskSize > 0 {
			utils.ExecShell(fmt.Sprintf("qemu-img resize %s %dG", utils.ShellSingleQuote(cloneDisk), params.DiskSize))
		}
	} else if params.CloneMode == "full" {
		// ===== 完整克隆：将模板数据完整复制到新磁盘，脱离链式依赖 =====
		progressFn(10, "创建完整克隆磁盘（脱离链式条件）...")
		var convertCmd string
		if params.DiskSize > 0 {
			convertCmd = fmt.Sprintf("qemu-img convert -f qcow2 -O qcow2 %s %s && qemu-img resize %s %dG",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk), utils.ShellSingleQuote(cloneDisk), params.DiskSize)
		} else {
			convertCmd = fmt.Sprintf("qemu-img convert -f qcow2 -O qcow2 %s %s",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk))
		}
		result := utils.ExecShellWithTimeout(convertCmd, 2*time.Hour)
		if result.Error != nil {
			return nil, fmt.Errorf("创建完整克隆磁盘失败: %s", result.Stderr)
		}
	} else {
		// ===== 链式克隆（Linux/Windows/FnOS） =====
		progressFn(10, "创建链式克隆磁盘...")
		var createCmd string
		if params.DiskSize > 0 {
			createCmd = fmt.Sprintf("qemu-img create -f qcow2 -F qcow2 -b %s %s %dG",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk), params.DiskSize)
		} else {
			createCmd = fmt.Sprintf("qemu-img create -f qcow2 -F qcow2 -b %s %s",
				utils.ShellSingleQuote(templatePath), utils.ShellSingleQuote(cloneDisk))
		}
		result := utils.ExecShell(createCmd)
		if result.Error != nil {
			return nil, fmt.Errorf("创建克隆磁盘失败: %s", result.Stderr)
		}
	}

	// 检查取消
	if err := CheckCanceled(ctx, "", cloneDisk); err != nil {
		return nil, err
	}

	if isFnOS {
		if err := D.PrepareFnOSSystemDiskExpansion(ctx, cloneDisk, progressFn); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
		if err := cloneFnOS(params, cloneDisk, progressFn); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
	}
	if tplType == "linux" {
		progressFn(25, "重置 Linux 首次启动身份...")
		if err := prepareLinuxCloneFirstBootIdentity(params, cloneDisk); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
		params.LinuxIdentityPrepared = true
	}
	if isWindows {
		if err := D.PrepareWindowsSystemDiskExpansion(ctx, cloneDisk, progressFn); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
	}

	progressFn(30, "创建虚拟机定义...")
	if err := D.EnsureOVSNetworkReady(); err != nil {
		_ = os.Remove(cloneDisk)
		return nil, err
	}

	memoryMeta, ramMB, _, err := memory.BuildVMMemoryMetadataForCreate(params.RAM, params.MemoryDynamic)
	if err != nil {
		_ = os.Remove(cloneDisk)
		return nil, err
	}

	// 克隆前检查宿主机可用内存
	if err := D.CheckHostMemory(ramMB); err != nil {
		_ = os.Remove(cloneDisk)
		return nil, err
	}

	// 检测模板是否使用 UEFI 启动，并保留普通 UEFI / 安全引导的区别。
	templateBootType := D.NormalizeTemplateBootType(meta.BootType)
	cloneBootType := ""
	needUEFI := false
	if params.UEFI != nil {
		needUEFI = *params.UEFI
		if needUEFI {
			cloneBootType = vm_xml.VMBootTypeUEFI
		} else {
			cloneBootType = vm_xml.VMBootTypeBIOS
		}
	} else if templateBootType == vm_xml.VMBootTypeUEFI || templateBootType == vm_xml.VMBootTypeUEFISecure {
		needUEFI = true
		cloneBootType = templateBootType
	} else {
		needUEFI = D.DetectTemplateBootType(templatePath) == "uefi"
		if needUEFI {
			cloneBootType = vm_xml.VMBootTypeUEFI
		}
	}

	if isWindows {
		// ===== Windows 克隆 =====
		if err := cloneWindows(ctx, params, cloneDisk, ramMB, memoryMeta, needUEFI, progressFn); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
	} else {
		// ===== Linux / FnOS / Other 克隆 =====
		if err := defineAndStartNonWindowsClone(params, cloneDisk, ramMB, memoryMeta, tplType, cloneBootType, needUEFI, meta.NVRAMPath); err != nil {
			_ = os.Remove(cloneDisk)
			return nil, err
		}
	}

	// 检查取消（VM 已创建，需要传 vmName 以便清理）
	if err := CheckCanceled(ctx, params.Name, cloneDisk); err != nil {
		return nil, err
	}

	progressFn(50, "虚拟机创建成功...")

	// 开机自启
	if params.Autostart {
		if err := libvirt_rpc.SetDomainAutostartRPC(params.Name, true); err != nil {
			logger.App.Warn("设置虚拟机自动启动失败", "vm", params.Name, "error", err)
		}
	}

	// 修复重启变关机
	D.FixOnReboot(params.Name)

	if len(params.ExtraDisks) > 0 {
		progressFn(52, "挂载额外磁盘...")
		if err := D.AddExtraDisksForVM(params.Name, params.ExtraDisks, cloneDir, params.DiskBus, params.IsAdmin, func(_ int, msg string) {
			progressFn(52, msg)
		}); err != nil {
			return nil, err
		}
	}

	// Other 类型不做任何初始化，直接完成
	if isOther {
		progressFn(100, "克隆完成（直接复制模式）")
		return &CloneResult{
			VMName:   params.Name,
			DiskPath: cloneDisk,
			Template: params.Template,
		}, nil
	}

	// 检查取消
	if err := CheckCanceled(ctx, params.Name, cloneDisk); err != nil {
		return nil, err
	}

	progressFn(60, "等待虚拟机启动...")

	// 等待获取 IP（可取消）
	time.Sleep(5 * time.Second)
	ip := WaitForIPWithContext(ctx, params.Name, LinuxCloneIPWaitSeconds)

	// Linux 初始化（仅 linux 类型）
	if tplType == "linux" {
		if ip == "" {
			return nil, fmt.Errorf("未获取到虚拟机 IP，Linux 初始化无法执行")
		}
		// 检查取消
		if err := CheckCanceled(ctx, params.Name, cloneDisk); err != nil {
			return nil, err
		}
		progressFn(70, "SSH 初始化中...")
		if err := InitLinuxClone(params, ip, progressFn); err != nil {
			logger.App.Warn("Linux SSH初始化失败", "error", err)
		}

		if !params.LinuxIdentityPrepared {
			progressFn(96, "等待虚拟机网络刷新...")
			oldIP := ip
			time.Sleep(15 * time.Second)
			newIP := ip_resolver.GetVMIP(params.Name, true)
			if newIP != "" && newIP != oldIP {
				ip = newIP
			}
		}
	}

	progressFn(100, "克隆完成")

	return &CloneResult{
		VMName:   params.Name,
		IP:       ip,
		DiskPath: cloneDisk,
		Template: params.Template,
	}, nil
}

// WaitForIPWithContext 等待虚拟机获取 IP（支持取消）
func WaitForIPWithContext(ctx context.Context, vmName string, maxWaitSeconds int) string {
	for waited := 0; waited < maxWaitSeconds; waited += 3 {
		select {
		case <-ctx.Done():
			return ""
		default:
		}
		ip := ip_resolver.GetVMIP(vmName, true)
		if ip != "" {
			return ip
		}
		// 可取消的 sleep
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(3 * time.Second):
		}
	}
	return ""
}
