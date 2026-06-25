# 开发环境依赖安装指南

本文档说明如何安装 QVMConsole 开发所需的所有依赖。

## 环境要求

| 依赖 | 最低版本 | 用途 |
|------|---------|------|
| Go | 1.22+ | 后端开发语言 |
| Node.js | 18+ | 前端构建工具链 |
| npm | 9+ | 前端包管理 |
| air | 1.61.7 | Go 热重载开发工具 |

## Windows 安装步骤

### 1. 安装 Go

从 [go.dev](https://go.dev/dl/) 下载 Windows 安装包并安装。

验证安装：

```powershell
go version
```

### 2. 安装 Node.js

从 [nodejs.org](https://nodejs.org/) 下载 LTS 版本并安装（npm 随 Node.js 一起安装）。

验证安装：

```powershell
node --version
npm --version
```

### 3. 安装 air（Go 热重载）

```powershell
go install github.com/air-verse/air@v1.61.7
```

验证安装：

```powershell
where air
```

> 确保 `%USERPROFILE%\go\bin` 在系统 PATH 环境变量中。

### 4. 安装前端依赖

```powershell
cd web
npm install
```

### 5. 下载 Go 模块依赖

```powershell
cd server
go mod download
```

## Linux 安装步骤

### 1. 安装 Go

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# 或手动安装（推荐，获取最新版本）
wget https://go.dev/dl/go1.23.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 2. 安装 Node.js

```bash
# 使用 NodeSource
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs
```

### 3. 安装 air

```bash
go install github.com/air-verse/air@v1.61.7
```

确保 `$(go env GOPATH)/bin` 在 PATH 中：

```bash
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### 4. 安装前端依赖

```bash
cd web
npm install
```

### 5. 下载 Go 模块依赖

```bash
cd server
go mod download
```

## macOS 安装步骤

### 1. 安装 Homebrew（如未安装）

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### 2. 安装 Go 和 Node.js

```bash
brew install go node
```

### 3. 安装 air

```bash
go install github.com/air-verse/air@v1.61.7
```

### 4. 安装前端依赖

```bash
cd web
npm install
```

### 5. 下载 Go 模块依赖

```bash
cd server
go mod download
```

## 一键安装脚本（Linux/macOS）

```bash
#!/bin/bash
set -e

echo "=== QVMConsole 开发环境依赖安装 ==="

# 安装 air
if ! command -v air &>/dev/null; then
    echo "安装 air..."
    go install github.com/air-verse/air@v1.61.7
else
    echo "air 已安装"
fi

# 前端依赖
echo "安装前端依赖..."
cd web && npm install
cd ..

# Go 模块
echo "下载 Go 模块..."
cd server && go mod download
cd ..

echo "=== 依赖安装完成 ==="
echo "启动开发服务器: ./start-dev.sh"
```

## 验证安装

全部安装完成后，执行以下命令确认环境：

```bash
go version          # 应显示 go1.22+
node --version      # 应显示 v18+
npm --version       # 应显示 9+
air -v              # 应显示 air v1.61.7
ls web/node_modules # 前端依赖目录应存在
```

## 注意事项

- Windows 下运行 `air` 不支持的参数 `--send_interrupt` 会被自动忽略
- 首次 `npm install` 可能需要较长时间，后续有缓存会快很多
- Go 模块会缓存到 `$GOPATH/pkg/mod`，后续项目共享
- 测试机（Linux）系统级 QEMU/KVM 依赖由 `install.sh` 管理，详见该脚本

## Windows 虚拟机初始化依赖

### genisoimage

**用途：** 为 Windows 虚拟机创建符合 OpenStack ConfigDrive 规范的 ISO 镜像（CloudbaseInit 初始化方案）

**安装方式（Debian/Ubuntu）：**

```bash
sudo apt install -y genisoimage
```

**说明：** 此工具在 Windows 克隆/重装时被调用，用于生成包含 `meta_data.json`（主机名、管理员密码、instance-id）的 config-2 标签 ISO，挂载到虚拟机 CD-ROM 后由 CloudbaseInit 读取完成初始化。该工具已由 `install.sh` 自动安装。

## 泄露密码检测服务

### Have I Been Pwned (HIBP) Pwned Passwords API

**用途：** 检查用户设置的密码是否在已知数据泄露事件中被暴露（110亿+条记录）

**调用方式：** 后端通过 HTTPS 调用 `https://api.pwnedpasswords.com/range/` API，采用 k-匿名性模型——仅发送密码 SHA-1 哈希的前 5 位字符，后缀在本地比对，**密码本身永不离开本机**

**网络要求：** 需要外网访问能力。当网络不可用时自动回退到内置常见弱密码列表（约 500 条高频弱密码）

**开关：** 系统设置 → 安全防护 → 泄露密码检测（默认开启，关闭后跳过所有密码校验）

**缓存：** API 响应结果在内存中缓存 30 分钟，避免重复请求

## SPICE 显示协议

### QEMU SPICE 服务端

**用途：** 为虚拟机提供 SPICE 显示协议（与 VNC 共存），供使用 virt-viewer / spicy 等外部客户端的用户连接。面板默认 VNC；SPICE 面向偏好高保真/本地化体验的用户。

**宿主依赖：** QEMU SPICE 服务端随 `qemu-system-x86` 自带（Debian/Ubuntu 默认包含 `libspice-server1`），通常无需额外安装。若编译自定义 QEMU，需启用 `--enable-spice`。建议同时具备 QXL 显卡模型支持（默认随 QEMU 提供）。

**安装方式（Debian/Ubuntu，按需）：**

```bash
sudo apt install -y qemu-system-x86 libspice-server1
```

**说明：** 面板在创建/克隆/导入虚拟机时默认注入 SPICE graphics（默认监听 127.0.0.1）。用户在虚拟机详情 → VNC 控制台 tab 的 SPICE 面板可开启/关闭、设置密码、切换对外暴露（暴露时自动放行宿主防火墙对应端口）。对外暴露后可下载 `.vv` 连接文件，由客户端的 virt-viewer 打开直连。

### 客户端工具（用户侧，非宿主）

**virt-viewer / spicy：** 用户使用 virt-viewer（Linux：`apt install virt-viewer`；Windows：从 [virt-manager.org](https://virt-manager.org/download/) 下载）或 spicy 打开下载的 `.vv` 文件即可直连。面板不提供 SPICE 的浏览器内客户端——SPICE 仅面向原生客户端。

