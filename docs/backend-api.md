# QVMConsole 后端 API 文档

## 概述

QVMConsole 后端使用 Go + Gin 开发，提供虚拟机、用户配额与账户安全相关 API。

- 默认监听端口：`8080`
- 认证方式：`Authorization: Bearer <token>`；外部程序也可使用 API Key 请求头
- 用户与任务数据存储在数据库中，虚拟机与大多数宿主机资源仍优先通过命令实时读取

## API 限频

全局 API 限频基于客户端 IP 的滑动窗口计数，分为公开接口和认证接口两档：

| 接口类型 | 默认限制 | 环境变量 |
|---------|---------|---------|
| 公开接口（`/api/public/*`, `/api/auth/*` 等无需认证的路由） | 20 次/分钟/IP | `KVM_RATE_LIMIT_PUBLIC` |
| 认证接口（需 Bearer Token 或 API Key） | 60 次/分钟/IP | `KVM_RATE_LIMIT_AUTH` |

- 超限返回 `HTTP 429 Too Many Requests`，响应头包含 `Retry-After`（秒）
- 正常请求响应头包含 `X-RateLimit-Remaining`（剩余次数）和 `X-RateLimit-Reset`（重置秒数）
- 设置为 `0` 可禁用对应档位的限频
- 客户端 IP 优先从 `X-Forwarded-For` 和 `X-Real-IP` 请求头获取

## 虚拟机列表缓存说明

虚拟机列表相关接口现在默认优先读取数据库缓存表 `vm_caches`，不再让每次列表请求都阻塞等待宿主机全量 `virsh` 扫描。

涉及接口：

- `GET /api/vm/list`
- `GET /api/vm/sse`
- `GET /api/self/vms`
- `GET /api/self/vms/sse`

行为说明：

- 管理员访问时，接口会立即返回数据库缓存，同时异步触发一次带冷却时间的宿主机刷新
- 普通用户访问时，接口始终按 `owner_username` 从数据库缓存过滤
- `present=false` 的缓存记录默认不会返回给前端
- 单台详情、IP、磁盘、迁移、开关机等接口仍然直接读取宿主机真实状态

当前列表接口仍支持的常用查询参数：

- `include_resource_usage=1`：返回运行中 VM 的 CPU / 内存缓存数据
- `include_ip=1`：返回数据库中缓存的 IP 字段；若不传，列表页可继续按需请求单台 IP

## 关键环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KVM_PORT` | `8080` | 服务端口 |
| `KVM_DB_PATH` | `./data/kvm_console.db` | SQLite 数据库路径 |
| `KVM_JWT_SECRET` | **必须设置**（默认值会导致启动失败） | JWT 签名密钥，生产环境必须设置为随机强密钥 |
| `KVM_SECURITY_SECRET` | 回退到 `KVM_JWT_SECRET` | 安全敏感数据（TOTP、SMTP密码）加密密钥，建议独立设置 |
| `KVM_VM_CREDENTIAL_SECRET` | 回退到 `KVM_JWT_SECRET` | 虚拟机凭据加密密钥，建议独立设置 |
| `KVM_JWT_EXPIRE_HOURS` | `24` | 访问令牌过期时间 |
| `KVM_JWT_SECRET_ROTATE_HOURS` | `24` | JWT 密钥自动轮换间隔（小时，0=禁用） |
| `KVM_TEMPLATE_DIR` | `/var/lib/libvirt/images/templates` | 模板目录 |
| `KVM_TEMPLATE_IMPORT_DIR` | `/var/lib/libvirt/images/templates/_imports` | 模板导入临时目录 |
| `KVM_TEMPLATE_EXPORT_DIR` | `/var/lib/libvirt/images/templates/_exports` | 模板导出目录 |
| `KVM_CLONE_DIR` | `/var/lib/libvirt/images` | 克隆磁盘目录 |
| `KVM_ISO_DIR` | `/var/lib/libvirt/images/ISO` | 全局 ISO 目录 |
| `KVM_DEFAULT_NETWORK` | `default` | 默认网络 |
| `KVM_NETWORK_BACKEND` | `ovs` | 网络后端，当前新平台使用 OVS |
| `KVM_OVS_BRIDGE` | `br-ovs` | OVS 网桥名称 |
| `KVM_OVS_UPLINK` | 自动检测 | OVS NAT 出口网卡 |
| `KVM_OVS_DHCP_START` | 自动 | OVS DHCP 起始地址 |
| `KVM_OVS_DHCP_END` | 自动 | OVS DHCP 结束地址 |
| `KVM_SUBNET_PREFIX` | `192.168.122` | 默认网段前缀 |
| `KVM_AUTO_PORT_START` | `10000` | 自动端口起始 |
| `KVM_AUTO_PORT_END` | `20000` | 自动端口终止 |
| `KVM_PUBLIC_BASE_URL` | 空 | 邮件中使用的面板访问地址 |
| `KVM_SITE_TITLE` | `QVMConsole` | 网站标题 |
| `KVM_DEVELOPMENT_MODE` | `false` | 开发环境模式，启用后绕过安全验证 |
| `KVM_SMTP_HOST` | 空 | SMTP 主机 |
| `KVM_SMTP_PORT` | `587` | SMTP 端口 |
| `KVM_SMTP_USERNAME` | 空 | SMTP 用户名 |
| `KVM_SMTP_PASSWORD_ENC` | 空 | 已加密的 SMTP 密码 |
| `KVM_SMTP_FROM_NAME` | `QVMConsole` | 发件人名称 |
| `KVM_SMTP_FROM_ADDRESS` | 空 | 发件邮箱 |
| `KVM_SMTP_SECURITY` | `starttls` | `none / starttls / ssl` |
| `KVM_SMTP_TIMEOUT_SECONDS` | `15` | SMTP 超时秒数 |
| `KVM_SCHEDULER_EVENT_RETENTION_HOURS` | `168` | 调度事件保留时长（小时） |

## 认证与账户安全

### API Key 外部调用

用户可在右上角账户菜单的“安全设置 -> API”中生成 API ID 和 API Key。API Key 属于当前用户，仍受角色、VM 归属、轻量云限制和配额限制约束。

推荐请求头：

```http
X-API-Key-ID: kvm_id_xxx
X-API-Key: kvm_sk_xxx
```

兼容格式：

```http
Authorization: ApiKey kvm_id_xxx:kvm_sk_xxx
```

API Key 管理接口：

- `GET /api/auth/api-key`：查看当前用户 API Key 状态
- `POST /api/auth/api-key`：生成或重新生成 API Key，需要高风险二次验证
- `DELETE /api/auth/api-key`：撤销 API Key，需要高风险二次验证

登录、邀请注册、找回密码、安全初始化、邮箱绑定和 2FA 绑定/关闭等账户安全流程只接受 JWT 流程令牌，不接受 API Key。

管理员 API Key 调用任意业务接口时不会触发敏感操作二次验证，便于节点间自动化接管；普通用户 API Key 和浏览器 Session 不会绕过二次验证。若业务接口返回 `428`，调用方需要先调用 `POST /api/auth/high-risk/verify`，再在原请求中携带 `X-High-Risk-Token`。

### Token 类型

系统内部存在 4 类令牌：

- `access`：正常访问业务接口
- `bootstrap`：安全初始化阶段，只允许安全配置与 SMTP 设置
- `login`：登录验证阶段，只允许继续完成本次登录验证
- `high_risk`：高风险 TOTP 二次校验后下发的短期令牌

### JWT 密钥安全

- 生产环境启动时必须设置 `KVM_JWT_SECRET` 为随机强密钥，否则服务拒绝启动
- `KVM_JWT_SECRET_ROTATE_HOURS` 默认 24 小时自动轮换 JWT 密钥（设为 0 禁用）
- 管理员可在系统设置页面手动触发密钥轮换（需高风险二次验证）
- 密钥轮换后所有旧 Token 立即失效，用户需重新登录
- 开发模式（`KVM_DEVELOPMENT_MODE=true`）下允许使用默认密钥启动并跳过自动轮换

### 登录阶段

`POST /api/auth/login`

请求体：

```json
{
  "username": "admin",
  "password": "YourPassword123!"
}
```

响应有 3 种阶段：

1. `stage=success`

```json
{
  "code": 200,
  "message": "登录成功",
  "data": {
    "stage": "success",
    "token": "access-token",
    "username": "admin",
    "role": "admin",
    "security": {
      "email": "admin@example.com",
      "masked_email": "a***n@example.com",
      "email_verified": true,
      "totp_enabled": true,
      "must_bind_email": false,
      "must_bind_2fa": false,
      "requires_login_verify": false,
      "smtp_configured": true,
      "status": "active",
      "login_verified_until": "2026-04-26T10:00:00+08:00",
      "high_risk_method": "totp",
      "has_recovery_codes": true
    }
  }
}
```

2. `stage=login_verify`

- 管理员每次登录都必须通过 `totp`
- 普通用户在 24 小时窗口外首次登录时必须二选一验证
- 返回 `login token`，前端继续调用登录验证接口
- 若启用了 `development_mode`，该阶段会被直接跳过

3. `stage=bootstrap_security`

- 管理员首次进入需要完成 `SMTP -> 邮箱绑定 -> 2FA 绑定`
- 普通用户邮箱未绑定时进入安全初始化
- 返回 `bootstrap token`
- 若启用了 `development_mode`，该阶段会被直接跳过

### 当前用户信息

`GET /api/auth/info`

- 需要 `access token`
- 返回当前用户基础信息与 `security` 对象

### 登录期验证

`POST /api/auth/login/email/send`

- 需要 `login token`
- 普通用户登录邮箱验证发送验证码

`POST /api/auth/login/verify`

请求体：

```json
{
  "method": "totp",
  "code": "123456",
  "challenge_id": 0
}
```

说明：

- `method=totp` 时不需要 `challenge_id`
- `method=email` 时必须携带 `challenge_id`
- 普通用户验证成功后会刷新 24 小时登录验证窗口

### 邮箱绑定与 2FA

`POST /api/auth/email/code/send`

- 需要 `access` 或 `bootstrap token`
- 用于邮箱绑定/换绑发送验证码

请求体：

```json
{
  "email": "user@example.com"
}
```

`POST /api/auth/email/bind`

```json
{
  "email": "user@example.com",
  "code": "123456",
  "challenge_id": 12
}
```

`POST /api/auth/2fa/setup`

- 需要 `access` 或 `bootstrap token`
- 返回 `secret` 与 `otpauth_url`

`POST /api/auth/2fa/enable`

```json
{
  "secret": "BASE32SECRET",
  "code": "123456"
}
```

`POST /api/auth/2fa/disable`

```json
{
  "password": "CurrentPassword123!",
  "code": "123456"
}
```

### 邀请注册

`GET /api/auth/invite?token=<token>`

- 公开接口
- 返回用户名、邮箱、角色、配额、邀请过期时间

`POST /api/auth/invite/complete`

```json
{
  "token": "invite-token",
  "password": "StrongPassword123!",
  "confirm_password": "StrongPassword123!"
}
```

说明：

- 邀请链接默认有效期 72 小时
- 若已配置 `public_base_url`，邮件中的链接会优先使用该地址
- 完成后自动激活账户、同步系统用户资源并登录

### 邮箱找回密码

推荐前端流程：

`POST /api/auth/password/forgot/send-code`

```json
{
  "email": "user@example.com"
}
```

说明：

- 向目标邮箱发送 10 分钟有效验证码
- 该接口不要求邮箱只绑定一个账号

`POST /api/auth/password/forgot/verify-code`

```json
{
  "email": "user@example.com",
  "code": "123456",
  "challenge_id": 12
}
```

说明：

- 校验成功后返回该邮箱下所有可重置的已激活账号
- 同时返回短期 `selection_token`，用于下一步选择账号

`POST /api/auth/password/forgot/select-account`

```json
{
  "selection_token": "selection-jwt-token",
  "username": "student01"
}
```

说明：

- 返回 `reset_token`
- 前端可直接跳转到 `/reset-password?token=<reset_token>`

兼容旧接口：

`POST /api/auth/password/forgot`

```json
{
  "email": "user@example.com"
}
```

说明：

- 旧接口仍会向邮箱发送重置链接，适合单账号邮箱的兼容调用
- 找回链接默认有效期 1 小时
- 若已配置 `public_base_url`，邮件中的链接会优先使用该地址
- 即使邮箱不存在，也返回统一成功提示，避免枚举账户

`POST /api/auth/password/reset`

```json
{
  "token": "reset-token",
  "password": "StrongPassword123!",
  "confirm_password": "StrongPassword123!"
}
```

说明：

- `token` 既支持旧版邮件链接中的重置令牌，也支持新流程返回的 `reset_token`

### 修改密码与用户名

`PUT /api/auth/password`

```json
{
  "old_password": "CurrentPassword123!",
  "new_password": "StrongPassword123!"
}
```

说明：

- 属于高风险操作
- 修改成功后需要重新登录

`PUT /api/auth/username`

```json
{
  "new_username": "newname",
  "password": "CurrentPassword123!"
}
```

### 高风险操作验证

当接口需要二次验证时，后端会返回 `HTTP 428`：

```json
{
  "code": 428,
  "message": "当前操作需要额外验证",
  "data": {
    "operation": "delete_vm",
    "method": "email",
    "challenge_id": 15,
    "masked_email": "u***r@example.com"
  }
}
```

处理规则：

- 已绑定 2FA：使用 `totp`
- 未绑定 2FA：使用邮箱验证码
- 邮箱高风险验证通过后，全局信任 1 小时
- TOTP 高风险验证按次生效
- 若启用了 `development_mode`，高风险验证会被直接绕过

验证接口：

`POST /api/auth/high-risk/verify`

```json
{
  "method": "totp",
  "code": "123456",
  "challenge_id": 0,
  "operation": "delete_vm"
}
```

当 `method=totp` 成功时返回：

```json
{
  "code": 200,
  "data": {
    "verification_token": "high-risk-token"
  }
}
```

前端需在原请求头中附加：

```text
X-High-Risk-Token: <verification_token>
```

## 用户管理

### 获取用户列表

`GET /api/user/list`

- 需要管理员权限
- 返回邮箱、状态、配额、虚拟机列表、SSH 状态等

### 创建邀请用户

`POST /api/user`

```json
{
  "username": "student01",
  "email": "student01@example.com",
  "role": "user",
  "cloud_type": "elastic",
  "max_cpu": 4,
  "max_memory": 8,
  "max_disk": 100,
  "max_vm": 5,
  "max_storage": 20
}
```

轻量云用户创建示例（选择已有 VM）：

```json
{
  "username": "lightweight01",
  "email": "lightweight01@example.com",
  "role": "user",
  "cloud_type": "lightweight",
  "lightweight_existing_vms": ["vm-001", "vm-002"],
  "lightweight_existing_vm_quotas": [
    {
      "vm_name": "vm-001",
      "traffic_down_gb": 100,
      "traffic_up_gb": 50,
      "bandwidth_down_mbps": 100,
      "bandwidth_up_mbps": 50,
      "max_port_forwards": 10,
      "max_snapshots": 2,
      "max_runtime_hours": 0
    }
  ]
}
```

轻量云用户创建示例（注册新 VM）：

```json
{
  "username": "lightweight02",
  "email": "lightweight02@example.com",
  "role": "user",
  "cloud_type": "lightweight",
  "dedicated_vpc_switch_id": 1,
  "lightweight_vm_registrations": [
    {
      "vm_name": "new-vm-001",
      "template": "ubuntu-22.04",
      "vcpu": 2,
      "ram": 4,
      "disk_size": 50,
      "traffic_down_gb": 100,
      "traffic_up_gb": 50,
      "bandwidth_down_mbps": 100,
      "bandwidth_up_mbps": 50,
      "max_port_forwards": 10,
      "max_snapshots": 2,
      "max_runtime_hours": 0
    }
  ]
}
```

说明：

- 不再直接设置密码
- 用户先进入 `pending_invite`
- 邮件发送失败时仍会返回可手动使用的 `invite_url`
- 轻量云用户支持两种 VM 来源：
  - `lightweight_existing_vms`：选择已有 VM，无需专用 VPC
  - `lightweight_vm_registrations`：注册新 VM，需要专用 VPC
- 选择已有 VM 时，通过 `lightweight_existing_vm_quotas` 设置每台 VM 的配额

### 重发邀请

`POST /api/user/:username/resend-invite`

### 更新配额

`PUT /api/user/:username/quota`

### 封禁/解封用户

`PUT /api/user/:username/status`

请求体：

```json
{
  "status": "disabled"
}
```

说明：

- `disabled`：封禁用户，并异步关闭其运行中的虚拟机和 SSH 访问
- `active`：解封用户，立即恢复账户可登录状态
- 仅普通已激活用户支持该操作
- 属于高风险操作，管理员执行时需要先完成二次验证

### 分配虚拟机

`PUT /api/user/:username/vms`

### 删除用户

`DELETE /api/user/:username`

- 属于高风险操作
- 返回异步任务信息

## 节点管理与虚拟机迁移

节点管理接口均需要管理员权限，并兼容管理员 API Key。

- `GET /api/nodes`：获取节点列表。
- `POST /api/nodes`：添加节点，请求体包含 `name`、`api_base_url`、`api_key_id`、`api_key`、`ssh_host`、`ssh_port`、`ssh_user`、`ssh_password`、`enabled`。
- `PUT /api/nodes/:id`：更新节点，`api_key` 和 `ssh_password` 留空表示不修改。
- `DELETE /api/nodes/:id`：删除节点。
- `POST /api/nodes/:id/probe`：探测节点能力。
- `GET /api/nodes/:id/migration-options?vm_name=<name>`：加载迁移表单选项，返回自动迁移模式、目标存储、VPC/安全组和用户处理信息。

迁移接口：

- `POST /api/vm/:name/migration/preview`：预检迁移，返回磁盘链校验、目标用户处理、VPC/轻量云 VPC、端口转发、`preview_id` 和阻止原因。该接口可选，适合提交前确认。
- `POST /api/vm/:name/migrate`：提交 `vm_migrate` 任务，属于高风险操作。携带 `preview_id` 时任务会复用预检结果；未携带时任务开始后会自动生成执行计划。
- `POST /api/migration/adopt-vm`：目标面板接管已迁移 VM，由源节点迁移任务使用。
- `GET /api/vm/:name/disk-migration/options`：获取本机硬盘迁移选项，返回自动冷热模式、可迁移硬盘和本机目标存储。
- `POST /api/vm/:name/disk/:dev/migrate`：提交 `vm_disk_migrate` 任务，属于高风险操作。该接口只迁移本机硬盘，不跨节点。

迁移预检请求体：

```json
{
  "node_id": 1,
  "target_storage_pool_id": "sda",
  "target_switch_id": 2,
  "target_security_group_id": 4,
  "enable_cpu_throttle": false,
  "cpu_throttle_percent": 50
}
```

提交迁移请求体：

```json
{
  "node_id": 1,
  "preview_id": "mig_xxx",
  "skip_precheck": false,
  "target_storage_pool_id": "sda",
  "target_switch_id": 2,
  "target_security_group_id": 4,
  "enable_cpu_throttle": false,
  "cpu_throttle_percent": 50
}
```

说明：

- 迁移模式由源 VM 当前状态自动决定：运行中为热迁移，关机为冷迁移。
- 热迁移预检会返回 `live_assessment`，包含平均带宽、dirty-rate、占比、CPU 限制决策和 `kvm_page_fault` 辅助信息。
- 热迁移 dirty-rate 占平均带宽 `<20%` 允许迁移；`20%-50%` 强制 CPU 限制；`>=50%` 阻止迁移。
- 迁移任务等待或执行期间，VM 状态会显示为 `migrating`，用户侧 VM 操作会返回“虚拟机正在迁移中”的中文错误。
- 目标存储必须选择，overlay 磁盘会落到目标存储的 VM 目录。
- 表单内容、VM 状态或目标节点资源发生变化时，已缓存的 `preview_id` 会失效；可重新预检，也可不带 `preview_id` 直接提交。
- `skip_precheck=true` 会跳过耗时 backing hash 对比，但任务仍会执行目标存储、同名 VM、网络选择等必要检查。
- 迁移接管会同步源用户的面板登录密码哈希、邮箱、邮箱验证状态和登录验证窗口；系统用户密码因无法反推明文，会在目标节点生成随机值。
- 目标无同名用户时不需要提交目标 VPC/安全组，目标面板会先创建用户并使用该用户默认网络；目标已有同名用户时，只能选择该用户下的 VPC/安全组。
- 链式克隆磁盘不会转换为独立盘；默认预检会校验目标同路径 backing 的 format、virtual size 和 sha256，`skip_precheck=true` 时不提前计算 sha256。
- 轻量云 VM 必须选择目标节点轻量云 VPC。
- 端口转发端口冲突时目标节点会自动分配新端口。

本机硬盘迁移说明：

- 运行中 VM 自动按热迁移执行，关机 VM 自动按冷迁移执行。
- 热迁移使用 libvirt blockcopy 并在 pivot 后同步持久化 XML。
- 冷迁移复制硬盘文件、修正链式 backing 路径并重写持久化 XML。
- 链式硬盘只迁移活动 overlay，backing 文件保持原路径。
- 成功后自动删除源硬盘文件；存在外部快照、光驱、空盘或块设备磁盘时拒绝迁移。

## 系统设置

### 获取公开系统设置

`GET /api/public/settings`

- 无需登录
- 当前返回字段：
  - `site_title`

### 获取系统运行环境信息

`GET /api/system-info`

- 需要登录（`access` 或 `bootstrap` Token / API Key）
- 返回字段：
  - `go_version` - Go 运行时版本
  - `os` - 操作系统类型（linux）
  - `distro` - Linux 发行版名称
  - `arch` - CPU 架构
  - `num_cpu` - CPU 核心数
  - `hostname` - 主机名
  - `num_goroutine` - 当前 goroutine 数量
  - `kernel` - 内核版本
  - `uptime` - 系统运行时长
  - `libvirt` - libvirt 版本
  - `qemu` - QEMU 版本

### 获取系统设置

`GET /api/settings`

- 管理员可使用 `access` 或 `bootstrap token`
- 除原有配置外，返回字段还包括：
  - `iso_dir`
  - `public_base_url`
  - `site_title`
  - `development_mode`
  - `smtp_host`
  - `smtp_port`
  - `smtp_username`
  - `smtp_from_name`
  - `smtp_from_address`
- `smtp_security`
- `smtp_timeout_seconds`
- `smtp_password_configured`
- `smtp_configured`
- `scheduler_event_retention_hours`

### 更新系统设置

`PUT /api/settings`

请求体字段示例：

```json
{
  "iso_dir": "/var/lib/libvirt/images/ISO",
  "public_base_url": "https://panel.example.com",
  "site_title": "QVMConsole",
  "development_mode": true,
  "smtp_host": "smtp.example.com",
  "smtp_port": 587,
  "smtp_username": "no-reply@example.com",
  "smtp_password": "smtp-password-or-app-code",
  "smtp_from_name": "QVMConsole",
  "smtp_from_address": "no-reply@example.com",
  "smtp_security": "starttls",
  "smtp_timeout_seconds": 15,
  "scheduler_event_retention_hours": 168
}
```

说明：

- `public_base_url` 支持填写 `域名:端口` 或完整 `http/https` 地址
- `development_mode=true` 时，会绕过登录二段验证、首次强制绑定和高风险操作验证
- 不会回显明文 SMTP 密码
- `smtp_password` 为空时表示保持现有密码不变
- `scheduler_event_retention_hours` 用于控制调度事件历史保留时间

## 调度事件中心

### 获取调度器概览

`GET /api/scheduler/list`

- 需要管理员权限
- 返回已注册调度器概览，字段包括：
  - `key`
  - `name`
  - `group`
  - `enabled`
  - `description`
  - `last_event_at`

### 获取调度事件列表

`GET /api/scheduler/events`

- 需要管理员权限
- 支持参数：
  - `page`
  - `page_size`
  - `scheduler_key`
  - `status`
  - `vm_name`
  - `start`
  - `end`

### 调度事件 SSE

`GET /api/scheduler/events/sse`

- 需要管理员权限
- SSE 事件名为 `scheduler_event`

### 测试 SMTP

`POST /api/settings/smtp/test`

```json
{
  "email": "admin@example.com"
}
```

## 宿主机设置

### 获取 KSM 状态

`GET /api/host/ksm`

- 需要管理员权限
- 返回宿主机 KSM 支持状态、当前运行参数、持久化挡位、统计指标与可选挡位列表
- 挡位包括 `off / conservative / balanced / aggressive / extreme`

### 设置 KSM 挡位

`PUT /api/host/ksm`

```json
{
  "profile": "balanced"
}
```

- 需要管理员权限
- 属于高风险操作，需要二次验证
- 会立即写入 `/sys/kernel/mm/ksm/*`
- 会写入 `/etc/kvm-console/ksm.env` 与 `kvm-console-ksm.service`，用于宿主机重启后恢复

### 获取 zRAM 状态

`GET /api/host/zram`

- 需要管理员权限
- 返回宿主机 zRAM 支持状态、当前设备、容量、已用量、压缩算法、swap 优先级、持久化挡位与可选挡位列表
- 挡位包括 `off / conservative / balanced / aggressive / extreme`

### 设置 zRAM 挡位

`PUT /api/host/zram`

```json
{
  "profile": "balanced"
}
```

- 需要管理员权限
- 属于高风险操作，需要二次验证
- 会立即重建面板管理的 `kvm-zram` 标签 zRAM swap
- 会写入 `/etc/kvm-console/zram.env` 与 `kvm-console-zram.service`，服务通过面板二进制的 `host-zram-apply` 内部子命令在宿主机重启后恢复

## 高风险保护覆盖范围

当前已接入高风险验证的主要操作包括：

- 创建虚拟机
- 删除虚拟机
- 封禁/解封用户
- 重置虚拟机密码
- 修改个人账户密码
- 删除快照、模板、存储池
- 删除用户
- 删除用户存储文件
- 清理已完成任务
- 删除磁盘文件（仅 `delete_file=true` 的物理删除场景）
- 迁移虚拟机硬盘
- 应用、禁用、回滚 KVM 网络防火墙规则
- 修改宿主机 KSM 挡位
- 修改宿主机 zRAM 挡位

## 其他核心接口

以下业务接口保持原有路径和调用方式不变：

- 虚拟机管理：`/api/vm/*`
- 模板管理：`/api/template/*`
- 快照：`/api/vm/:name/snapshot*`，创建快照请求支持 `description`、`include_memory`、`pause_for_memory_snapshot` 和可选 `name`；未传 `name` 时系统自动生成。显式传入的快照名称只能使用英文、数字、下划线、点和短横线，且最长 `64` 个字符；运行中 UEFI pflash 老 VM 若需要转换 NVRAM，会返回 `409` 和 `require_nvram_fix=true`，调用方确认后可带 `auto_fix_nvram=true` 重新提交。运行中创建包含内存的快照时，`pause_for_memory_snapshot` 默认为 `true`，表示先暂停虚拟机、快照写入完成后恢复运行；传 `false` 则不主动暂停，实际停顿和一致性取决于 libvirt/QEMU。`DELETE /api/vm/:name/snapshots` 会提交删除全部快照任务，并复用 `delete_snapshot` 二次验证；如果历史内部快照已不在当前活动磁盘链，清空任务会仅删除对应 libvirt 元数据以重建干净快照树，并清理不再被当前 VM 或剩余快照引用的 `.snap_*` / `.snap_restore_*` 残留文件。运行中 VM 挂载 9p/VirtFS 时不支持包含内存状态的内部快照，后端会返回中文提示。
- 网络管理：`/api/network/*`
- KVM 网络防火墙：`/api/firewall/*`
- 存储池：`/api/storage-pool/*`
- 任务中心：`/api/task/*`

如需了解安全流程与界面行为，请同时参考 [security-2fa-email.md](./security-2fa-email.md)。

## 模板链路删除接口

`DELETE /api/template/:name` 支持 `delete_mode`：

- `cascade`：默认模式，删除当前节点及其子节点，可按确认范围联动删除关联 VM。
- `promote_children`：仅删除当前节点，直接子模板和当前节点直接关联 VM 通过安全 `qemu-img rebase` 继承到上级模板，所有相关 VM 必须先关机。
- `promote_children_hot`：热删除当前节点，运行中的 VM 通过 `virsh blockpull` / `virsh blockcopy --pivot` 在线处理，失败时保留当前节点。

提升模式请求示例：

```json
{
  "delete_mode": "promote_children_hot",
  "delete_vms": false,
  "expected_vms": ["demo-vm-01"]
}
```

## 模板克隆接口

### 提交模板克隆任务

`POST /api/vm/clone`

说明：

- 支持 JWT 与 API Key；普通用户自助接口为 `POST /api/self/vm/clone`
- `clone_mode` 可选：`linked`（链式克隆，默认）/ `full`（完整克隆，脱离链式条件）
- `template_type=fnos` 时可传 `preserve_fnos_device_id` 或 `fnos_device_id`
- `preserve_fnos_device_id=false` 或不传：保持原有 FnOS 初始化行为，首次启动后生成新的系统标识与设备 ID
- `preserve_fnos_device_id=true`：FnOS 离线初始化账号、密码、主机名时保留模板内 `/etc/machine-id`、`/usr/trim/etc/machine_id` 和 `/etc/device_id`，用于保证链式克隆后的飞牛设备 ID 与模板一致
- `fnos_device_id`：可填写 `32` 位或 `40` 位十六进制 ID，后端会写入 `/etc/device_id` 和 `/usr/trim/etc/machine_id`；传入该字段时会自动按保留设备 ID 处理

请求体片段：

```json
{
  "name": "fnos-demo",
  "template": "FnOS",
  "template_type": "fnos",
  "clone_mode": "linked",
  "preserve_fnos_device_id": true,
  "fnos_device_id": "679ca7cf8fe242c4a64141a25a68f677"
}
```

## 重装系统接口

### 提交重装系统任务

`POST /api/vm/:name/reinstall`

说明：

- 支持 JWT 与 API Key
- 属于高风险操作，即使使用 API Key 也必须先完成二次验证，并在原请求头中携带 `X-High-Risk-Token`
- 仅支持弹性云 VM，轻量云场景不开放
- 提交后会自动删除该 VM 的全部快照，再强制关闭虚拟机并替换第一块系统盘
- CPU、内存、网络、VPC、安全组、备注、配额、额外数据盘和原有磁盘存储位置都会保留
- `disk_size` 不传时默认使用当前系统盘大小；如果当前系统盘小于模板原始磁盘大小，会自动提升到模板最小值
- 如果显式传入的 `disk_size` 小于模板原始磁盘大小，后端会自动提升到模板最小值
- 允许把系统盘从“当前更大”缩小到“仍不小于模板原始磁盘”的容量
- 会按模板类型重新执行初始化：
  - `linux`：离线重置 machine-id 与网络身份，开机后继续执行 SSH 初始化
  - `windows`：离线扩容并重新注入 `unattend.xml`，复用首次冷重启策略
  - `fnos`：离线扩容并重新写入首次管理员信息、主机名和设备 ID
- 若所选模板与当前 VM 的启动族不一致（BIOS/UEFI），接口会直接返回中文错误，不会开始重装
- 同一台 VM 同时只允许一个等待中或执行中的重装任务

请求体示例：

```json
{
  "template": "ubuntu2404-base",
  "disk_size": 80,
  "hostname": "vm-reinstall-demo",
  "user": "demo_user",
  "password": "StrongPass123!",
  "preserve_fnos_device_id": false,
  "fnos_device_id": ""
}
```

FnOS 示例：

```json
{
  "template": "FnOS",
  "disk_size": 200,
  "hostname": "fnos-reinstall",
  "user": "admin",
  "password": "StrongPass123!",
  "preserve_fnos_device_id": true,
  "fnos_device_id": "679ca7cf8fe242c4a64141a25a68f677"
}
```

## 原生链式克隆接口

### 提交原生链式克隆任务

`POST /api/vm/linked-clone`

说明：

- 仅管理员可调用
- 支持 JWT 与 API Key
- `clone_mode` 可选：`linked`（链式克隆，默认）/ `full`（完整克隆，脱离链式条件）
- 该接口默认基于模板生成 `qcow2` backing 链式磁盘并启动 VM；选择完整克隆则使用 `qemu-img convert` 创建独立磁盘
- 不执行 Linux SSH 初始化、Windows 应答文件注入、FnOS 首次账号写入等来宾初始化动作
- `disk_bus`、`video_model`、`cpu_topology_mode` 和 `first_boot_reboot_mode` 可选；为空时优先使用模板元数据中的 `default_config.disk_bus` / `default_config.video_model` / `default_config.cpu_topology_mode` / `default_config.first_boot_reboot_mode`，旧模板则回退为当前默认值
- `cpu_limit_percent` 可选，`0` 或不传表示无限制，`1-100` 表示按当前 `vCPU` 总能力限速

请求体示例：

```json
{
  "name": "demoalpha",
  "template": "ubuntu2404-base",
  "template_type": "linux",
  "clone_mode": "linked",
  "vcpu": 2,
  "ram": 4,
  "disk_size": 40,
  "disk_bus": "virtio",
  "switch_id": 3,
  "security_group_id": 2,
  "storage_pool_id": "disk-sdb1",
  "autostart": false,
  "freeze": false,
  "apic": true,
  "pae": true,
  "rtc_offset": "utc",
  "rtc_startdate": "now",
  "boot_type": "uefi",
  "nic_model": "virtio",
  "video_model": "virtio",
  "cpu_topology_mode": "auto",
  "cpu_limit_percent": 80,
  "first_boot_reboot_mode": "normal"
}
```

响应示例：

```json
{
  "code": 200,
  "message": "原生链式克隆任务已提交",
  "data": {
    "task_id": 15
  }
}
```

## 批量克隆接口

### 提交批量克隆任务

`POST /api/vm/batch-clone`

说明：

- 支持 JWT 与 API Key
- 基于指定模板批量创建多台虚拟机，所有虚拟机共享相同的硬件配置
- `prefix` 作为虚拟机名称前缀，最终命名为 `{prefix}-01`, `{prefix}-02`...
- `start_num` 为起始编号（默认 1），`count` 为创建数量
- `clone_mode` 可选：`linked`（链式克隆，默认）/ `full`（完整克隆）
- `hostname` 可选，为空则由系统自动生成
- `user` 可选（新建用户），为空则不创建，使用模板默认用户
- `disk_bus`、`video_model`、`cpu_topology_mode`、`first_boot_reboot_mode` 可选
- 通过灰度发布中间件保护，维护模式下不可用
- 任务提交后返回 `task_id`，可在任务中心查看进度
- 执行结果每个条目若失败会在 `error` 字段显示失败原因

请求体示例：

```json
{
  "prefix": "web",
  "start_num": 1,
  "count": 5,
  "template": "ubuntu2404-base",
  "template_type": "linux",
  "clone_mode": "linked",
  "vcpu": 2,
  "ram": 4,
  "disk_size": 40,
  "hostname": "web",
  "user": "admin",
  "password": "StrongPass123!",
  "freeze": false,
  "uefi": false,
  "template_root_pass": "",
  "template_user": "ubuntu",
  "video_model": "virtio",
  "disk_bus": "virtio",
  "cpu_topology_mode": "auto",
  "first_boot_reboot_mode": "normal"
}
```

响应示例：

```json
{
  "code": 200,
  "message": "批量克隆任务已提交",
  "data": {
    "task_id": 16
  }
}
```

## 虚拟机 XML 接口

### 获取持久化 XML

`GET /api/vm/:name/xml`

说明：

- 返回 `virsh dumpxml --inactive` 对应的持久化 domain XML
- 仅用于读取高级设置中的 XML 编辑器内容
- 仅管理员可调用

响应示例：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "xml": "<domain type='kvm'>...</domain>"
  }
}
```

### 保存持久化 XML

`PUT /api/vm/:name/xml`

请求体：

```json
{
  "xml": "<domain type='kvm'>...</domain>"
}
```

说明：

- 属于高风险操作，调用前需要完成二次验证
- 仅管理员可调用
- 后端会校验 XML 基本结构是否合法
- 不支持通过该接口修改虚拟机名称
- 保存时调用 `virsh define` 更新持久化配置
- 运行中的虚拟机通常需要关机再启动后生效

## 硬件直通 (PCI Passthrough)

### 获取可直通设备列表

`GET /api/host/passthrough`

说明：

- 返回宿主机上所有可直通的 PCI 设备列表
- 自动过滤系统关键设备（Host Bridge、ISA Bridge、RAID 控制器等）
- 标记设备是否已被其他虚拟机使用、是否已绑定 vfio-pci 等

响应示例：

```json
{
  "code": 200,
  "message": "ok",
  "data": [
    {
      "pci_address": "0000:04:00.0",
      "domain": "0000",
      "bus": "04",
      "slot": "00",
      "function": "0",
      "vendor_id": "10de",
      "vendor_name": "NVIDIA Corporation",
      "product_id": "1bb3",
      "product_name": "GP104GL [Tesla P4]",
      "class_name": "3D 控制器 / GPU",
      "iommu_group": 21,
      "driver_in_use": "vfio-pci",
      "is_vfio_bound": true,
      "is_used_by_vm": false,
      "used_by_vm_name": ""
    }
  ]
}
```

### 绑定设备到 vfio-pci

`POST /api/host/passthrough/bind`

仅管理员可调用。

请求体：

```json
{
  "pci_address": "0000:04:00.0"
}
```

### 从 vfio-pci 解绑设备

`POST /api/host/passthrough/unbind`

仅管理员可调用。

请求体：

```json
{
  "pci_address": "0000:04:00.0"
}
```

### 获取虚拟机的直通设备

`GET /api/vm/:name/passthrough`

响应示例：

```json
{
  "code": 200,
  "message": "ok",
  "data": [
    {
      "pci_address": "0000:04:00.0",
      "domain": "0000",
      "bus": "04",
      "slot": "00",
      "function": "0"
    }
  ]
}
```

### 添加直通设备到虚拟机

`POST /api/vm/:name/passthrough`

仅管理员可调用。虚拟机必须处于关机状态。

请求体：

```json
{
  "pci_address": "0000:04:00.0"
}
```

### 从虚拟机移除直通设备

`DELETE /api/vm/:name/passthrough`

仅管理员可调用。虚拟机必须处于关机状态。

请求体：

```json
{
  "pci_address": "0000:04:00.0"
}
```

### 创建/编辑虚拟机时配置直通设备

创建虚拟机 (`POST /api/vm/create`) 和编辑虚拟机 (`PUT /api/vm/:name`) 接口均支持在请求体中传入 `host_devices` 字段：

```json
{
  "host_devices": [
    { "pci_address": "0000:04:00.0" }
  ]
}
```

创建时会自动验证 IOMMU、绑定 vfio-pci 并将设备添加到虚拟机 XML 中。编辑时会完全替换当前直通设备列表。
