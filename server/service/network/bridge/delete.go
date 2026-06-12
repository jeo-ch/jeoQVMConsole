package bridge

import (
	"fmt"
	"os"

	"kvm_console/logger"
	"kvm_console/model"
	ovspkg "kvm_console/service/ovs"
	"kvm_console/utils"
)

func DeleteNetworkBridge(id uint) error {
	if id == 0 {
		return fmt.Errorf("默认网桥不能删除")
	}
	var row model.NetworkBridge
	if err := model.DB.First(&row, id).Error; err != nil {
		return fmt.Errorf("网桥不存在")
	}
	var count int64
	model.DB.Model(&model.VPCSwitch{}).Where("bridge_name = ?", row.Name).Count(&count)
	if count > 0 {
		return fmt.Errorf("该网桥仍有交换机使用，不能删除")
	}
	_ = os.Remove(bridgeRestoreScriptPath(row.Name))
	// 仅当 OVS 桥实际存在时执行物理清理；桥已不存在则跳过直接清理记录
	if ovsBridgeExists(row.Name) {
		if row.MigrateHostIP {
			migrateBridgeIPv4ToInterface(row.Name, row.UplinkIF)
		}
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-port", row.Name, row.UplinkIF)
		utils.ExecCommand("ovs-vsctl", "--if-exists", "del-br", row.Name)
	}
	disableBridgeRestoreUnitIfEmpty()
	// 先恢复 OVS 默认网络（此时 IP/路由已迁回物理口，可正常检测 uplink）
	if HookEnsureOVSNetworkReady != nil {
		if err := HookEnsureOVSNetworkReady(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复默认 OVS 网络失败: %w", err)
		}
	}
	if HookEnsureAllVPCSwitchRuntime != nil {
		if err := HookEnsureAllVPCSwitchRuntime(); err != nil {
			return fmt.Errorf("网桥已删除，但恢复 VPC 交换机网络失败: %w", err)
		}
	}
	// 最后恢复 networkd DHCP（networkctl reload 可能短暂影响路由，必须在 EnsureOVSNetworkReady 之后）
	removeNetworkdDHCPOverrideForPort(row.UplinkIF)
	if err := model.DB.Delete(&row).Error; err != nil {
		return err
	}
	return nil
}

func EnsureAllNetworkBridgesRuntime() error {
	if model.DB == nil {
		return nil
	}
	var rows []model.NetworkBridge
	if err := model.DB.Where("mode = ?", BridgeModeDirect).Find(&rows).Error; err != nil {
		return err
	}
	var lastErr error
	for _, row := range rows {
		cfg := HostIPConfig{
			Addrs:   row.HostAddrs,
			Gateway: row.HostGateway,
			Metric:  row.HostMetric,
		}
		if err := EnsureOVSBridgeDirect(row.Name, row.UplinkIF, row.MigrateHostIP, cfg); err != nil {
			lastErr = err
			logger.App.Warn("恢复桥接网桥失败", "bridge", row.Name, "error", err)
		}
	}
	return lastErr
}

func defaultNetworkBridgeRecords() []NetworkBridgeInfo {
	return []NetworkBridgeInfo{{
		Name:      ovspkg.OvsBridgeName(),
		Mode:      BridgeModeNAT,
		IsDefault: true,
	}}
}
