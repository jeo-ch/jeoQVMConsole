package service

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"kvm_console/model"
	"kvm_console/utils"
)

var errVMCacheSourceMissing = errors.New("vm cache source missing")

var vmCacheRefreshCooldown = 8 * time.Second

var vmCacheNow = time.Now

var vmCacheListNamesFromHost = defaultVMCacheListNamesFromHost

var vmCacheBuildRecordFromHost = defaultVMCacheBuildRecordFromHost

var vmCacheRunFullSync = func() error {
	return SyncVMCacheFromHost()
}

var vmCacheRefreshState = struct {
	sync.Mutex
	inProgress    bool
	lastTriggered time.Time
}{}

// BootstrapVMCacheFromHost 在程序启动时同步当前宿主机 VM 缓存。
func BootstrapVMCacheFromHost() error {
	return SyncVMCacheFromHost()
}

// TriggerAdminVMCacheRefreshIfNeeded 在管理员访问列表接口时异步触发一次缓存刷新。
func TriggerAdminVMCacheRefreshIfNeeded() {
	if model.DB == nil {
		return
	}

	now := vmCacheNow()
	vmCacheRefreshState.Lock()
	if vmCacheRefreshState.inProgress {
		vmCacheRefreshState.Unlock()
		return
	}
	if !vmCacheRefreshState.lastTriggered.IsZero() && now.Sub(vmCacheRefreshState.lastTriggered) < vmCacheRefreshCooldown {
		vmCacheRefreshState.Unlock()
		return
	}
	vmCacheRefreshState.inProgress = true
	vmCacheRefreshState.lastTriggered = now
	vmCacheRefreshState.Unlock()

	go func() {
		defer func() {
			vmCacheRefreshState.Lock()
			vmCacheRefreshState.inProgress = false
			vmCacheRefreshState.Unlock()
		}()
		if err := vmCacheRunFullSync(); err != nil {
			log.Printf("[警告] 管理员触发虚拟机缓存刷新失败: %v", err)
		}
	}()
}

// ListCachedVMs 从数据库缓存读取当前 VM 列表。
func ListCachedVMs(options ...VMListOptions) ([]VmInfo, error) {
	return listCachedVMs("", false, options...)
}

// ListCachedVMsByOwner 从数据库缓存读取指定用户的 VM 列表。
func ListCachedVMsByOwner(username string, options ...VMListOptions) ([]VmInfo, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return []VmInfo{}, nil
	}
	return listCachedVMs(username, true, options...)
}

func listCachedVMs(username string, limitOwner bool, options ...VMListOptions) ([]VmInfo, error) {
	listOptions := VMListOptions{}
	if len(options) > 0 {
		listOptions = options[0]
	}
	if model.DB == nil {
		return []VmInfo{}, nil
	}

	query := model.DB.Where("present = ?", true).Order("name ASC")
	if limitOwner {
		query = query.Where("owner_username = ?", username)
	}

	var records []model.VMCache
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("读取虚拟机缓存失败: %w", err)
	}

	vms := make([]VmInfo, 0, len(records))
	for _, record := range records {
		vms = append(vms, vmInfoFromCacheRecord(record, listOptions))
	}
	return vms, nil
}

func vmInfoFromCacheRecord(record model.VMCache, options VMListOptions) VmInfo {
	vm := VmInfo{
		Name:      record.Name,
		Remark:    record.Remark,
		Group:     record.GroupName,
		Status:    record.Status,
		VCPU:      record.VCPU,
		Memory:    record.MemoryMB,
		MaxMemory: record.MaxMemoryMB,
		DiskSize:  record.DiskSizeText,
		Template:  record.Template,
		Autostart: record.Autostart,
		CreatedAt: record.CreatedAtText,
		InRescue:  record.InRescue,
	}

	if options.IncludeIP {
		vm.IP = record.CachedIP
	}
	if options.IncludeNetworkInfo {
		vm.NicModel = record.NicModel
		vm.MacAddress = record.MacAddress
	}
	if options.IncludeBandwidth {
		vm.BandwidthIn = record.BandwidthIn
		vm.BandwidthOut = record.BandwidthOut
	}
	if options.IncludeResourceUsage && strings.EqualFold(strings.TrimSpace(record.Status), "running") {
		if cached := GetCachedStats(record.Name); cached != nil {
			vm.CPUPercent = cached.CPUPercent
			if cached.MemTotal > 0 {
				vm.MemPercent = float64(cached.MemUsed) / float64(cached.MemTotal) * 100
			}
		}
	}

	runtimeInfo := GetVMRuntimeInfo(record.Name, record.Status)
	vm.ContinuousRuntimeSeconds = runtimeInfo.ContinuousRuntimeSeconds
	vm.ContinuousRunningSince = runtimeInfo.ContinuousRunningSince
	vm.Locked = IsVMLocked(record.Name)
	applyVMUnderMigrationStatus(&vm)
	return vm
}

// SyncVMCacheFromHost 从宿主机全量同步当前 VM 到数据库缓存。
func SyncVMCacheFromHost() error {
	if model.DB == nil {
		return fmt.Errorf("数据库未初始化")
	}

	names, err := vmCacheListNamesFromHost()
	if err != nil {
		return err
	}

	syncedAt := vmCacheNow()
	seen := make(map[string]bool, len(names))
	records := make([]model.VMCache, 0, len(names))

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		seen[name] = true
		record, buildErr := vmCacheBuildRecordFromHost(name, syncedAt)
		if buildErr != nil {
			log.Printf("[警告] 构建虚拟机 %s 缓存失败，已保留旧缓存: %v", name, buildErr)
			continue
		}
		records = append(records, record)
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "name"}},
				UpdateAll: true,
			}).Create(&record).Error; err != nil {
				return fmt.Errorf("写入虚拟机缓存失败: %w", err)
			}
		}

		if len(seen) == 0 {
			if err := tx.Model(&model.VMCache{}).
				Where("present = ?", true).
				Updates(map[string]interface{}{
					"present":        false,
					"last_synced_at": syncedAt,
				}).Error; err != nil {
				return fmt.Errorf("更新虚拟机缓存状态失败: %w", err)
			}
			return nil
		}

		namesToKeep := make([]string, 0, len(seen))
		for name := range seen {
			namesToKeep = append(namesToKeep, name)
		}
		if err := tx.Model(&model.VMCache{}).
			Where("name NOT IN ?", namesToKeep).
			Where("present = ?", true).
			Updates(map[string]interface{}{
				"present":        false,
				"last_synced_at": syncedAt,
			}).Error; err != nil {
			return fmt.Errorf("标记失效虚拟机缓存失败: %w", err)
		}
		return nil
	})
}

// RefreshVMCacheByName 刷新单台虚拟机缓存，不存在时自动标记失效。
func RefreshVMCacheByName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || model.DB == nil {
		return nil
	}

	record, err := vmCacheBuildRecordFromHost(name, vmCacheNow())
	if err != nil {
		if errors.Is(err, errVMCacheSourceMissing) {
			return MarkVMCacheMissing(name)
		}
		return err
	}
	return upsertVMCacheRecord(record)
}

// MarkVMCacheMissing 将指定虚拟机缓存标记为已失效。
func MarkVMCacheMissing(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || model.DB == nil {
		return nil
	}

	updates := map[string]interface{}{
		"present":        false,
		"last_synced_at": vmCacheNow(),
	}
	return model.DB.Model(&model.VMCache{}).Where("name = ?", name).Updates(updates).Error
}

// RefreshVMCacheByNameAsync 异步刷新单台虚拟机缓存。
func RefreshVMCacheByNameAsync(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	go func() {
		if err := RefreshVMCacheByName(name); err != nil {
			log.Printf("[警告] 刷新虚拟机 %s 缓存失败: %v", name, err)
		}
	}()
}

// MarkVMCacheMissingAsync 异步标记虚拟机缓存失效。
func MarkVMCacheMissingAsync(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	go func() {
		if err := MarkVMCacheMissing(name); err != nil {
			log.Printf("[警告] 标记虚拟机 %s 缓存失效失败: %v", name, err)
		}
	}()
}

func upsertVMCacheRecord(record model.VMCache) error {
	if model.DB == nil {
		return nil
	}
	return model.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		UpdateAll: true,
	}).Create(&record).Error
}

func defaultVMCacheListNamesFromHost() ([]string, error) {
	// 优先使用 go-libvirt RPC
	if IsLibvirtRPCAvailable() {
		domains, err := listAllDomainsRPC()
		if err == nil {
			names := make([]string, 0, len(domains))
			for _, dom := range domains {
				if dom.Name != "" {
					names = append(names, dom.Name)
				}
			}
			return names, nil
		}
		log.Printf("[go-libvirt] listAllDomainsRPC 失败，降级为 virsh: %v", err)
	}

	// fallback: 原有 virsh 逻辑
	result := utils.ExecCommand("virsh", "list", "--all", "--name")
	if result.Error != nil {
		if IsMaintenanceModeEnabled() && (isLibvirtUnavailableText(result.Stderr) || IsLibvirtUnavailableError(result.Error)) {
			return nil, result.Error
		}
		return nil, result.Error
	}

	lines := strings.Split(result.Stdout, "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func defaultVMCacheBuildRecordFromHost(name string, syncedAt time.Time) (model.VMCache, error) {
	record := model.VMCache{
		Name:          name,
		OwnerUsername: firstNonEmpty(strings.TrimSpace(FindVMOwner(name)), "admin"),
		Present:       true,
		LastSyncedAt:  syncedAt,
	}

	var vcpu int
	var maxMemKB, usedMemKB uint64
	var autostart bool
	var status string
	var dominfoViaRPC bool

	// 优先使用 go-libvirt RPC 替代 virsh dominfo / domstate
	if IsLibvirtRPCAvailable() {
		vcpuRPC, maxMemKB_RPC, usedMemKB_RPC, autostartRPC, infoErr := getDomainInfoRPC(name)
		if infoErr != nil {
			if isDomainNotFoundError(infoErr) {
				return model.VMCache{}, errVMCacheSourceMissing
			}
			log.Printf("[go-libvirt] getDomainInfoRPC(%s) 失败，降级为 virsh: %v", name, infoErr)
		} else {
			vcpu = vcpuRPC
			maxMemKB = maxMemKB_RPC
			usedMemKB = usedMemKB_RPC
			autostart = autostartRPC
			dominfoViaRPC = true
		}

		statusRPC, stateErr := getDomainStateRPC(name)
		if stateErr != nil {
			log.Printf("[go-libvirt] getDomainStateRPC(%s) 失败，降级为 virsh: %v", name, stateErr)
		} else {
			status = statusRPC
		}
	}

	// fallback: virsh dominfo
	if !dominfoViaRPC {
		domInfoResult := utils.ExecCommand("virsh", "dominfo", name)
		if domInfoResult.Error != nil {
			if isVMCacheSourceMissingResult(domInfoResult) {
				return model.VMCache{}, errVMCacheSourceMissing
			}
			return model.VMCache{}, domInfoResult.Error
		}
		vcpu = parseInfoInt(domInfoResult.Stdout, "CPU(s):")
		maxMemKB = uint64(parseInfoInt(domInfoResult.Stdout, "Max memory:"))
		usedMemKB = uint64(parseInfoInt(domInfoResult.Stdout, "Used memory:"))
		autostart = strings.Contains(domInfoResult.Stdout, "Autostart:      enable")
	}

	// fallback: virsh domstate
	if status == "" {
		stateResult := utils.ExecCommand("virsh", "domstate", name)
		if stateResult.Error == nil {
			status = strings.TrimSpace(stateResult.Stdout)
		}
	}
	UpdateVMRuntimeState(name, status, syncedAt)

	record.Status = status
	record.VCPU = vcpu
	record.MaxMemoryMB = int(maxMemKB / 1024)
	record.MemoryMB = int(usedMemKB / 1024)
	record.Autostart = autostart

	record.CreatedAtText = readVMCreatedAtText(name)
	if remark, err := GetVMRemark(name); err == nil {
		record.Remark = remark
	}

	if group, err := GetVMGroup(name); err == nil {
		record.GroupName = group
	}

	diskInfo := getVMDiskInfo(name)
	record.DiskSizeText = diskInfo.size
	record.Template = diskInfo.template

	netInfo := getVMNetworkInfo(name)
	record.NicModel = netInfo.nicModel
	record.MacAddress = netInfo.mac

	record.BandwidthIn, record.BandwidthOut = GetVMBandwidthMbps(name)
	record.InRescue = IsInRescueMode(name)
	record.CachedIP = getVMIP(name, strings.EqualFold(record.Status, "running"))

	return record, nil
}

// isDomainNotFoundError 判断 RPC 错误是否为域不存在
func isDomainNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "domain not found") ||
		strings.Contains(msg, "no domain with matching name") ||
		strings.Contains(msg, "failed to get domain") ||
		strings.Contains(msg, "不存在")
}

func readVMCreatedAtText(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	xmlPath := fmt.Sprintf("/etc/libvirt/qemu/%s.xml", name)
	result := utils.ExecShell(fmt.Sprintf("stat -c '%%W|%%Y' %s 2>/dev/null", utils.ShellSingleQuote(xmlPath)))
	if result.Error != nil {
		return ""
	}

	parts := strings.Split(strings.TrimSpace(result.Stdout), "|")
	if len(parts) < 2 {
		return ""
	}

	createdSeconds := int64(0)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil || value <= 0 {
			continue
		}
		createdSeconds = value
		break
	}
	if createdSeconds <= 0 {
		return ""
	}
	return time.Unix(createdSeconds, 0).Format("2006-01-02 15:04:05")
}

func isVMCacheSourceMissingResult(result *utils.CmdResult) bool {
	if result == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	return strings.Contains(message, "domain not found") ||
		strings.Contains(message, "failed to get domain") ||
		strings.Contains(message, "no domain with matching name")
}

func UpdateVMCacheOwner(name, owner string) {
	name = strings.TrimSpace(name)
	if name == "" || model.DB == nil {
		return
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "admin"
	}
	if err := model.DB.Model(&model.VMCache{}).Where("name = ?", name).Update("owner_username", owner).Error; err != nil {
		log.Printf("[警告] 更新虚拟机 %s 缓存归属失败: %v", name, err)
	}
}

func SyncVMCacheOwner(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	UpdateVMCacheOwner(name, firstNonEmpty(strings.TrimSpace(FindVMOwner(name)), "admin"))
}

func SyncVMCacheOwnersForAssignment(username string, assignedVMs []string) {
	username = strings.TrimSpace(username)
	if username == "" || model.DB == nil {
		return
	}

	assignedSet := make(map[string]bool, len(assignedVMs))
	for _, vmName := range assignedVMs {
		vmName = strings.TrimSpace(vmName)
		if vmName == "" {
			continue
		}
		assignedSet[vmName] = true
		UpdateVMCacheOwner(vmName, username)
	}

	var records []model.VMCache
	if err := model.DB.Where("owner_username = ?", username).Find(&records).Error; err != nil {
		log.Printf("[警告] 查询用户 %s 的虚拟机缓存归属失败: %v", username, err)
		return
	}
	for _, record := range records {
		if assignedSet[record.Name] {
			continue
		}
		SyncVMCacheOwner(record.Name)
	}
}
