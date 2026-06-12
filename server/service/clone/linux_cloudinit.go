package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kvm_console/logger"
	"kvm_console/utils"
)

// prepareLinuxNoCloudInit 通过 virt-customize 离线完成 Linux 克隆全部初始化
// 无需 SSH 连接：清理身份信息、写入 cloud-init NoCloud seed 文件、离线修改密码与用户名
// 适用于所有 Linux 模板，cloud-init seed 对无 cloud-init 的模板无害（被忽略）
func prepareLinuxNoCloudInit(params *CloneParams, cloneDisk string) error {
	// 生成 cloud-init seed 文件内容
	metaData := buildNoCloudMetaData(params)
	userData := buildNoCloudUserData(params)

	// 写入临时目录，通过 virt-customize --upload 注入磁盘
	tmpDir, err := os.MkdirTemp("", "nocloud-*")
	if err != nil {
		return fmt.Errorf("创建 cloud-init 临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	metaPath := filepath.Join(tmpDir, "meta-data")
	userPath := filepath.Join(tmpDir, "user-data")
	if err := os.WriteFile(metaPath, []byte(metaData), 0644); err != nil {
		return fmt.Errorf("写入 cloud-init meta-data 失败: %w", err)
	}
	if err := os.WriteFile(userPath, []byte(userData), 0644); err != nil {
		return fmt.Errorf("写入 cloud-init user-data 失败: %w", err)
	}

	templateUser := params.TemplateUser

	args := []string{
		"-a", cloneDisk,
		"--no-network",
		// 1. 清理 machine-id（重置实例身份）
		"--run-command", "truncate -s 0 /etc/machine-id 2>/dev/null || rm -f /etc/machine-id",
		"--run-command", "rm -f /var/lib/dbus/machine-id 2>/dev/null || true",
		// 2. 清理 DHCP 租约（防止 IP 冲突）
		"--run-command", "rm -f /var/lib/dhcp/*.leases 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/NetworkManager/*.lease 2>/dev/null || true",
		"--run-command", "rm -f /var/lib/systemd/network/*.lease 2>/dev/null || true",
		"--run-command", "rm -rf /run/systemd/netif/leases/* 2>/dev/null || true",
		// 3. 启用 cloud-init（删除 disabled 标记，允许首次启动时执行）
		"--run-command", "rm -f /etc/cloud/cloud-init.disabled",
		// 4. 清理安装器遗留的 cloud-init 配置文件
		//    99-installer.cfg: subiquity/curtin 安装后写入，强制 datasource=None + 重建 disabled 文件 + 禁用 growpart
		//    这些配置对克隆后的 VM 有害，必须删除以恢复 NoCloud 数据源和磁盘扩容功能
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/99-installer.cfg 2>/dev/null || true",
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/00-subiquity-disable-cloudinit-networking.cfg 2>/dev/null || true",
		"--run-command", "rm -f /etc/cloud/cloud.cfg.d/curtin-preserve-sources.cfg 2>/dev/null || true",
		// 5. 清理 cloud-init 实例缓存（强制重新初始化，而非跳过）
		"--run-command", "rm -rf /var/lib/cloud/instances/* /var/lib/cloud/instance",
		// 6. 写入 cloud-init NoCloud seed 文件（文件系统方式，无时序问题）
		"--run-command", "mkdir -p /var/lib/cloud/seed/nocloud",
		"--upload", metaPath + ":/var/lib/cloud/seed/nocloud/meta-data",
		"--upload", userPath + ":/var/lib/cloud/seed/nocloud/user-data",
		// 7. 离线写入 hostname（即使模板无 cloud-init 也保证生效）
		"--run-command", fmt.Sprintf("printf '%%s\\n' %s > /etc/hostname", utils.ShellSingleQuote(params.Hostname)),
		"--run-command", buildLinuxHostsCommand(params.Hostname),
		"--quiet",
	}

	// 7. 离线修改密码（通过 virt-customize --password，直接修改 /etc/shadow，无需 cloud-init）
	if params.Password != "" {
		args = append(args, "--password", "root:password:"+params.Password)
		if templateUser != "" {
			args = append(args, "--password", templateUser+":password:"+params.Password)
		}
	}

	// 8. 用户名重命名（离线，通过 usermod；如目标用户名与模板用户名不同）
	if params.User != "" && templateUser != "" && params.User != templateUser {
		renameCmd := fmt.Sprintf(
			`OLD=%s; NEW=%s; `+
				`if id "$OLD" >/dev/null 2>&1 && ! id "$NEW" >/dev/null 2>&1; then `+
				`usermod -l "$NEW" "$OLD" 2>/dev/null; `+
				`usermod -d /home/"$NEW" -m "$NEW" 2>/dev/null; `+
				`groupmod -n "$NEW" "$OLD" 2>/dev/null; `+
				`find /etc/sudoers.d/ -type f -exec sed -i "s/$OLD/$NEW/g" {} \; 2>/dev/null || true; `+
				`fi`,
			utils.ShellSingleQuote(templateUser),
			utils.ShellSingleQuote(params.User),
		)
		args = append(args, "--run-command", renameCmd)
		// 重命名后再对新用户名设置一次密码（确保生效）
		if params.Password != "" {
			args = append(args, "--password", params.User+":password:"+params.Password)
		}
	}

	result := utils.ExecCommandLongRunning("virt-customize", args...)
	if result.Error != nil {
		return fmt.Errorf("Linux 克隆离线初始化失败: %s", D.FirstNonEmpty(result.Stderr, result.Error.Error()))
	}
	logger.App.Info("Linux 离线初始化完成（NoCloud）", "vm", params.Name, "hostname", params.Hostname)
	return nil
}

// buildNoCloudMetaData 生成 cloud-init meta-data YAML 内容
// instance-id 每次克隆唯一，确保 cloud-init 识别为首次启动
func buildNoCloudMetaData(params *CloneParams) string {
	instanceID := fmt.Sprintf("iid-%s-%d", params.Name, time.Now().Unix())
	return fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceID, params.Hostname)
}

// buildNoCloudUserData 生成 cloud-init user-data cloud-config 内容
// 仅负责 hostname 确认 + 磁盘自动扩容；密码/用户名已在 virt-customize 阶段离线处理
func buildNoCloudUserData(params *CloneParams) string {
	var sb strings.Builder
	sb.WriteString("#cloud-config\n\n")
	sb.WriteString(fmt.Sprintf("hostname: %s\n", params.Hostname))
	sb.WriteString("manage_etc_hosts: true\n\n")
	sb.WriteString("ssh_pwauth: true\n\n")
	// growpart 对普通分区有效；对 LVM 系统由下方 runcmd 补充处理
	sb.WriteString("growpart:\n  mode: auto\n  devices: ['/']\nresize_rootfs: true\n\n")
	sb.WriteString("runcmd:\n")
	sb.WriteString(fmt.Sprintf("  - hostnamectl set-hostname %s 2>/dev/null || true\n", params.Hostname))
	// LVM 感知磁盘扩容脚本：自动检测根分区是否为 LVM，并执行 pvresize + lvextend
	// 使用 /sys/class/block 获取父磁盘和分区号，比 lsblk pkname/partn 更可靠
	sb.WriteString("  - |\n")
	sb.WriteString("    set +e\n")
	sb.WriteString("    ROOT_DEV=$(findmnt -n -o SOURCE /)\n")
	sb.WriteString("    if echo \"$ROOT_DEV\" | grep -q 'mapper'; then\n")
	sb.WriteString("      VG=$(lvs --noheadings -o vg_name \"$ROOT_DEV\" 2>/dev/null | awk '{print $1}' | head -1)\n")
	sb.WriteString("      PV=$(pvs --noheadings -o pv_name,vg_name 2>/dev/null | awk -v vg=\"$VG\" '$2==vg{print $1;exit}')\n")
	sb.WriteString("      if [ -n \"$PV\" ]; then\n")
	sb.WriteString("        PV_NAME=$(basename \"$PV\")\n")
	sb.WriteString("        SYS=$(readlink -f \"/sys/class/block/$PV_NAME\")\n")
	sb.WriteString("        DISK=$(basename \"$(dirname \"$SYS\")\")\n")
	sb.WriteString("        PARTNUM=$(cat \"/sys/class/block/$PV_NAME/partition\" 2>/dev/null)\n")
	sb.WriteString("        if [ -n \"$DISK\" ] && [ -n \"$PARTNUM\" ]; then\n")
	sb.WriteString("          growpart \"/dev/$DISK\" \"$PARTNUM\" 2>/dev/null || true\n")
	sb.WriteString("          pvresize \"$PV\" 2>/dev/null || true\n")
	sb.WriteString("          lvextend -r -l +100%FREE \"$ROOT_DEV\" 2>/dev/null || true\n")
	sb.WriteString("        fi\n")
	sb.WriteString("      fi\n")
	sb.WriteString("    fi\n")
	return sb.String()
}
