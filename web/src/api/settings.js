import request from '@/utils/request'

function withStageToken(token) {
  return token ? { Authorization: `Bearer ${token}` } : {}
}

// 获取公开系统设置
export function getPublicSettings() {
  return request({
    url: '/public/settings',
    method: 'get'
  })
}

// 获取系统版本信息
export function getPublicVersion() {
  return request({
    url: '/public/version',
    method: 'get'
  })
}

// 获取公开的系统运行环境信息
export function getPublicSystemInfo() {
  return request({
    url: '/public/system-info',
    method: 'get'
  })
}

// 获取系统设置
export function getSettings(token = '') {
  return request({
    url: '/settings',
    method: 'get',
    headers: withStageToken(token)
  })
}

// 更新系统设置
export function updateSettings(data, token = '') {
  return request({
    url: '/settings',
    method: 'put',
    data,
    headers: withStageToken(token)
  })
}

// 测试 SMTP 发信
export function testSMTP(data, token = '') {
  return request({
    url: '/settings/smtp/test',
    method: 'post',
    data,
    headers: withStageToken(token)
  })
}

// 手动轮换 JWT 密钥
export function rotateJWTSecret(data, token = '') {
  return request({
    url: '/settings/jwt-secret/rotate',
    method: 'post',
    data,
    headers: withStageToken(token)
  })
}

// 获取宿主机 Intel KVM unrestricted_guest 状态
export function getHostKVMUnrestrictedGuestStatus() {
  return request({
    url: '/host/kvm-intel-unrestricted-guest',
    method: 'get'
  })
}

// 设置宿主机 Intel KVM unrestricted_guest
export function updateHostKVMUnrestrictedGuest(data) {
  return request({
    url: '/host/kvm-intel-unrestricted-guest',
    method: 'put',
    data,
    timeout: 30000
  })
}

// 获取宿主机 KSM 状态
export function getHostKSMStatus() {
  return request({
    url: '/host/ksm',
    method: 'get'
  })
}

// 设置宿主机 KSM 挡位
export function updateHostKSMProfile(data) {
  return request({
    url: '/host/ksm',
    method: 'put',
    data,
    timeout: 30000
  })
}

// 获取宿主机 zRAM 状态
export function getHostZRAMStatus() {
  return request({
    url: '/host/zram',
    method: 'get'
  })
}

// 设置宿主机 zRAM 挡位
export function updateHostZRAMProfile(data) {
  return request({
    url: '/host/zram',
    method: 'put',
    data,
    timeout: 30000
  })
}

// 获取 CPU 亲和性预设列表
export function getCPUAffinityPresets() {
  return request({
    url: '/cpu-affinity-presets',
    method: 'get'
  })
}

// 保存 CPU 亲和性预设列表（管理员）
export function saveCPUAffinityPresets(data) {
  return request({
    url: '/settings/cpu-affinity-presets',
    method: 'put',
    data
  })
}
