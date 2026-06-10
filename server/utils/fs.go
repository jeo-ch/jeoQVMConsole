package utils

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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
