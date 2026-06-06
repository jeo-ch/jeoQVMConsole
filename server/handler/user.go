package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"kvm_console/model"
	"kvm_console/service"
	"kvm_console/taskqueue"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username                   string                                     `json:"username" binding:"required"`
	Email                      string                                     `json:"email"`
	Password                   string                                     `json:"password"` // SMTP 未配置时必填
	Role                       string                                     `json:"role"`   // admin/user
	CloudType                  string                                     `json:"cloud_type"` // elastic/lightweight
	DedicatedVPCSwitchID       uint                                       `json:"dedicated_vpc_switch_id"`
	MaxCPU                     int                                        `json:"max_cpu"`           // CPU配额
	MaxMemory                  int                                        `json:"max_memory"`        // 内存配额(GB)
	MaxDisk                    int                                        `json:"max_disk"`          // 磁盘配额(GB)
	MaxVM                      int                                        `json:"max_vm"`            // 最大VM数量
	MaxStorage                 int                                        `json:"max_storage"`       // 存储配额(GB)
	MaxRuntimeHours            int                                        `json:"max_runtime_hours"` // 总运行时长配额(小时)
	EnablePortForward          *bool                                      `json:"enable_port_forward"`
	MaxPortForwards            *int                                       `json:"max_port_forwards"`  // 端口转发数量配额
	MaxSnapshots               *int                                       `json:"max_snapshots"`      // 快照数量配额
	MaxBandwidthUp             float64                                    `json:"max_bandwidth_up"`   // 上行带宽(Mbps)
	MaxBandwidthDown           float64                                    `json:"max_bandwidth_down"` // 下行带宽(Mbps)
	MaxTrafficDown             float64                                    `json:"max_traffic_down"`   // 下行日流量(GB)
	MaxTrafficUp               float64                                    `json:"max_traffic_up"`     // 上行日流量(GB)
	MaxPublicIPs               int                                        `json:"max_public_ips"`     // 公网 IP 数量
	LightweightVMRegistrations []service.LightweightVMRegistrationRequest `json:"lightweight_vm_registrations"`
	LightweightExistingVMs     []string                                   `json:"lightweight_existing_vms"`      // 选择已有VM列表
	LightweightExistingVMQuotas []service.LightweightVMQuotaRequest       `json:"lightweight_existing_vm_quotas"` // 已有VM配额
}

// UpdateQuotaRequest 更新配额请求
type UpdateQuotaRequest struct {
	MaxCPU               int     `json:"max_cpu"`
	MaxMemory            int     `json:"max_memory"`
	MaxDisk              int     `json:"max_disk"`
	MaxVM                int     `json:"max_vm"`
	MaxStorage           int     `json:"max_storage"`
	MaxRuntimeHours      int     `json:"max_runtime_hours"`
	EnablePortForward    *bool   `json:"enable_port_forward"`
	MaxPortForwards      int     `json:"max_port_forwards"`
	MaxSnapshots         int     `json:"max_snapshots"`
	MaxBandwidthUp       float64 `json:"max_bandwidth_up"`
	MaxBandwidthDown     float64 `json:"max_bandwidth_down"`
	MaxTrafficDown       float64 `json:"max_traffic_down"`
	MaxTrafficUp         float64 `json:"max_traffic_up"`
	MaxPublicIPs         int     `json:"max_public_ips"`
	CloudType            string  `json:"cloud_type"`
	DedicatedVPCSwitchID uint    `json:"dedicated_vpc_switch_id"`
}

// AssignVMsRequest 分配虚拟机请求
type AssignVMsRequest struct {
	VMs               []string                            `json:"vms" binding:"required"`
	LightweightQuotas []service.LightweightVMQuotaRequest `json:"lightweight_quotas"`
}

// UpdateUserStatusRequest 更新用户状态请求
type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// GetUserList 获取用户列表
func GetUserList(c *gin.Context) {
	users, err := service.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取用户列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    users,
	})
}

func resolveCreateUserMaxPortForwards(role string, value *int) int {
	if role != "user" {
		if value == nil {
			return 0
		}
		return *value
	}
	if value == nil {
		return 10
	}
	return *value
}

func resolveCreateUserMaxSnapshots(role string, value *int) int {
	if role != "user" {
		if value == nil {
			return 0
		}
		return *value
	}
	if value == nil {
		return 5
	}
	return *value
}

func resolveCreateUserEnablePortForward(role string, value *bool) bool {
	if value != nil {
		return *value
	}
	if role != "user" {
		return true
	}
	return true
}

func resolveUpdateUserEnablePortForward(username string, value *bool) (bool, error) {
	if value != nil {
		return *value, nil
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return false, err
	}
	return user.EnablePortForward, nil
}

// CreateUser 创建用户
func CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}
	cloudType := service.NormalizeCloudType(req.CloudType)
	enablePortForward := resolveCreateUserEnablePortForward(role, req.EnablePortForward)
	maxPortForwards := resolveCreateUserMaxPortForwards(role, req.MaxPortForwards)
	maxSnapshots := resolveCreateUserMaxSnapshots(role, req.MaxSnapshots)

	// SMTP 未配置时，邮件发送不可用，必须填写密码直接创建激活用户
	smtpConfigured := service.IsSMTPConfigured()
	if !smtpConfigured {
		email := strings.TrimSpace(req.Email)
		if email == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "SMTP 尚未配置，无法发送邀请邮件，请填写完整的用户信息（包括邮箱和密码）",
			})
			return
		}
		password := strings.TrimSpace(req.Password)
		if password == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "SMTP 未配置时，必须为用户设置初始密码",
			})
			return
		}
		if err := service.ValidateStrongPassword(password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "密码不符合要求: " + err.Error(),
			})
			return
		}

		// 直接创建激活用户
		user, err := service.CreateActiveUserDirectly(req.Username, email, password, role, cloudType,
			req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours,
			enablePortForward, maxPortForwards, maxSnapshots,
			req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs)
		if err != nil {
			if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "已被使用") {
				c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建用户失败: " + err.Error()})
			return
		}

		// 处理轻量云 VM 注册
		if service.IsLightweightCloudType(cloudType) && len(req.LightweightVMRegistrations) > 0 {
			if _, err := service.CreateLightweightVMRegistrations(user.Username, req.LightweightVMRegistrations, "admin"); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 200, "message": "用户已创建，但轻量云 VM 注册失败: " + err.Error(), "data": gin.H{"username": user.Username}})
				return
			}
		}
		if service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0 {
			quotaByVM := make(map[string]service.LightweightVMQuotaRequest)
			for _, quota := range req.LightweightExistingVMQuotas {
				quotaByVM[quota.VMName] = quota
			}
			quotas := make([]service.LightweightVMQuotaRequest, 0, len(req.LightweightExistingVMs))
			for _, vmName := range req.LightweightExistingVMs {
				if q, ok := quotaByVM[vmName]; ok {
					quotas = append(quotas, q)
				} else {
					quotas = append(quotas, service.LightweightVMQuotaRequest{
						VMName: vmName, MaxPortForwards: 10, MaxSnapshots: 2,
					})
				}
			}
			_ = service.AssignVMsToUserWithQuotas(user.Username, req.LightweightExistingVMs, quotas)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "用户已创建（SMTP 未配置，用户可直接使用初始密码登录）",
			"data":    gin.H{"username": user.Username},
		})
		return
	}

	// SMTP 已配置：原有邀请注册流程
	email := strings.TrimSpace(req.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "邮箱不能为空"})
		return
	}

	// 如果选择已有VM，不需要专用VPC
	dedicatedVPCSwitchID := req.DedicatedVPCSwitchID
	useExistingVMs := service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0
	if useExistingVMs {
		dedicatedVPCSwitchID = 0
	}

	user, inviteToken, err := service.CreatePendingInvitedUserWithExistingVMs(req.Username, req.Email, role, cloudType, dedicatedVPCSwitchID, useExistingVMs,
		req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours, enablePortForward, maxPortForwards, maxSnapshots,
		req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs)
	if err != nil {
		// 用户名或邮箱冲突返回 409 Conflict
		if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "已被使用") {
			c.JSON(http.StatusConflict, gin.H{
				"code":    409,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "创建用户失败: " + err.Error(),
		})
		return
	}

	if service.IsLightweightCloudType(cloudType) && len(req.LightweightVMRegistrations) > 0 {
		if _, err := service.CreateLightweightVMRegistrations(user.Username, req.LightweightVMRegistrations, "admin"); err != nil {
			model.DB.Delete(user)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "创建轻量云 VM 注册失败: " + err.Error(),
			})
			return
		}
	}

	// 分配已有VM给用户
	if service.IsLightweightCloudType(cloudType) && len(req.LightweightExistingVMs) > 0 {
		// 将配额请求转换为VM名到配额的映射
		quotaByVM := make(map[string]service.LightweightVMQuotaRequest)
		for _, quota := range req.LightweightExistingVMQuotas {
			quotaByVM[quota.VMName] = quota
		}
		
		// 构建配额请求列表
		quotas := make([]service.LightweightVMQuotaRequest, 0, len(req.LightweightExistingVMs))
		for _, vmName := range req.LightweightExistingVMs {
			if q, ok := quotaByVM[vmName]; ok {
				quotas = append(quotas, q)
			} else {
				quotas = append(quotas, service.LightweightVMQuotaRequest{
					VMName:          vmName,
					MaxPortForwards: 10,
					MaxSnapshots:    2,
				})
			}
		}
		
		if err := service.AssignVMsToUserWithQuotas(user.Username, req.LightweightExistingVMs, quotas); err != nil {
			model.DB.Delete(user)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "分配已有 VM 失败: " + err.Error(),
			})
			return
		}
	}

	inviteURL := buildBaseURL(c) + "/invite?token=" + inviteToken
	if sendErr := service.SendInviteEmail(user, inviteURL); sendErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "邀请用户已创建，但邮件发送失败，请检查 SMTP 配置后重发邀请",
			"data": gin.H{
				"invite_url": inviteURL,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邀请邮件已发送",
		"data": gin.H{
			"invite_url": inviteURL,
		},
	})
}

// CreateLightweightVMRegistrations 管理员为轻量云用户登记待开通 VM。
func CreateLightweightVMRegistrations(c *gin.Context) {
	username := c.Param("username")
	var req struct {
		Registrations []service.LightweightVMRegistrationRequest `json:"registrations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	if len(req.Registrations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请至少添加一台待注册 VM"})
		return
	}
	operator, _ := c.Get("username")
	createdBy, _ := operator.(string)
	regs, err := service.CreateLightweightVMRegistrations(username, req.Registrations, createdBy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	var user model.User
	if err := model.DB.Where("username = ?", username).First(&user).Error; err == nil && user.Status == service.UserStatusActive && strings.TrimSpace(user.Email) != "" {
		if sendErr := service.SendLightweightVMRegistrationEmail(&user, buildBaseURL(c)+"/vm/list", regs); sendErr != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "轻量云 VM 已登记，但确认邮件发送失败，请检查 SMTP 配置",
				"data":    regs,
			})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云 VM 已登记",
		"data":    regs,
	})
}

// DeleteLightweightVMRegistration 删除未开通的轻量云 VM 注册。
func DeleteLightweightVMRegistration(c *gin.Context) {
	username := c.Param("username")
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "注册记录 ID 无效"})
		return
	}
	if err := service.DeleteLightweightVMRegistration(username, uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "注册记录已删除"})
}

// RemoveLightweightVMRegistrationByVMName 将已开通 VM 从轻量云注册列表中移除。
func RemoveLightweightVMRegistrationByVMName(c *gin.Context) {
	username := c.Param("username")
	vmName := c.Param("vmName")
	if err := service.RemoveLightweightVMRegistrationByVMName(username, vmName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "轻量云 VM 已移除"})
}

// UpdateLightweightVMQuota 更新轻量云单 VM 的流量、带宽和端口转发配额。
func UpdateLightweightVMQuota(c *gin.Context) {
	username := c.Param("username")
	var req service.LightweightVMQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	quota, reg, err := service.UpdateLightweightVMQuotaByVMName(username, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云 VM 配额已更新",
		"data": gin.H{
			"quota":        quota,
			"registration": reg,
		},
	})
}

// ResendInvite 重发邀请
func ResendInvite(c *gin.Context) {
	username := c.Param("username")
	user, inviteToken, err := service.ResendInviteToken(username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	inviteURL := buildBaseURL(c) + "/invite?token=" + inviteToken
	if err := service.SendInviteEmail(user, inviteURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "重发邀请失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "邀请邮件已重发",
		"data": gin.H{
			"invite_url": inviteURL,
		},
	})
}

// UpdateUserQuota 更新用户配额
func UpdateUserQuota(c *gin.Context) {
	username := c.Param("username")
	var req UpdateQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	enablePortForward, err := resolveUpdateUserEnablePortForward(username, req.EnablePortForward)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "读取端口转发开关失败: " + err.Error(),
		})
		return
	}

	cloudType := service.NormalizeCloudType(req.CloudType)
	if strings.TrimSpace(req.CloudType) == "" {
		var user model.User
		if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "读取用户类型失败: " + err.Error(),
			})
			return
		}
		cloudType = service.NormalizeCloudType(user.CloudType)
		req.DedicatedVPCSwitchID = user.DedicatedVPCSwitchID
	}
	if err := service.UpdateUserCloudProfile(username, cloudType, req.DedicatedVPCSwitchID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新用户类型失败: " + err.Error(),
		})
		return
	}

	if !service.IsLightweightCloudType(cloudType) {
		if err := service.UpdateUserQuota(username, req.MaxCPU, req.MaxMemory, req.MaxDisk, req.MaxVM, req.MaxStorage, req.MaxRuntimeHours, enablePortForward, req.MaxPortForwards, req.MaxSnapshots,
			req.MaxBandwidthUp, req.MaxBandwidthDown, req.MaxTrafficDown, req.MaxTrafficUp, req.MaxPublicIPs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "更新配额失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配额更新成功",
	})
}

// GetUserQuotaUsage 获取用户配额使用情况
func GetUserQuotaUsage(c *gin.Context) {
	username := c.Param("username")

	usage, err := service.GetUserQuotaUsage(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取配额信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    usage,
	})
}

// AssignVMs 分配虚拟机给用户
func AssignVMs(c *gin.Context) {
	username := c.Param("username")
	var req AssignVMsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := service.AssignVMsToUserWithQuotas(username, req.VMs, req.LightweightQuotas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "分配失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "虚拟机分配成功",
	})

	// 分配后重新计算该用户所有 VM 的带宽
	assignedVMs := append([]string(nil), req.VMs...)
	go func() {
		if service.IsLightweightCloudUser(username) {
			for _, vmName := range assignedVMs {
				if err := service.ApplyLightweightVMBandwidth(vmName); err != nil {
					fmt.Printf("[警告] 分配轻量云 VM %s 后应用带宽失败: %v\n", vmName, err)
				}
			}
			return
		}
		if err := service.RebalanceUserBandwidth(username); err != nil {
			fmt.Printf("[警告] 分配VM后重新分配用户 %s 带宽失败: %v\n", username, err)
		}
	}()
}

// UpdateUserStatus 更新用户状态（封禁/解封）
func UpdateUserStatus(c *gin.Context) {
	if !requireHighRiskVerification(c, "change_user_status") {
		return
	}

	username := c.Param("username")
	var req UpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	targetStatus := strings.TrimSpace(req.Status)
	operator, _ := c.Get("username")

	switch targetStatus {
	case service.UserStatusDisabled:
		params := map[string]string{"username": username}
		task, err := taskqueue.SubmitWithStruct(model.TaskTypeDisableUser, params, operator.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "提交封禁用户任务失败: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "封禁用户任务已提交",
			"data": gin.H{
				"task_id": task.ID,
			},
		})
	case service.UserStatusActive:
		if err := service.UpdateUserStatus(username, service.UserStatusActive); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "用户已解封",
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不支持的用户状态",
		})
	}
}

// DeleteUser 删除用户（异步任务，级联删除所有资产）
func DeleteUser(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_user") {
		return
	}
	username := c.Param("username")

	// 不允许删除 admin 用户
	if username == "admin" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "不能删除管理员用户",
		})
		return
	}

	// 提交异步删除用户任务
	params := map[string]string{"username": username}
	operator, _ := c.Get("username")
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDeleteUser, params, operator.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交删除用户任务失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除用户任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ToggleUserSSHRequest SSH 开关请求
type ToggleUserSSHRequest struct {
	Enabled bool `json:"enabled"`
}

// ToggleUserSSH 切换用户 SSH 访问权限
func ToggleUserSSH(c *gin.Context) {
	username := c.Param("username")
	var req ToggleUserSSHRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	if err := service.SetUserSSH(username, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "切换 SSH 状态失败: " + err.Error(),
		})
		return
	}

	status := "已关闭"
	if req.Enabled {
		status = "已开启"
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "用户 " + username + " 的 SSH 访问" + status,
	})
}

// ==================== 用户自助 API ====================

// GetSelfQuota 获取当前用户的配额信息
func GetSelfQuota(c *gin.Context) {
	username, _ := c.Get("username")

	usage, err := service.GetUserQuotaUsage(username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取配额信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    usage,
	})
}

// GetSelfVMs 获取当前用户的VM列表
func GetSelfVMs(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	usernameStr, _ := username.(string)

	// 管理员返回所有VM
	if role == "admin" {
		service.TriggerAdminVMCacheRefreshIfNeeded()
		vms, err := service.ListCachedVMs(buildVMListOptions(c))
		if err != nil {
			respondVMListError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "ok",
			"data":    vms,
		})
		return
	}

	// 普通用户只返回自己的VM
	allVMs, err := service.ListCachedVMsByOwner(usernameStr, buildVMListOptions(c))
	if err != nil {
		respondVMListError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    allVMs,
	})
}

// GetSelfVMsSSE SSE 实时推送当前用户的VM列表
func GetSelfVMsSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	username, _ := c.Get("username")
	role, _ := c.Get("role")
	usernameStr, _ := username.(string)
	isAdmin := role == "admin"

	clientGone := c.Request.Context().Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	listOptions := buildVMListOptions(c)

	// 获取用户的 VM 列表数据
	getUserVMs := func() ([]service.VmInfo, error) {
		if isAdmin {
			service.TriggerAdminVMCacheRefreshIfNeeded()
			return service.ListCachedVMs(listOptions)
		}
		return service.ListCachedVMsByOwner(usernameStr, listOptions)
	}

	// 立即发送一次
	if vms, err := getUserVMs(); err == nil {
		c.SSEvent("vm_list", vms)
		c.Writer.Flush()
	}

	for {
		select {
		case <-clientGone:
			return
		case <-ticker.C:
			vms, err := getUserVMs()
			if err != nil {
				if service.IsLibvirtUnavailableError(err) {
					c.SSEvent("vm_list", []service.VmInfo{})
					c.Writer.Flush()
				}
				continue
			}
			c.SSEvent("vm_list", vms)
			c.Writer.Flush()
		}
	}
}

// GetSelfLightweightVMRegistrations 获取当前用户待确认轻量云服务器。
func GetSelfLightweightVMRegistrations(c *gin.Context) {
	username, _ := c.Get("username")
	usernameStr := username.(string)
	regs, err := service.ListLightweightVMRegistrations(usernameStr, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取待开通服务器失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": regs})
}

// ConfirmSelfLightweightVMRegistration 用户确认并开通轻量云服务器。
func ConfirmSelfLightweightVMRegistration(c *gin.Context) {
	if !requireHighRiskVerification(c, "create_vm") {
		return
	}
	if !requireMaintenanceModeDisabled(c, "开通轻量云服务器") {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "注册记录 ID 无效"})
		return
	}
	var req service.LightweightVMConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误"})
		return
	}
	username, _ := c.Get("username")
	usernameStr := username.(string)
	params, err := service.BuildLightweightVMProvisionParams(uint(id), usernameStr, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	task, err := taskqueue.SubmitWithStruct(model.TaskTypeLightweightVMProvision, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "提交开通任务失败: " + err.Error()})
		return
	}
	service.MarkLightweightVMRegistrationTask(params.RegistrationID, task.ID)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "轻量云服务器开通任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// SelfCloneVmRequest 用户自助克隆请求
type SelfCloneVmRequest struct {
	Name                 string                          `json:"name" binding:"required"`
	Remark               string                          `json:"remark"`
	Template             string                          `json:"template" binding:"required"`
	TemplateType         string                          `json:"template_type"`
	CloneMode            string                          `json:"clone_mode"`
	VCPU                 int                             `json:"vcpu" binding:"required"`
	RAM                  int                             `json:"ram" binding:"required"`
	DiskSize             int                             `json:"disk_size"`
	Hostname             string                          `json:"hostname"`
	User                 string                          `json:"user"`
	Password             string                          `json:"password"`
	Autostart            bool                            `json:"autostart"`
	Freeze               bool                            `json:"freeze"`
	APIC                 *bool                           `json:"apic"`
	PAE                  *bool                           `json:"pae"`
	RTCOffset            string                          `json:"rtc_offset"`
	RTCStartDate         string                          `json:"rtc_startdate"`
	GuestAgent           *service.VMGuestAgentConfig     `json:"guest_agent"`
	SMBIOS1              *service.VMSMBIOS1Config        `json:"smbios1"`
	UEFI                 *bool                           `json:"uefi"`
	DiskBus              string                          `json:"disk_bus"`
	VideoModel           string                          `json:"video_model"`
	CPUTopologyMode      string                          `json:"cpu_topology_mode"`
	FirstBootRebootMode  string                          `json:"first_boot_reboot_mode"`
	MemoryDynamic        *service.VMMemoryDynamicRequest `json:"memory_dynamic"`
	SwitchID             uint                            `json:"switch_id"`
	SecurityGroupID      uint                            `json:"security_group_id"`
	StoragePoolID        string                          `json:"storage_pool_id"`
	ExtraDisks           []service.ExtraDiskParam        `json:"extra_disks"`
	NicModel             string                          `json:"nic_model"`
	PreserveFnOSDeviceID bool                            `json:"preserve_fnos_device_id"`
	FnOSDeviceID         string                          `json:"fnos_device_id"`
}

// SelfCloneVm 用户自助从模板克隆VM
func SelfCloneVm(c *gin.Context) {
	if !requireMaintenanceModeDisabled(c, "克隆并启动虚拟机") {
		return
	}
	var req SelfCloneVmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "参数错误",
		})
		return
	}

	username, _ := c.Get("username")
	usernameStr := username.(string)
	if err := service.ValidateVMName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	if err := service.EnsureTemplateVisibleForClone(req.Template, false); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	// 从模板元数据获取类型
	meta := service.GetTemplateMeta(req.Template)
	templateType := req.TemplateType
	if templateType == "" {
		templateType = meta.Type
	}
	req.User = service.NormalizeCloneUsernameForTemplate(templateType, req.User)

	if err := service.ValidateCloneCredentialsForTemplate(templateType, req.Hostname, req.User, req.Password, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}
	if strings.TrimSpace(req.FnOSDeviceID) != "" {
		if err := service.ValidateFnOSDeviceID(req.FnOSDeviceID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
		req.PreserveFnOSDeviceID = true
	}

	diskSize, err := service.ResolveCloneDiskSizeGB(req.Template, req.DiskSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 配额检查：模板系统盘和额外数据盘都计入普通用户硬盘配额。
	totalDiskGB := diskSize
	for _, disk := range req.ExtraDisks {
		if disk.Size > 0 {
			totalDiskGB += disk.Size
		}
	}
	if err := service.CheckQuota(usernameStr, req.VCPU, req.RAM, totalDiskGB); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}
	switchID, securityGroupID, err := service.ResolveVPCForVMCreate(usernameStr, req.SwitchID, req.SecurityGroupID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": err.Error(),
		})
		return
	}

	params := &service.CloneParams{
		Name:                 req.Name,
		Remark:               req.Remark,
		Template:             req.Template,
		TemplateType:         templateType,
		CloneMode:            req.CloneMode,
		VCPU:                 req.VCPU,
		RAM:                  req.RAM,
		DiskSize:             diskSize,
		Hostname:             req.Hostname,
		User:                 req.User,
		Password:             req.Password,
		Autostart:            req.Autostart,
		Freeze:               req.Freeze,
		APIC:                 req.APIC,
		PAE:                  req.PAE,
		RTCOffset:            req.RTCOffset,
		RTCStartDate:         req.RTCStartDate,
		GuestAgent:           req.GuestAgent,
		SMBIOS1:              req.SMBIOS1,
		UEFI:                 req.UEFI,
		DiskBus:              req.DiskBus,
		VideoModel:           req.VideoModel,
		CPUTopologyMode:      req.CPUTopologyMode,
		FirstBootRebootMode:  req.FirstBootRebootMode,
		TemplateRootPass:     meta.RootPassword,
		TemplateUser:         meta.TemplateUser,
		MemoryDynamic:        sanitizeUserMemoryDynamicRequest(req.MemoryDynamic, req.RAM),
		SwitchID:             switchID,
		SecurityGroupID:      securityGroupID,
		StoragePoolID:        req.StoragePoolID,
		ExtraDisks:           req.ExtraDisks,
		NicModel:             req.NicModel,
		PreserveFnOSDeviceID: req.PreserveFnOSDeviceID,
		FnOSDeviceID:         req.FnOSDeviceID,
		IsAdmin:              false,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeClone, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交克隆任务失败: " + err.Error(),
		})
		return
	}

	// 将VM添加到用户的访问列表（任务还未完成，但先注册归属）
	_ = service.AddVMToUser(usernameStr, req.Name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "克隆任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// SelfDeleteVmRequest 用户自助删除VM请求
type SelfDeleteVmRequest struct {
	DeleteDisks   []string `json:"delete_disks"`   // 要删除的磁盘路径列表
	TransferDisks []string `json:"transfer_disks"` // 要转移到用户存储的磁盘路径列表
}

// SelfDeleteVm 用户自助删除VM
func SelfDeleteVm(c *gin.Context) {
	if !requireHighRiskVerification(c, "delete_vm") {
		return
	}
	name := c.Param("name")
	username, _ := c.Get("username")
	usernameStr := username.(string)

	// 检查虚拟机是否已锁定
	if service.IsVMLocked(name) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "该虚拟机已锁定，无法删除。请先解锁后再操作。",
		})
		return
	}

	// 检查用户是否拥有该VM
	if !service.UserOwnsVM(usernameStr, name) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    403,
			"message": "无权操作此虚拟机",
		})
		return
	}

	var req SelfDeleteVmRequest
	c.ShouldBindJSON(&req) // 可选参数

	// 如果有需要转移的磁盘，先检查存储池是否已初始化，再检查配额
	if len(req.TransferDisks) > 0 {
		// 检查存储池是否已开通
		if !service.IsStorageInitialized(usernameStr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "您尚未开通「我的存储」，无法转移磁盘。请先前往「我的存储」页面初始化存储池，或勾选所有磁盘直接删除。",
			})
			return
		}
		// 检查存储配额
		_, err := service.CheckDiskTransferQuota(usernameStr, req.TransferDisks)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": err.Error(),
			})
			return
		}
	}

	params := map[string]interface{}{
		"name":           name,
		"delete_disks":   req.DeleteDisks,
		"transfer_disks": req.TransferDisks,
		"transfer_user":  usernameStr,
	}

	task, err := taskqueue.SubmitWithStruct(model.TaskTypeDelete, params, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交删除任务失败: " + err.Error(),
		})
		return
	}

	// 从用户访问列表中移除
	_ = service.RemoveVMFromUser(usernameStr, name)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "删除任务已提交",
		"data": gin.H{
			"task_id": task.ID,
		},
	})
}

// ResetUserTraffic 重置用户流量配额
func ResetUserTraffic(c *gin.Context) {
	username := c.Param("username")

	if err := service.ResetUserTrafficQuota(username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "重置流量配额失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "用户 " + username + " 的流量配额已重置",
	})
}
