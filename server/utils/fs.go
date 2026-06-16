package utils

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// AtomicWriteFile 原子写文件：先写入临时文件，再通过 os.Rename 替换目标文件
// 保证并发安全，避免写入一半时其他进程读到不完整内容
func AtomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, perm); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("重命名临时文件失败: %w", err)
	}

	return nil
}

// GetUserIDs 查询系统用户的 UID 和组的 GID
// 如果 groupname 为空，则使用用户的主组 GID
func GetUserIDs(username, groupname string) (uid, gid int, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, 0, fmt.Errorf("查找用户 %s 失败: %w", username, err)
	}

	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 UID 失败: %w", err)
	}

	if groupname != "" {
		g, err := user.LookupGroup(groupname)
		if err != nil {
			return 0, 0, fmt.Errorf("查找组 %s 失败: %w", groupname, err)
		}
		gid, err = strconv.Atoi(g.Gid)
		if err != nil {
			return 0, 0, fmt.Errorf("解析 GID 失败: %w", err)
		}
	} else {
		gid, err = strconv.Atoi(u.Gid)
		if err != nil {
			return 0, 0, fmt.Errorf("解析主组 GID 失败: %w", err)
		}
	}

	return uid, gid, nil
}

// FileExists 使用 Go 原生 os.Stat 检查文件是否存在（非目录）
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// FileReadable 检查文件是否存在且可读
func FileReadable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// 尝试打开文件验证可读性
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// ChownLibvirtQEMU 尝试将文件 chown 为 libvirt-qemu:kvm，失败则回退到 qemu:qemu
// 返回错误仅当两次尝试都失败时
func ChownLibvirtQEMU(path string) error {
	if uid, gid, err := GetUserIDs("libvirt-qemu", "kvm"); err == nil {
		if err := os.Chown(path, uid, gid); err == nil {
			return nil
		}
	}
	// 回退到 qemu:qemu
	uid, gid, err := GetUserIDs("qemu", "qemu")
	if err != nil {
		return fmt.Errorf("chown %s 失败: 无法查找 libvirt-qemu/kvm 或 qemu/qemu: %w", path, err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %s 失败: %w", path, err)
	}
	return nil
}

// SetFileImmutable 对文件设置 Linux 不可变属性 (chattr +i)
// 设置后文件不可被修改、删除、重命名，即使 root 也无法直接操作
func SetFileImmutable(path string) error {
	result := ExecCommand("chattr", "+i", path)
	if result.Error != nil {
		return fmt.Errorf("设置文件不可变属性失败 %s: %s", path, result.Stderr)
	}
	return nil
}

// RemoveFileImmutable 移除文件的 Linux 不可变属性 (chattr -i)
func RemoveFileImmutable(path string) error {
	result := ExecCommand("chattr", "-i", path)
	if result.Error != nil {
		// 如果文件已经不可变或 chattr 不可用，不视为错误
		return nil
	}
	return nil
}

// IsFileImmutable 检查文件是否设置了不可变属性
func IsFileImmutable(path string) bool {
	result := ExecCommand("lsattr", path)
	if result.Error != nil {
		return false
	}
	// lsattr 输出格式: "----i---------e------- /path/to/file"
	// i 表示 immutable
	return strings.Contains(result.Stdout, "-i-") || strings.Contains(result.Stdout, "----i")
}

// ==================== 大文件上传落盘模式 ====================

// largeUploadDiskMode 标记当前是否启用了大文件上传落盘模式
// 当 /tmp 为 tmpfs 且空间有限时，Go multipart 解析的临时目录会被重定向到磁盘
var largeUploadDiskMode bool

// SetLargeUploadDiskMode 设置大文件上传落盘模式标记
func SetLargeUploadDiskMode(v bool) {
	largeUploadDiskMode = v
}

// IsLargeUploadDiskMode 返回当前是否处于大文件上传落盘模式
func IsLargeUploadDiskMode() bool {
	return largeUploadDiskMode
}
