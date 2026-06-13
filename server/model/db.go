package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"kvm_console/config"
	"kvm_console/logger"
)

// gormAppLogger 将 GORM 日志写入 appWriter，不直接输出到 stdout
// 可被 KVM_LOG_CONSOLE_TYPES=app 控制是否显示在终端
type gormAppLogger struct {
	slowThreshold time.Duration
}

func (l *gormAppLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l // 始终 Warn 级别，忽略外部设置
}

func (l *gormAppLogger) Info(_ context.Context, msg string, args ...interface{}) {
	// Info 级别不输出（GORM info 太嘈杂）
}

func (l *gormAppLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	logger.App.Warn(msg, "source", "gorm")
}

func (l *gormAppLogger) Error(_ context.Context, msg string, args ...interface{}) {
	logger.App.Error(msg, "source", "gorm")
}

func (l *gormAppLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	if err != nil && err.Error() != "record not found" {
		logger.App.Error("数据库查询错误", "elapsed", elapsed, "rows", rows, "sql", sql, "error", err)
		return
	}
	if l.slowThreshold > 0 && elapsed > l.slowThreshold {
		logger.App.Warn("慢查询", "elapsed", elapsed, "rows", rows, "sql", sql)
	}
}

// DB 全局数据库实例
var DB *gorm.DB

// InitDB 初始化数据库
func InitDB() {
	// 确保数据目录存在
	dbDir := filepath.Dir(config.GlobalConfig.DBPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		logger.App.Error("创建数据库目录失败", "error", err)
		os.Exit(1)
	}

	var err error
	DB, err = gorm.Open(sqlite.Open(config.GlobalConfig.DBPath), &gorm.Config{
		Logger: &gormAppLogger{},
	})
	if err != nil {
		logger.App.Error("连接数据库失败", "error", err)
		os.Exit(1)
	}

	hadMaxPortForwardsColumn := DB.Migrator().HasColumn(&User{}, "max_port_forwards")
	hadEnablePortForwardColumn := DB.Migrator().HasColumn(&User{}, "enable_port_forward")
	hadUserMaxSnapshotsColumn := DB.Migrator().HasColumn(&User{}, "max_snapshots")
	hadLightweightQuotaMaxSnapshotsColumn := DB.Migrator().HasColumn(&LightweightVMQuota{}, "max_snapshots")
	hadLightweightRegistrationMaxSnapshotsColumn := DB.Migrator().HasColumn(&LightweightVMRegistration{}, "max_snapshots")
	hadLightweightQuotaMaxRuntimeColumn := DB.Migrator().HasColumn(&LightweightVMQuota{}, "max_runtime_hours")
	hadLightweightRegistrationMaxRuntimeColumn := DB.Migrator().HasColumn(&LightweightVMRegistration{}, "max_runtime_hours")
	hadVPCBindingInterfaceOrderColumn := DB.Migrator().HasColumn(&VPCVMBinding{}, "interface_order")
	hadVPCSwitchCIDRColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "cidr")

	// 自动迁移表结构
	if err := DB.AutoMigrate(&User{}, &UserAPIKey{}, &VmStatsRecord{}, &PortForwardIP{}, &PortForwardWhitelist{}, &PortForwardProbeState{}, &HostStatsRecord{}, &UserTrafficDaily{}, &SystemSetting{}, &VMCredential{}, &VMCache{}, &AuthActionToken{}, &SecurityChallenge{}, &SchedulerEvent{}, &VMSchedule{}, &NetworkBridge{}, &HostStoragePool{}, &HostNode{},
		&LightweightVMQuota{}, &LightweightVMTrafficMonthly{}, &LightweightVMRegistration{},
		&VPCSwitch{}, &VPCSecurityGroup{}, &VPCSecurityGroupRule{}, &VPCVMBinding{}, &VPCSwitchTrafficMonthly{}, &PublicIP{}, &PublicIPBinding{},
		&VMLock{}); err != nil {
		logger.App.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
	migrateUserCloudType()
	migratePublicIPCIDRColumn()
	migrateUserPortForwardFeature(hadEnablePortForwardColumn)
	migrateUserPortForwardQuota(hadMaxPortForwardsColumn)
	migrateUserSnapshotQuota(hadUserMaxSnapshotsColumn)
	migrateLightweightSnapshotQuota(hadLightweightQuotaMaxSnapshotsColumn, hadLightweightRegistrationMaxSnapshotsColumn)
	migrateLightweightRuntimeQuota(hadLightweightQuotaMaxRuntimeColumn, hadLightweightRegistrationMaxRuntimeColumn)
	migrateVPCBindingInterfaceOrder(hadVPCBindingInterfaceOrderColumn)
	migrateVPCSwitchCIDRColumn(hadVPCSwitchCIDRColumn)

	// 兼容旧用户：补齐默认状态，确保升级后能继续登录
	if err := DB.Model(&User{}).Where("status = '' OR status IS NULL").Updates(map[string]interface{}{
		"status": "active",
	}).Error; err != nil {
		logger.App.Warn("修复旧用户状态失败", "error", err)
	}

	// 初始化默认管理员
	initDefaultAdmin()
	logger.App.Info("数据库初始化完成")
}

func migrateUserCloudType() {
	if DB == nil {
		return
	}
	if err := DB.Model(&User{}).
		Where("cloud_type = '' OR cloud_type IS NULL").
		Update("cloud_type", "elastic").Error; err != nil {
		logger.App.Warn("初始化用户云类型失败", "error", err)
	}
}

func migrateUserPortForwardQuota(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ? AND (max_port_forwards IS NULL OR max_port_forwards = 0)", "admin").
		Update("max_port_forwards", 10).Error; err != nil {
		logger.App.Warn("初始化用户端口转发配额失败", "error", err)
	}
}

func migrateUserPortForwardFeature(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ?", "admin").
		Update("enable_port_forward", true).Error; err != nil {
		logger.App.Warn("初始化用户端口转发开关失败", "error", err)
	}
}

func migrateUserSnapshotQuota(hadColumn bool) {
	if DB == nil || hadColumn {
		return
	}
	if err := DB.Model(&User{}).
		Where("role <> ? AND (max_snapshots IS NULL OR max_snapshots = 0)", "admin").
		Update("max_snapshots", 5).Error; err != nil {
		logger.App.Warn("初始化用户快照配额失败", "error", err)
	}
}

func migrateLightweightSnapshotQuota(hadQuotaColumn, hadRegistrationColumn bool) {
	if DB == nil {
		return
	}
	if !hadQuotaColumn {
		if err := DB.Model(&LightweightVMQuota{}).
			Where("max_snapshots IS NULL OR max_snapshots = 0").
			Update("max_snapshots", 2).Error; err != nil {
			logger.App.Warn("初始化轻量云VM快照配额失败", "error", err)
		}
	}
	if !hadRegistrationColumn {
		if err := DB.Model(&LightweightVMRegistration{}).
			Where("max_snapshots IS NULL OR max_snapshots = 0").
			Update("max_snapshots", 2).Error; err != nil {
			logger.App.Warn("初始化轻量云VM注册快照配额失败", "error", err)
		}
	}
}

func migrateLightweightRuntimeQuota(hadQuotaColumn, hadRegistrationColumn bool) {
	if DB == nil {
		return
	}
	if !hadQuotaColumn {
		if err := DB.Model(&LightweightVMQuota{}).
			Where("max_runtime_hours IS NULL").
			Update("max_runtime_hours", 0).Error; err != nil {
			logger.App.Warn("初始化轻量云VM运行时长配额失败", "error", err)
		}
	}
	if !hadRegistrationColumn {
		if err := DB.Model(&LightweightVMRegistration{}).
			Where("max_runtime_hours IS NULL").
			Update("max_runtime_hours", 0).Error; err != nil {
			logger.App.Warn("初始化轻量云VM注册运行时长配额失败", "error", err)
		}
	}
}

func migratePublicIPCIDRColumn() {
	if DB == nil || !DB.Migrator().HasTable(&PublicIP{}) {
		return
	}
	if !DB.Migrator().HasColumn(&PublicIP{}, "c_id_r") || !DB.Migrator().HasColumn(&PublicIP{}, "cidr") {
		return
	}
	if err := DB.Exec("UPDATE public_ips SET cidr = c_id_r WHERE (cidr IS NULL OR cidr = '') AND c_id_r IS NOT NULL AND c_id_r <> ''").Error; err != nil {
		logger.App.Warn("迁移公网IP CIDR字段失败", "error", err)
	}
}

func migrateVPCBindingInterfaceOrder(hadColumn bool) {
	if DB == nil {
		return
	}
	// 修复联合唯一索引：从 vm_name 单列索引迁移到 (vm_name, interface_order) 联合索引
	// GORM AutoMigrate 可能无法正确重建索引，需要手动处理
	if !hadColumn {
		// 首次迁移：填充默认值
		if err := DB.Model(&VPCVMBinding{}).
			Where("interface_order IS NULL OR interface_order = 0").
			Update("interface_order", 0).Error; err != nil {
			logger.App.Warn("初始化VPC绑定interface_order失败", "error", err)
		}
		if err := DB.Model(&VPCVMBinding{}).
			Where("nic_model IS NULL OR nic_model = ''").
			Update("nic_model", "virtio").Error; err != nil {
			logger.App.Warn("初始化VPC绑定nic_model失败", "error", err)
		}
	}

	// 始终确保索引正确：删除可能的旧单列唯一索引，创建新联合唯一索引
	migrateVPCBindingUniqueIndex()
}

func migrateVPCBindingUniqueIndex() {
	if DB == nil {
		return
	}
	// GORM 可能生成多种索引名称，逐一尝试删除旧索引
	oldIndexNames := []string{
		"uni_vpc_vm_bindings_vm_name",
		"idx_vpc_vm_bindings_vm_name",
		"uq_vpc_vm_bindings_vm_name",
	}
	for _, name := range oldIndexNames {
		DB.Exec("DROP INDEX IF EXISTS " + name)
	}
	// 创建新的联合唯一索引
	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vm_interface ON vpc_vm_bindings(vm_name, interface_order)").Error; err != nil {
		logger.App.Warn("创建VPC绑定联合唯一索引失败", "error", err)
	}
}

// migrateVPCSwitchCIDRColumn 为旧版 vpc_switches 表补齐 cidr 列
// GORM 默认将 CIDR 映射为 c_id_r（连续大写字母被拆分为独立单词），
// 旧版数据库中存在 c_id_r 列存储实际 CIDR 值，需迁移至显式指定的 cidr 列。
func migrateVPCSwitchCIDRColumn(hadColumn bool) {
	if DB == nil {
		return
	}

	// Stage 0: 检查并修复 GORM 错误命名的 c_id_r 列 → cidr 列
	hasOldColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "c_id_r")
	hasNewColumn := DB.Migrator().HasColumn(&VPCSwitch{}, "cidr")

	if hasOldColumn && hasNewColumn {
		// 将 c_id_r 中的数据迁移到 cidr（仅更新 cidr 为空的记录）
		if err := DB.Exec("UPDATE vpc_switches SET cidr = c_id_r WHERE (cidr IS NULL OR cidr = '') AND c_id_r IS NOT NULL AND c_id_r <> ''").Error; err != nil {
			logger.App.Warn("迁移 c_id_r → cidr 数据失败", "error", err)
		} else {
			logger.App.Info("已从 c_id_r 迁移数据到 cidr 列")
		}
		// 创建唯一索引（可能因之前迁移失败而缺失）
		if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vpc_switches_cidr ON vpc_switches(cidr)").Error; err != nil {
			logger.App.Warn("创建 vpc_switches.cidr 唯一索引失败", "error", err)
		}
		// 删除旧的无效索引（c_id_r 列上的索引，如果有的话）
		DB.Exec("DROP INDEX IF EXISTS idx_vpc_switches_c_id_r")
		return
	}

	if hadColumn {
		return
	}

	logger.App.Info("开始迁移 vpc_switches.cidr 列")

	// 1. 添加 cidr 列（暂不设 NOT NULL，避免与已有数据冲突）
	if !hasNewColumn {
		if err := DB.Exec("ALTER TABLE vpc_switches ADD COLUMN cidr TEXT DEFAULT ''").Error; err != nil {
			logger.App.Warn("添加 vpc_switches.cidr 列失败", "error", err)
			return
		}
	}

	// 2. 收集已占用的 CIDR，避免冲突
	prefix := strings.Trim(config.GlobalConfig.VPCSubnetPrefix, ". ")
	if prefix == "" {
		prefix = "10.200"
	}

	var switches []VPCSwitch
	if err := DB.Order("id ASC").Find(&switches).Error; err != nil {
		logger.App.Warn("迁移时查询交换机列表失败", "error", err)
		return
	}

	used := map[string]bool{}
	for _, sw := range switches {
		if sw.CIDR != "" {
			used[sw.CIDR] = true
		}
	}

	// 3. 为每个未设置 CIDR 的交换机分配一个唯一 CIDR
	idx := 1
	for _, sw := range switches {
		if sw.CIDR != "" {
			continue
		}
		var cidr string
		for {
			cidr = fmt.Sprintf("%s.%d.0/24", prefix, idx)
			idx++
			if !used[cidr] {
				break
			}
		}
		used[cidr] = true
		if err := DB.Model(&VPCSwitch{}).Where("id = ?", sw.ID).Update("cidr", cidr).Error; err != nil {
			logger.App.Warn("更新交换机 cidr 失败", "sw_id", sw.ID, "error", err)
		}
	}

	// 4. 创建唯一索引
	if err := DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vpc_switches_cidr ON vpc_switches(cidr)").Error; err != nil {
		logger.App.Warn("创建 vpc_switches.cidr 唯一索引失败", "error", err)
	}

	logger.App.Info("vpc_switches.cidr 列迁移完成")
}

// initDefaultAdmin 创建默认管理员账号
func initDefaultAdmin() {
	var count int64
	DB.Model(&User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(config.GlobalConfig.DefaultAdminPass), bcrypt.DefaultCost,
	)
	if err != nil {
		logger.App.Error("生成密码哈希失败", "error", err)
		os.Exit(1)
	}

	admin := User{
		Username:     config.GlobalConfig.DefaultAdminUser,
		PasswordHash: string(hashedPassword),
		Role:         "admin",
		Status:       "active",
	}

	if err := DB.Create(&admin).Error; err != nil {
		logger.App.Warn("创建默认管理员失败", "error", err)
	} else {
		logger.App.Info("默认管理员账号已创建", "username", admin.Username)
	}
}
