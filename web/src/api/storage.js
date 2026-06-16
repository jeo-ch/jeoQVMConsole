import request from '@/utils/request'

// ==================== 用户存储池 API ====================

// 获取存储池信息
export function getStorageInfo() {
  return request({
    url: '/self/storage/info',
    method: 'get'
  })
}

// 初始化存储池
export function initStorage() {
  return request({
    url: '/self/storage/init',
    method: 'post'
  })
}

// 获取文件列表
export function getStorageFiles(category) {
  return request({
    url: `/self/storage/files/${category}`,
    method: 'get'
  })
}

// 上传文件
export function uploadStorageFile(category, formData, onProgress) {
  return request({
    url: `/self/storage/upload/${category}`,
    method: 'post',
    data: formData,
    timeout: 0, // 大文件上传不超时
    maxContentLength: Infinity, // 不限制请求体大小
    maxBodyLength: Infinity,    // 不限制请求体大小
    onUploadProgress: onProgress
  })
}

// 大文件上传检测：查询服务器是否因 /tmp 空间不足启用了落盘模式
export function checkLargeUpload(fileSize) {
  return request({
    url: '/self/storage/upload-check',
    method: 'get',
    params: { size: fileSize }
  })
}

// 删除文件
export function deleteStorageFile(category, filename) {
  return request({
    url: `/self/storage/file/${category}/${encodeURIComponent(filename)}`,
    method: 'delete'
  })
}

// 下载文件
export function getStorageDownloadUrl(category, filename) {
  const token = localStorage.getItem('token')
  const baseUrl = import.meta.env.VITE_API_BASE || '/api'
  return `${baseUrl}/self/storage/download/${category}/${encodeURIComponent(filename)}?token=${token}`
}

// 获取用户ISO列表（VM创建用）
export function getUserISOs() {
  return request({
    url: '/self/storage/isos',
    method: 'get'
  })
}

// 获取用户所有VM的挂载列表
export function getUserMounts() {
  return request({
    url: '/self/storage/mounts',
    method: 'get'
  })
}

// 挂载存储池到VM
export function mountStorage(data) {
  return request({
    url: '/self/storage/mount',
    method: 'post',
    data
  })
}

// 卸载存储池
export function unmountStorage(vmName, tag) {
  return request({
    url: `/self/storage/mount/${vmName}/${tag}`,
    method: 'delete'
  })
}

// 用户自助创建VM
export function selfCreateVm(data) {
  return request({
    url: '/self/vm/create',
    method: 'post',
    data
  })
}

// 导出虚拟机
export function exportVM(data) {
  return request({
    url: '/self/vm/export',
    method: 'post',
    data
  })
}

// 导入虚拟机
export function importVM(data) {
  return request({
    url: '/self/vm/import',
    method: 'post',
    data
  })
}

// 上传磁盘文件
export function uploadDiskFile(formData, onProgress) {
  return request({
    url: '/self/storage/upload/disk',
    method: 'post',
    data: formData,
    timeout: 0, // 大文件上传不超时
    maxContentLength: Infinity, // 不限制请求体大小
    maxBodyLength: Infinity,    // 不限制请求体大小
    onUploadProgress: onProgress
  })
}
