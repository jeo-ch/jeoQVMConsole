package service

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"

	"kvm_console/config"
)

var (
	libvirtConn   *libvirt.Libvirt
	libvirtConnMu sync.RWMutex
	libvirtSocket = "/var/run/libvirt/libvirt-sock"
)

// InitLibvirtRPC 初始化 go-libvirt RPC 连接（程序启动时调用）
// 连接失败不影响程序启动，后续会降级为 virsh
func InitLibvirtRPC() {
	log.Printf("[libvirt-rpc] 开始初始化 (UseGoLibvirt=%v, socket=%s)", config.GlobalConfig.UseGoLibvirt, libvirtSocket)

	if !config.GlobalConfig.UseGoLibvirt {
		log.Println("[libvirt-rpc] 配置已禁用 go-libvirt，跳过初始化")
		return
	}

	l, err := dialLibvirt()
	if err != nil {
		log.Printf("[libvirt-rpc] 初始化连接失败（将降级为 virsh 命令行）: %v", err)
		startBackgroundReconnect()
		return
	}

	libvirtConnMu.Lock()
	libvirtConn = l
	libvirtConnMu.Unlock()

	// 验证连接：获取 libvirt 版本
	ver, err := l.ConnectGetLibVersion()
	if err != nil {
		log.Printf("[libvirt-rpc] 连接已建立但版本查询失败: %v", err)
	} else {
		log.Printf("[libvirt-rpc] 连接初始化成功 (libvirt 版本: %d.%d.%d)", ver/1000000, (ver/1000)%1000, ver%1000)
	}
}

// GetLibvirt 获取 libvirt RPC 连接（单例，自动重连）
// 返回 nil, err 表示连接不可用，调用方应降级为 virsh
func GetLibvirt() (*libvirt.Libvirt, error) {
	// 快速路径：读锁检查现有连接
	libvirtConnMu.RLock()
	conn := libvirtConn
	libvirtConnMu.RUnlock()

	if conn != nil {
		// 简单探测连接是否存活
		if _, err := conn.ConnectGetLibVersion(); err == nil {
			return conn, nil
		}
		// 连接已断开，尝试重连
	}

	// 慢路径：写锁内重连
	l, err := reconnectLibvirt()
	if err != nil {
		return nil, fmt.Errorf("go-libvirt RPC 不可用: %w", err)
	}

	libvirtConnMu.Lock()
	// 关闭旧连接（忽略错误）
	if libvirtConn != nil {
		_ = libvirtConn.Disconnect()
	}
	libvirtConn = l
	libvirtConnMu.Unlock()

	return l, nil
}

// IsLibvirtRPCAvailable 快速检测 go-libvirt RPC 是否可用
// 用于在各函数入口快速判断是否尝试 RPC 路径
// 纯内存检查，O(1) 性能，不做网络操作
func IsLibvirtRPCAvailable() bool {
	if config.GlobalConfig == nil || !config.GlobalConfig.UseGoLibvirt {
		return false
	}
	libvirtConnMu.RLock()
	available := libvirtConn != nil
	libvirtConnMu.RUnlock()
	return available
}

// CloseLibvirt 关闭 go-libvirt 连接（程序退出时调用）
func CloseLibvirt() {
	libvirtConnMu.Lock()
	defer libvirtConnMu.Unlock()

	if libvirtConn != nil {
		_ = libvirtConn.Disconnect()
		libvirtConn = nil
		log.Println("[libvirt-rpc] 连接已关闭")
	}
}

// reconnectLibvirt 内部重连逻辑（带退避）
// 最多重试 3 次，间隔 1s, 2s, 4s
func reconnectLibvirt() (*libvirt.Libvirt, error) {
	var lastErr error
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for i := 0; i < 3; i++ {
		if i > 0 {
			log.Printf("[libvirt-rpc] 第 %d 次重连，等待 %v ...", i+1, backoffs[i-1])
			time.Sleep(backoffs[i-1])
		}

		l, err := dialLibvirt()
		if err != nil {
			lastErr = err
			continue
		}
		return l, nil
	}

	return nil, fmt.Errorf("重连失败（已重试 3 次）: %w", lastErr)
}

// startBackgroundReconnect 在初始化失败时启动后台重连
// 最多重试 5 次，间隔递增 (5s, 10s, 15s, 20s, 25s)
func startBackgroundReconnect() {
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(time.Duration(5*(i+1)) * time.Second)

			if !config.GlobalConfig.UseGoLibvirt {
				return
			}

			l, err := dialLibvirt()
			if err != nil {
				log.Printf("[libvirt-rpc] 后台重连第 %d 次失败: %v", i+1, err)
				continue
			}

			libvirtConnMu.Lock()
			libvirtConn = l
			libvirtConnMu.Unlock()
			log.Println("[libvirt-rpc] 后台重连成功")
			return
		}
		log.Println("[libvirt-rpc] 后台重连放弃（已尝试 5 次），将持续使用 virsh 命令行")
	}()
}

// dialLibvirt 建立单次 go-libvirt RPC 连接
func dialLibvirt() (*libvirt.Libvirt, error) {
	conn, err := net.DialTimeout("unix", libvirtSocket, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接 unix socket %s 失败: %w", libvirtSocket, err)
	}

	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("RPC 握手失败: %w", err)
	}

	return l, nil
}
