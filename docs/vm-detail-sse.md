# 虚拟机详情页 SSE 实时推送

## 功能概述

虚拟机详情页面使用 Server-Sent Events (SSE) 实时推送所有内容，包括基本信息和实时监控数据，替代了原有的 HTTP 请求轮询方式。页面进入时自动建立 SSE 连接，服务端每 3 秒推送一次虚拟机最新详情数据（含 stats），离开页面时自动断开连接。

**整个详情页面零 XHR 轮询请求**，所有数据均通过单一 SSE 连接获取。

## 技术架构

```
前端 (detail.vue)                    后端 (vm.go)
     │                                    │
     │  EventSource 连接                   │
     ├──────────────────► GET /api/vm/:name/sse
     │                                    │
     │  event: vm_detail                  │
     │  data: { VmDetail + Stats }        │
     ◄────────────────── 每3秒推送一次 ────┤
     │        │                           │
     │    vmInfo.stats                     │
     │        ↓                            │
     │  ResourceCharts                     │
     │  (externalStats prop)               │
     │                                    │
     │  页面离开 / 连接断开                  │
     ├──── close() ──────►  goroutine 退出 │
```

## 后端接口

### `GET /api/vm/:name/sse`

- **认证方式**：通过 URL 查询参数 `token` 传递 JWT Token
- **响应类型**：`text/event-stream`
- **事件名称**：`vm_detail`
- **推送间隔**：3 秒
- **数据格式**：`VmDetail` 结构体，包含 `stats` 字段（从后台采集缓存读取，不阻塞）

### 请求示例

```
GET /api/vm/my-vm/sse?token=<JWT_TOKEN>
```

### 响应数据结构

```json
{
  "name": "my-vm",
  "status": "running",
  "vcpu": 4,
  "memory": 4096,
  "ip": "192.168.1.100",
  "stats": {
    "cpu_percent": 25.3,
    "mem_used": 2048000,
    "mem_total": 4096000,
    "net_rx_bytes": 12345678,
    "net_tx_bytes": 87654321,
    "disk_rd_bytes": 111222333,
    "disk_wr_bytes": 444555666
  }
}
```

## 前端实现

### 连接管理（detail.vue）

- **建立连接**：页面挂载时（`onMounted`）调用 `initSSE()` 建立 SSE 连接
- **关闭连接**：页面卸载时（`onUnmounted`）调用 `closeSSE()` 关闭连接
- **自动重连**：SSE 连接出错 5 秒后自动重试
- **VM 切换**：通过侧边栏切换虚拟机时，自动断开旧连接并建立新连接

### 监控图表（ResourceCharts.vue）

新增 `externalStats` prop：
- **传入时**：禁用自行 XHR 轮询（`setInterval`），通过 `watch(externalStats)` 接收外部 SSE 推送的 stats 数据
- **不传时**：保持原有行为，自行每 5 秒轮询一次（兼容宿主机监控等其他场景）

### 交互优化

1. **首次加载**：首次进入页面显示 loading 状态，SSE 收到第一条数据后自动隐藏
2. **操作反馈**：开关机等操作下发后，按钮进入 loading 状态，SSE 检测到状态变化后自动解除
3. **编辑配置**：编辑虚拟机配置成功后，SSE 会自动推送最新配置，无需手动刷新
4. **成功提示去重**：编辑弹窗自身负责展示一次成功提示，详情页仅静默重连 SSE 立即拉取最新数据，避免保存后连续弹出两条成功通知

## 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `server/handler/vm.go` | 新增 `GetVmDetailSSE` handler |
| `server/service/libvirt.go` | `VmDetail` 增加 `Stats` 字段，`GetVM` 自动填充缓存 stats |
| `server/router/router.go` | 注册 `/:name/sse` 路由 |
| `web/src/api/vm.js` | 新增 `createVmDetailSSE` 函数 |
| `web/src/views/vm/detail.vue` | 重写数据获取逻辑为 SSE，传递 stats 给 ResourceCharts；页面区域懒加载（监控图表、信息卡片区按需渲染） |
| `web/src/components/ResourceCharts.vue` | 新增 `externalStats` prop，支持外部数据驱动 |

## 页面区域懒加载

详情页采用分区域按需加载策略，避免低性能设备一次性渲染全部内容造成卡顿：

| 区域 | 加载时机 | 骨架屏 |
|------|---------|--------|
| ① Hero 状态横幅 | SSE 首条数据到达后立即渲染 | 页面级 `el-skeleton` |
| ② 实时资源概览条 | 同上 | 无（依赖 Hero 数据） |
| ③ 功能管理 Tabs | 同上（tab 内组件使用 `lazy` 属性） | 无 |
| ④ 监控图表区 | 滚动至视口 200px 以内 或 点击快速导航"监控" | `el-skeleton` 占位 |
| ⑤ 信息卡片区 | 滚动至视口 200px 以内 或 点击快速导航"配置" | 卡片级 `el-skeleton` 占位 |
| 磁盘 IOPS 数据 | 信息卡片区进入视口时异步加载 | 卡片内 `v-loading` |

实现方式：`IntersectionObserver` + `rootMargin: 200px` 提前触发，确保用户滚动到位时内容已渲染完毕。快速导航点击时主动触发加载，避免等待计时。

## 与列表页 SSE 的对比

| 特性 | 列表页 SSE | 详情页 SSE |
|------|-----------|-----------|
| 路由 | `/api/vm/sse` | `/api/vm/:name/sse` |
| 事件名 | `vm_list` | `vm_detail` |
| 推送间隔 | 2 秒 | 3 秒 |
| 数据结构 | `[]VmInfo` | `VmDetail`（含 `Stats`） |
| 默认开启 | 手动开关 | 自动开启 |
| 包含 stats | ❌ | ✅ |
