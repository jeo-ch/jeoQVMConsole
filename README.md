# QVMConsole - 开源虚拟机管理控制台

<div align="center">

<img width="2403" height="1257" alt="sudbsi" src="https://github.com/user-attachments/assets/965011d7-9cf3-4ef4-b39e-7b22fe99a1c8" />

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![GitHub Stars](https://img.shields.io/github/stars/qvmconsole/qvmconsole?style=social)](https://github.com/qvmconsole/qvmconsole)
[![GitHub Forks](https://img.shields.io/github/forks/qvmconsole/qvmconsole?style=social)](https://github.com/qvmconsole/qvmconsole)
[![GitHub Issues](https://img.shields.io/github/issues/qvmconsole/qvmconsole)](https://github.com/qvmconsole/qvmconsole/issues)
[![GitHub Pull Requests](https://img.shields.io/github/issues-pr/qvmconsole/qvmconsole)](https://github.com/qvmconsole/qvmconsole/pulls)

[**官方网站**](https://www.qvmconsole.cn/) | [**文档站点**](https://qvmcdocs.xiaozhuhouses.asia/) | [**部署指南**](https://qvmcdocs.xiaozhuhouses.asia/docs/install/)

</div>

## 项目简介

QVMConsole 是一个面向小型企业和个人私有云服务场景的开源虚拟机管理平台，基于 KVM/QEMU 虚拟化技术深度集成，提供从虚拟机生命周期管理、网络与存储编排、快照与克隆、防火墙与带宽治理，到 Web 控制台与 API 一体化交付的完整解决方案。

### 核心价值

- **降低运维门槛**：提供"即开即用"的虚拟化管理平台，减少重复造轮子的成本
- **模块化设计**：可插拔网络后端（如 Open vSwitch），适配多样化的网络拓扑与安全策略
- **双入口架构**：Web 控制台与 RESTful API 兼顾自动化与人工运维效率
- **可观测性**：任务队列与 SSE 机制实现长耗时操作的可观测与可中断，保障大规模并发下的稳定性

## 核心功能

### 虚拟机生命周期管理
- 完整的电源操作（开机/关机/重启/强制断电/重置）
- 配额控制与权限校验
- 维护模式与优雅关机
- 动态内存配置与调整

### 网络虚拟化
- VPC 逻辑交换机与安全组
- 端口转发与静态 IP 管理
- 防火墙策略（VM/宿主机双层）
- 网络诊断与抓包工具

### 存储管理
- 宿主机存储池管理（格式化/分区/LVM 卷）
- 模板管理（制作/导入/导出/删除）
- 磁盘管理与 IOPS 限制
- 用户 ISO 挂载

### 用户权限与配额
- 多租户支持（弹性云/轻量云）
- 细粒度配额管理（CPU/内存/磁盘/VM 数/存储/带宽/流量/公网 IP/端口转发/快照）
- SSH 访问控制与邀请注册流程

### 监控与任务调度
- VM/宿主机统计与历史数据
- 异步任务队列与 SSE 实时推送
- 定时事件中心与资源回收

### 快照备份
- 创建/恢复/删除/批量删除快照
- NVRAM 与共享目录兼容性检查
- 配额校验与任务跟踪

## 技术栈

### 后端
- **语言**: Go 1.25.4
- **Web 框架**: Gin v1.12.0
- **数据库**: SQLite + GORM v1.31.1
- **虚拟化**: go-libvirt RPC
- **认证**: JWT v5.3.1
- **日志**: lumberjack v2.2.1

### 前端
- **框架**: Vue 3.5.30
- **UI 库**: Element Plus v2.13.5
- **HTTP 客户端**: Axios v1.15.2
- **VNC 客户端**: @novnc/novnc v1.7.0
- **构建工具**: Vite v8.0.0

### 虚拟化基础设施
- **虚拟化平台**: KVM/QEMU
- **网络虚拟化**: Open vSwitch
- **Windows 初始化**: ConfigDrive 标准支持

## 系统要求

### 硬件要求
- 支持 VT-x/AMD-V 的 CPU
- 至少 4GB RAM（推荐 8GB+）
- 至少 50GB 可用磁盘空间

### 软件要求
- **操作系统**: Debian/Ubuntu（推荐 Debian 12+）
- **虚拟化**: KVM/QEMU
- **网络**: Open vSwitch
- **依赖工具**: genisoimage（用于 Windows 虚拟机初始化）

## 项目状态与参与方式

### 内测阶段说明
当前项目正处于内测阶段，仓库仅提供源码供具备开发能力的用户本地编译和测试使用。在正式公测前，暂不提供预编译的二进制文件。若您希望参与内测并直接本地部署，欢迎加入我们的QQ群：654641487。

### 开发贡献指南
作为一个由独立开发者维护的大型开源项目，QVMConsole 需要社区贡献者的支持才能持续完善。我们欢迎并鼓励您使用 AI 等工具进行功能修复与开发，但请务必遵守以下准则：

1. **规则遵守**：在使用 AI 工具时，必须将根目录的 `AGENTS.md` 文件作为核心提示词规则
2. **功能边界**：开源版本中不得提交包含 Pro 版功能的代码。Pro 版功能清单详见：[赞助功能说明](https://qvmcdocs.xiaozhuhouses.asia/docs/install/sponsorship)
3. **场景通用性**：提交的功能应面向通用化使用场景，符合广大用户的需求。针对特定场景的定制功能建议自行 fork 仓库维护

### 安全漏洞报告
如果您发现项目存在安全漏洞，无论严重程度如何，请勿在 GitHub Issues 中公开报告，以避免安全风险被恶意利用。

**安全报告渠道**：
- 作者QQ：3354416548
- 电子邮件：xiaozhuhs@foxmail.com

---

## 致谢

感谢所有为 QVMConsole 做出贡献的开发者！

---

<div align="center">

**QVMConsole** - 让虚拟化管理更简单

[官方网站](https://www.qvmconsole.cn/) | [文档站点](https://qvmcdocs.xiaozhuhouses.asia/) | [部署指南](https://qvmcdocs.xiaozhuhouses.asia/docs/install/)

</div>
