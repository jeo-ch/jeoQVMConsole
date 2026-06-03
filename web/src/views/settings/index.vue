<template>
  <div class="settings-container">
    <el-card v-loading="loading">
      <div style="margin-bottom: 20px;">
        <h2>系统设置</h2>
        <el-text type="info">常规配置保存后立即生效并持久化到数据库（重启保留）。宿主机级兼容性选项会写入系统配置文件；若配置了环境变量，环境变量优先。</el-text>
      </div>

      <el-form :model="form" ref="formRef" label-width="180px" style="max-width: 700px;">

        <el-tabs v-model="activeTab" class="settings-tabs">
          <el-tab-pane label="基础设置" name="basic">
            <el-divider content-position="left">
              <el-icon style="margin-right: 4px;"><InfoFilled /></el-icon>
              站点展示
            </el-divider>

            <el-form-item label="网站标题">
              <el-input v-model="form.site_title" placeholder="请输入网站标题" maxlength="60" show-word-limit />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                将用于登录页标题、侧边栏名称和浏览器标签页标题 | 环境变量: KVM_SITE_TITLE
              </div>
            </el-form-item>

            <!-- 端口分配 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Connection /></el-icon>
          端口自动分配
        </el-divider>

        <el-form-item label="分配范围">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <el-input-number v-model="form.auto_port_start" :min="1024" :max="65535" style="flex: 1;" />
            <span style="color: #909399;">—</span>
            <el-input-number v-model="form.auto_port_end" :min="1024" :max="65535" style="flex: 1;" />
          </div>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            端口转发自动分配时使用此范围（当前: {{ form.auto_port_start }} - {{ form.auto_port_end }}，
            共 {{ (form.auto_port_end || 0) - (form.auto_port_start || 0) }} 个端口）
          </div>
          <div class="form-tip" style="margin-top: 2px;">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_AUTO_PORT_START / KVM_AUTO_PORT_END
          </div>
        </el-form-item>
            <el-form-item label="访问链接">
          <el-input v-model="form.public_base_url" placeholder="如 panel.example.com:8080 或 https://panel.example.com" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            邀请注册、找回密码等邮件里的跳转链接会优先使用这里 | 环境变量: KVM_PUBLIC_BASE_URL
          </div>
        </el-form-item>
            <!-- 服务信息 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><InfoFilled /></el-icon>
          服务信息
        </el-divider>

        <el-form-item label="服务端口">
          <el-input :model-value="form.port" disabled />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_PORT（重启后生效）
          </div>
        </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="存储与网络" name="network">
            <!-- 存储路径 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><FolderOpened /></el-icon>
          存储路径
        </el-divider>

        <el-form-item label="模板目录">
          <el-input v-model="form.template_dir" placeholder="/var/lib/libvirt/images/templates" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_TEMPLATE_DIR
          </div>
        </el-form-item>

        <el-form-item label="模板导入临时目录">
          <el-input v-model="form.template_import_dir" placeholder="/var/lib/libvirt/images/templates/_imports" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            建议与模板目录放在同一磁盘，避免导入大模板时占满 /tmp | 环境变量: KVM_TEMPLATE_IMPORT_DIR
          </div>
        </el-form-item>

        <el-form-item label="模板导出目录">
          <el-input v-model="form.template_export_dir" placeholder="/var/lib/libvirt/images/templates/_exports" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            建议与模板目录放在同一磁盘，避免导出大模板时占满 /tmp | 环境变量: KVM_TEMPLATE_EXPORT_DIR
          </div>
        </el-form-item>

        <el-form-item label="克隆磁盘目录">
          <el-input v-model="form.clone_dir" placeholder="/var/lib/libvirt/images" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_CLONE_DIR
          </div>
        </el-form-item>

        <el-form-item label="ISO 存放位置">
          <el-input v-model="form.iso_dir" placeholder="/var/lib/libvirt/images/ISO" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            创建虚拟机和救援系统下拉框都会读取这个目录下的 `.iso` 文件 | 环境变量: KVM_ISO_DIR
          </div>
        </el-form-item>

        <el-form-item label="端口转发持久化目录">
          <el-input v-model="form.port_forward_dir" disabled />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_PORTFORWARD_DIR（仅通过环境变量修改）
          </div>
        </el-form-item>
            <!-- 网络设置 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Connection /></el-icon>
          网络设置
        </el-divider>

        <el-form-item label="默认网络">
          <el-input v-model="form.default_network" placeholder="default" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            保留给历史配置查看；新平台默认使用 OVS | 环境变量: KVM_DEFAULT_NETWORK
          </div>
        </el-form-item>

        <el-form-item label="网络后端">
          <el-input v-model="form.network_backend" disabled />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            当前仅支持 OVS | 环境变量: KVM_NETWORK_BACKEND
          </div>
        </el-form-item>

        <el-form-item label="OVS 网桥">
          <el-input v-model="form.ovs_bridge" placeholder="br-ovs" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            VM 接入的 OVS 网桥，不迁移宿主机物理网卡 | 环境变量: KVM_OVS_BRIDGE
          </div>
        </el-form-item>

        <el-form-item label="OVS 出口网卡">
          <el-input v-model="form.ovs_uplink" placeholder="留空自动检测默认路由网卡" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            OVS NAT 出口网卡，留空自动检测 | 环境变量: KVM_OVS_UPLINK
          </div>
        </el-form-item>

        <el-form-item label="网段前缀">
          <el-input v-model="form.subnet_prefix" placeholder="192.168.122" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SUBNET_PREFIX
          </div>
        </el-form-item>

        <el-form-item label="OVS DHCP 范围">
          <div style="display: flex; align-items: center; gap: 10px; width: 100%;">
            <el-input v-model="form.ovs_dhcp_start" placeholder="192.168.122.2" />
            <span style="color: #909399;">—</span>
            <el-input v-model="form.ovs_dhcp_end" placeholder="192.168.122.254" />
          </div>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            留空时按网段前缀自动使用 .2 - .254 | 环境变量: KVM_OVS_DHCP_START / KVM_OVS_DHCP_END
          </div>
        </el-form-item>

        <el-form-item label="外网网卡">
          <el-input v-model="form.external_nic" placeholder="留空自动检测（如 eth0、ens33）" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            端口转发用的外网网卡名称，留空通过默认路由自动检测 | 环境变量: KVM_EXTERNAL_NIC
          </div>
        </el-form-item>

        <el-form-item label="公网 IP">
          <el-input v-model="form.host_ip" placeholder="留空自动检测，也可手动填写固定公网 IP" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            端口转发展示和规则优先使用这里的公网 IP，留空时自动检测默认出口 IP | 环境变量: KVM_HOST_IP
          </div>
        </el-form-item>
            <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Odometer /></el-icon>
          全局带宽限制
        </el-divider>

        <el-alert
          title="全局带宽限制会应用于所有非轻量云的虚拟机及VPC交换机。有效带宽 = 配置值 - 5Mbps（保留缓冲），所有运行中的虚拟机均分总带宽。0 = 不限制。"
          type="info"
          :closable="false"
          style="margin-bottom: 18px;"
        />

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="下行总带宽(Mbps)">
              <el-input-number v-model="form.max_burst_inbound" :min="0" :max="100000" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                全局限速下行总带宽，所有VM均分 | 环境变量: KVM_MAX_BURST_INBOUND
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="上行总带宽(Mbps)">
              <el-input-number v-model="form.max_burst_outbound" :min="0" :max="100000" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                全局限速上行总带宽，所有VM均分 | 环境变量: KVM_MAX_BURST_OUTBOUND
              </div>
            </el-form-item>
          </el-col>
        </el-row>
        <div class="form-tip" style="margin-bottom: 16px;">
          <el-icon><InfoFilled /></el-icon>
          保存后立即生效：每台运行中VM设置全量有效带宽为上限（配置50Mbps时有效45Mbps，每台VM上限均为45Mbps）。多台VM同时跑满时由TCP拥塞控制自然分享带宽。
        </div>

        <!-- 默认磁盘 IOPS 限制 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Operation /></el-icon>
          默认磁盘 IOPS 限制
        </el-divider>
        <el-alert
          title="此设置仅作为新建虚拟机时的参考默认值。已存在的虚拟机需在编辑页面中单独配置磁盘 IOPS 限制。0 表示不限制。"
          type="info"
          :closable="false"
          style="margin-bottom: 18px;"
        />
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="默认总 IOPS">
              <el-input-number v-model="form.default_disk_iops_total" :min="0" :step="100" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                新建虚拟机磁盘的默认总 IOPS 限制 | 环境变量: KVM_DEFAULT_DISK_IOPS_TOTAL
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="默认读 IOPS">
              <el-input-number v-model="form.default_disk_iops_read" :min="0" :step="100" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                新建虚拟机磁盘的默认读 IOPS 限制 | 环境变量: KVM_DEFAULT_DISK_IOPS_READ
              </div>
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="默认写 IOPS">
              <el-input-number v-model="form.default_disk_iops_write" :min="0" :step="100" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                新建虚拟机磁盘的默认写 IOPS 限制 | 环境变量: KVM_DEFAULT_DISK_IOPS_WRITE
              </div>
            </el-form-item>
          </el-col>
        </el-row>
          </el-tab-pane>

          <el-tab-pane label="宿主机设置" name="host">
            <el-divider content-position="left">
              <el-icon style="margin-right: 4px;"><Cpu /></el-icon>
              KSM 内存去重
            </el-divider>

            <el-alert
              title="KSM 是宿主机级内存页去重能力，会影响当前宿主机上的所有虚拟机。挡位越高，扫描越积极，CPU 开销也越明显。"
              type="info"
              :closable="false"
              style="margin-bottom: 18px;"
            />

            <el-form-item label="KSM 挡位">
              <div class="host-setting-field">
                <div class="host-setting-row">
                  <el-radio-group
                    v-model="ksmSelectedProfile"
                    :disabled="ksmLoading || ksmSaving || !ksmStatus?.supported"
                    @change="handleKSMProfileChange"
                  >
                    <el-radio-button
                      v-for="profile in ksmProfileOptions"
                      :key="profile.key"
                      :label="profile.key"
                    >
                      {{ profile.name }}
                    </el-radio-button>
                  </el-radio-group>
                  <el-tag v-if="ksmStatus?.enabled" type="success" effect="plain">运行中</el-tag>
                  <el-tag v-else type="info" effect="plain">已关闭</el-tag>
                  <el-tag v-if="ksmStatus?.persistent_configured" type="info" effect="plain">
                    持久配置：{{ getKSMProfileName(ksmStatus.persistent_profile) }}
                  </el-tag>
                  <el-button size="small" text type="primary" @click="ksmHelpVisible = true">说明</el-button>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  {{ ksmSummary }}
                </div>
                <div class="ksm-profile-list">
                  <div
                    v-for="profile in ksmProfileOptions"
                    :key="profile.key"
                    class="ksm-profile-item"
                    :class="{ active: ksmSelectedProfile === profile.key }"
                  >
                    <strong>{{ profile.name }}</strong>
                    <span>{{ profile.description }}</span>
                  </div>
                </div>
              </div>
            </el-form-item>

            <el-form-item label="运行参数">
              <div class="host-setting-field">
                <div class="host-setting-row">
                  <el-tag effect="plain">run: {{ formatKSMValue(ksmStatus?.runtime_config?.run) }}</el-tag>
                  <el-tag effect="plain">pages_to_scan: {{ formatKSMValue(ksmStatus?.runtime_config?.pages_to_scan) }}</el-tag>
                  <el-tag effect="plain">sleep: {{ formatKSMValue(ksmStatus?.runtime_config?.sleep_millisecs) }}ms</el-tag>
                  <el-tag effect="plain">NUMA 跨节点: {{ formatKSMBool(ksmStatus?.runtime_config?.merge_across_nodes) }}</el-tag>
                  <el-tag effect="plain">零页合并: {{ formatKSMBool(ksmStatus?.runtime_config?.use_zero_pages) }}</el-tag>
                  <el-tag effect="plain">智能扫描: {{ formatKSMBool(ksmStatus?.runtime_config?.smart_scan) }}</el-tag>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  持久化文件: /etc/kvm-console/ksm.env，开机恢复服务: kvm-console-ksm.service
                </div>
              </div>
            </el-form-item>

            <el-form-item label="去重统计">
              <div class="host-setting-row">
                <el-tag effect="plain">共享页: {{ formatKSMValue(ksmStatus?.metrics?.pages_shared) }}</el-tag>
                <el-tag effect="plain">被共享页: {{ formatKSMValue(ksmStatus?.metrics?.pages_sharing) }}</el-tag>
                <el-tag effect="plain">未共享页: {{ formatKSMValue(ksmStatus?.metrics?.pages_unshared) }}</el-tag>
                <el-tag effect="plain">扫描页: {{ formatKSMValue(ksmStatus?.metrics?.pages_scanned) }}</el-tag>
                <el-tag effect="plain">完整扫描: {{ formatKSMValue(ksmStatus?.metrics?.full_scans) }}</el-tag>
              </div>
            </el-form-item>

            <el-divider content-position="left">
              <el-icon style="margin-right: 4px;"><Odometer /></el-icon>
              zRAM 压缩内存
            </el-divider>

            <el-alert
              title="zRAM 会在内存中创建压缩 swap，适合纯虚拟化宿主机作为内存压力缓冲。挡位越高，可用压缩空间越大，CPU 开销也越明显。"
              type="info"
              :closable="false"
              style="margin-bottom: 18px;"
            />

            <el-form-item label="zRAM 挡位">
              <div class="host-setting-field">
                <div class="host-setting-row">
                  <el-radio-group
                    v-model="zramSelectedProfile"
                    :disabled="zramLoading || zramSaving || !zramStatus?.supported"
                    @change="handleZRAMProfileChange"
                  >
                    <el-radio-button
                      v-for="profile in zramProfileOptions"
                      :key="profile.key"
                      :label="profile.key"
                    >
                      {{ profile.name }}
                    </el-radio-button>
                  </el-radio-group>
                  <el-tag v-if="zramStatus?.enabled" type="success" effect="plain">运行中</el-tag>
                  <el-tag v-else type="info" effect="plain">已关闭</el-tag>
                  <el-tag v-if="zramStatus?.persistent_configured" type="info" effect="plain">
                    持久配置：{{ getZRAMProfileName(zramStatus.persistent_profile) }}
                  </el-tag>
                  <el-button size="small" text type="primary" @click="zramHelpVisible = true">说明</el-button>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  {{ zramSummary }}
                </div>
                <div class="ksm-profile-list">
                  <div
                    v-for="profile in zramProfileOptions"
                    :key="profile.key"
                    class="ksm-profile-item"
                    :class="{ active: zramSelectedProfile === profile.key }"
                  >
                    <strong>{{ profile.name }}</strong>
                    <span>{{ profile.description }}</span>
                  </div>
                </div>
              </div>
            </el-form-item>

            <el-form-item label="zRAM 运行参数">
              <div class="host-setting-field">
                <div class="host-setting-row">
                  <el-tag effect="plain">设备: {{ zramStatus?.runtime_config?.device || '-' }}</el-tag>
                  <el-tag effect="plain">容量: {{ formatZRAMMB(zramStatus?.runtime_config?.size_mb) }}</el-tag>
                  <el-tag effect="plain">已用: {{ formatZRAMMB(zramStatus?.runtime_config?.used_mb) }}</el-tag>
                  <el-tag effect="plain">算法: {{ zramStatus?.runtime_config?.algorithm || '-' }}</el-tag>
                  <el-tag effect="plain">优先级: {{ formatKSMValue(zramStatus?.runtime_config?.priority) }}</el-tag>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  持久化文件: /etc/kvm-console/zram.env，开机恢复服务: kvm-console-zram.service
                </div>
              </div>
            </el-form-item>

            <el-divider content-position="left">
              <el-icon style="margin-right: 4px;"><Cpu /></el-icon>
              虚拟化兼容性
            </el-divider>

            <el-alert
              title="这里是宿主机级 KVM 参数，会影响当前宿主机上的所有 Intel KVM 虚拟机。普通情况下请保持默认。"
              type="warning"
              :closable="false"
              style="margin-bottom: 18px;"
            />

            <el-form-item label="KVM Unrestricted Guest">
              <div class="kvm-compat-field">
                <div class="kvm-compat-row">
                  <el-switch
                    v-model="kvmUnrestrictedGuestEnabled"
                    active-text="启用"
                    inactive-text="禁用"
                    :loading="kvmUnrestrictedGuestSaving"
                    :disabled="kvmUnrestrictedGuestLoading || kvmUnrestrictedGuestSaving || !kvmUnrestrictedGuestStatus?.supported"
                    @change="handleKVMUnrestrictedGuestChange"
                  />
                  <el-tag v-if="kvmUnrestrictedGuestStatus?.runtime_available" :type="kvmUnrestrictedGuestStatus.runtime_enabled ? 'success' : 'warning'" effect="plain">
                    运行时：{{ kvmUnrestrictedGuestStatus.runtime_enabled ? '已启用' : '已禁用' }}
                  </el-tag>
                  <el-tag v-if="kvmUnrestrictedGuestStatus?.persistent_configured" type="info" effect="plain">
                    持久配置：{{ kvmUnrestrictedGuestStatus.persistent_enabled ? '启用' : '禁用' }}
                  </el-tag>
                  <el-tag v-if="kvmUnrestrictedGuestStatus?.requires_reload" type="warning" effect="plain">待重载</el-tag>
                  <el-button size="small" text type="primary" @click="kvmUnrestrictedGuestHelpVisible = true">说明</el-button>
                </div>
                <div class="form-tip">
                  <el-icon><InfoFilled /></el-icon>
                  {{ kvmUnrestrictedGuestSummary }}
                </div>
              </div>
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="调度与高级" name="advanced">
            <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Odometer /></el-icon>
          动态内存调度
        </el-divider>

        <el-form-item label="启用自动调度">
          <el-switch v-model="form.dynamic_memory_scheduler_enabled" active-text="启用" inactive-text="关闭" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            仅对已启用动态内存且允许自动气球调度的 VM 生效 | 环境变量: KVM_DYNAMIC_MEMORY_SCHEDULER_ENABLED
          </div>
        </el-form-item>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="调度间隔">
              <el-input-number v-model="form.dynamic_memory_interval_seconds" :min="10" :max="3600" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位秒，默认 30
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="调整冷却">
              <el-input-number v-model="form.dynamic_memory_cooldown_seconds" :min="30" :max="7200" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位秒，同一 VM 两次调整之间的最短间隔
              </div>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="宿主保留内存">
              <el-input-number v-model="form.dynamic_memory_host_reserve_mb" :min="512" :max="1048576" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位 MB，默认 2048
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="宿主保留比例">
              <el-input-number v-model="form.dynamic_memory_host_reserve_percent" :min="5" :max="80" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位 %，最终保留值取固定值与比例值中的较大者
              </div>
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="增长阈值">
              <el-input-number v-model="form.dynamic_memory_increase_threshold_percent" :min="5" :max="50" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                可用内存比例低于该值时尝试增长
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="回收阈值">
              <el-input-number v-model="form.dynamic_memory_reclaim_threshold_percent" :min="10" :max="90" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                空闲内存比例高于该值时才考虑回收
              </div>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="首次观察期">
          <el-input-number v-model="form.dynamic_memory_observation_hours" :min="0" :max="168" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            单位小时，观察期内不自动回收到启动内存以下 | 环境变量前缀: KVM_DYNAMIC_MEMORY_*
          </div>
        </el-form-item>

        <el-form-item label="调度事件保留">
          <el-input-number v-model="form.scheduler_event_retention_hours" :min="1" :max="2160" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            单位小时，默认 168，小于该时长的调度事件会被后台定时清理 | 环境变量: KVM_SCHEDULER_EVENT_RETENTION_HOURS
          </div>
        </el-form-item>

        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Connection /></el-icon>
          端口转发 HTTP 探测
        </el-divider>

        <el-form-item label="启用自动探测">
          <el-switch v-model="form.port_forward_http_probe_enabled" active-text="启用" inactive-text="关闭" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            每轮只探测 TCP 转发，发现明文 HTTP 且未命中白名单时会自动封禁 | 环境变量: KVM_PORT_FORWARD_HTTP_PROBE_ENABLED
          </div>
        </el-form-item>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="探测间隔">
              <el-input-number v-model="form.port_forward_http_probe_interval_minutes" :min="5" :max="1440" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位分钟，默认 60 | 环境变量: KVM_PORT_FORWARD_HTTP_PROBE_INTERVAL_MINUTES
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="连接超时">
              <el-input-number v-model="form.port_forward_http_probe_timeout_seconds" :min="1" :max="30" style="width: 100%;" />
              <div class="form-tip">
                <el-icon><InfoFilled /></el-icon>
                单位秒，默认 3 | 环境变量: KVM_PORT_FORWARD_HTTP_PROBE_TIMEOUT_SECONDS
              </div>
            </el-form-item>
          </el-col>
        </el-row>
            <!-- 批量克隆 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><CopyDocument /></el-icon>
          批量克隆
        </el-divider>

        <el-form-item label="最大同时克隆数">
          <el-input-number v-model="form.batch_clone_max_concurrency" :min="1" :max="100" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            批量克隆时最多允许同时克隆的虚拟机数量，默认 10，设为 1 时退化为顺序克隆 | 环境变量: KVM_BATCH_CLONE_MAX_CONCURRENCY
          </div>
        </el-form-item>
            <!-- 救援系统 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><FirstAidKit /></el-icon>
          救援系统
        </el-divider>

        <el-form-item label="救援系统 ISO">
          <el-select v-model="form.rescue_iso" placeholder="请选择救援系统 ISO" clearable filterable style="width: 100%;">
            <el-option
              v-for="iso in isoList"
              :key="iso.path"
              :label="iso.name"
              :value="iso.path"
            />
          </el-select>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            选择一个 ISO 文件作为虚拟机救援系统，列表来源于上方 ISO 存放位置 | 环境变量: KVM_RESCUE_ISO
          </div>
        </el-form-item>
            <!-- CPU 亲和性预设 -->
        <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Cpu /></el-icon>
          CPU 亲和性预设
        </el-divider>

        <el-form-item>
          <div class="preset-manager" style="width: 100%;">
            <div v-if="affinityPresets.length === 0" style="color: #909399; font-size: 13px; margin-bottom: 12px;">
              暂无预设，可点击下方按钮添加。
            </div>
            <div v-for="(preset, idx) in affinityPresets" :key="idx" class="preset-row" style="display: flex; gap: 8px; margin-bottom: 8px; align-items: center;">
              <el-input v-model="preset.name" placeholder="预设名称" style="width: 200px;" />
              <el-input v-model="preset.value" placeholder="核心值，如 0-3" style="width: 280px;" />
              <el-button type="danger" :icon="Delete" circle size="small" @click="affinityPresets.splice(idx, 1)" />
            </div>
            <div style="display: flex; gap: 8px; margin-top: 8px;">
              <el-button size="small" @click="affinityPresets.push({ name: '', value: '' })">
                <el-icon><Plus /></el-icon>
                添加预设
              </el-button>
              <el-button type="primary" size="small" :loading="affinityPresetsSaving" @click="saveAffinityPresets">
                <el-icon><Check /></el-icon>
                保存预设
              </el-button>
              <el-button size="small" @click="loadAffinityPresets(true)">
                <el-icon><Refresh /></el-icon>
                重置
              </el-button>
            </div>
          </div>
        </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="安全与维护" name="security">
            <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Message /></el-icon>
          邮件与安全验证
        </el-divider>

        <el-alert
          :title="form.smtp_configured ? 'SMTP 已配置，可用于邮箱绑定、邀请注册和密码找回。' : 'SMTP 尚未配置，邮箱绑定、邀请注册和密码找回将不可用。'"
          :type="form.smtp_configured ? 'success' : 'warning'"
          :closable="false"
          style="margin-bottom: 18px;"
        />

        <el-form-item label="启用开发环境">
          <el-switch v-model="form.development_mode" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            启用后将绕过登录二段验证、首次强制绑定和高风险操作验证，仅建议在开发调试环境使用 | 环境变量: KVM_DEVELOPMENT_MODE
          </div>
        </el-form-item>
            <el-form-item label="SMTP 主机">
          <el-input v-model="form.smtp_host" placeholder="如 smtp.qq.com" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_HOST
          </div>
        </el-form-item>

        <el-form-item label="SMTP 端口">
          <el-input-number v-model="form.smtp_port" :min="1" :max="65535" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_PORT
          </div>
        </el-form-item>

        <el-form-item label="SMTP 用户名">
          <el-input v-model="form.smtp_username" placeholder="通常为发件邮箱账号" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_USERNAME
          </div>
        </el-form-item>

        <el-form-item label="SMTP 密码">
          <el-input
            v-model="form.smtp_password"
            type="password"
            show-password
            :placeholder="form.smtp_password_configured ? '留空表示保持当前密码不变' : '请输入 SMTP 密码或授权码'"
          />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            {{ form.smtp_password_configured ? '当前已保存 SMTP 密码，留空不会覆盖。' : '环境变量: KVM_SMTP_PASSWORD_ENC' }}
          </div>
        </el-form-item>

        <el-form-item label="发件人名称">
          <el-input v-model="form.smtp_from_name" placeholder="默认展示名称" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_FROM_NAME
          </div>
        </el-form-item>

        <el-form-item label="发件邮箱">
          <el-input v-model="form.smtp_from_address" placeholder="如 no-reply@example.com" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_FROM_ADDRESS
          </div>
        </el-form-item>

        <el-form-item label="加密方式">
          <el-select v-model="form.smtp_security" style="width: 100%;">
            <el-option label="STARTTLS" value="starttls" />
            <el-option label="SSL/TLS" value="ssl" />
            <el-option label="无加密" value="none" />
          </el-select>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_SECURITY
          </div>
        </el-form-item>

        <el-form-item label="超时秒数">
          <el-input-number v-model="form.smtp_timeout_seconds" :min="5" :max="120" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            环境变量: KVM_SMTP_TIMEOUT_SECONDS
          </div>
        </el-form-item>

        <el-form-item label="测试收件邮箱">
          <el-input v-model="form.smtp_test_email" placeholder="保存配置后发送测试邮件" />
        </el-form-item>

            <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Warning /></el-icon>
          JWT 密钥管理
        </el-divider>

        <el-form-item label="自动轮换间隔">
          <el-input-number v-model="form.jwt_secret_rotate_hours" :min="0" :max="720" :disabled="form.development_mode" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            默认 24 小时自动轮换 JWT 签名密钥，设为 0 禁用自动轮换。开发模式下自动轮换会被跳过 | 环境变量: KVM_JWT_SECRET_ROTATE_HOURS
          </div>
        </el-form-item>

        <el-form-item v-if="form.jwt_secret_last_rotated" label="上次轮换时间">
          <el-tag type="info">{{ form.jwt_secret_last_rotated }}</el-tag>
        </el-form-item>

        <el-form-item label="手动轮换JWT密钥">
          <el-button
            type="danger"
            :loading="rotatingJWT"
            :disabled="form.development_mode"
            @click="handleRotateJWT"
          >
            {{ form.development_mode ? '开发模式不允许轮换' : (rotatingJWT ? '轮换中...' : '立即轮换 JWT 密钥') }}
          </el-button>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            <span v-if="form.development_mode" style="color: var(--el-color-warning);">开发模式下 JWT 密钥轮换功能已禁用</span>
            <span v-else>轮换后所有 Token 将立即失效，所有用户需重新登录。此操作需高风险二次验证</span>
          </div>
        </el-form-item>

            <el-divider content-position="left">
          <el-icon style="margin-right: 4px;"><Warning /></el-icon>
          维护模式
        </el-divider>

        <el-alert
          title="启用维护模式后，系统会异步关闭所有运行中的虚拟机，并停用配置中的宿主机服务。维护模式期间将阻止虚拟机启动。"
          type="warning"
          :closable="false"
          style="margin-bottom: 18px;"
        />

        <el-form-item label="启用维护模式">
          <el-switch v-model="form.maintenance_mode" active-text="启用" inactive-text="关闭" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            保存时会要求二次验证，启用后可到任务中心查看执行进度 | 环境变量: KVM_MAINTENANCE_MODE
          </div>
        </el-form-item>

        <el-form-item label="关机等待时间">
          <el-input-number v-model="form.maintenance_vm_shutdown_timeout_seconds" :min="5" :max="3600" style="width: 100%;" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            单位秒，维护模式关闭虚拟机时先尝试优雅关机，超时后会强制断电 | 环境变量: KVM_MAINTENANCE_VM_SHUTDOWN_TIMEOUT_SECONDS
          </div>
        </el-form-item>

        <el-form-item label="维护服务列表">
          <el-input
            v-model="form.maintenance_service_units"
            type="textarea"
            :rows="5"
            placeholder="每行一个 systemd unit，也支持逗号分隔"
          />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            建议填写 libvirtd 等相关服务；`kvm-console.service` 即使加入也会被自动跳过，确保主机重启后面板仍自动启动 | 环境变量: KVM_MAINTENANCE_SERVICE_UNITS
          </div>
        </el-form-item>
          </el-tab-pane>
        </el-tabs>

        <el-form-item>
          <el-button type="primary" :loading="saving" @click="handleSave">
            <el-icon style="margin-right: 4px;"><Check /></el-icon>
            保存设置
          </el-button>
          <el-button @click="fetchData">
            <el-icon style="margin-right: 4px;"><Refresh /></el-icon>
            重置
          </el-button>
          <el-button :loading="testing" @click="handleTestSMTP">
            <el-icon style="margin-right: 4px;"><Message /></el-icon>
            测试发信
          </el-button>
        </el-form-item>
      </el-form>

    </el-card>

    <el-dialog v-model="ksmHelpVisible" title="KSM 内存去重说明" width="640px" append-to-body>
      <div class="settings-detail-content">
        <p>KSM 会在宿主机后台扫描匿名内存页，将内容完全相同的页面合并为同一份物理内存。纯虚拟化宿主机上，如果多个 VM 运行相近系统或模板，通常能节省明显内存。</p>
        <p>“均衡”是默认推荐挡位；“积极”和“极致”会更快扫描重复页，适合内存紧张或 VM 密度较高的宿主机，但会带来更多 CPU 开销。</p>
        <p>关闭 KSM 不会立即把已合并页面全部拆开，内核会在页面被写入时逐步拆分。需要彻底回收合并状态时，建议安排维护窗口后再做更激进的内核级操作。</p>
        <p>保存挡位会写入当前运行时，并通过 `kvm-console-ksm.service` 在宿主机重启后自动恢复。</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="ksmHelpVisible = false">我知道了</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="zramHelpVisible" title="zRAM 压缩内存说明" width="640px" append-to-body>
      <div class="settings-detail-content">
        <p>zRAM 会把一段内存作为压缩块设备并启用 swap。它不会像磁盘 swap 那样写入硬盘，适合作为虚拟化宿主机在内存压力上升时的缓冲层。</p>
        <p>面板只管理带有 `kvm-zram` 标签的 zRAM 设备；切换挡位时会先关闭旧的面板管理设备，再按新挡位创建。</p>
        <p>“均衡”是默认推荐挡位，逻辑容量为宿主机内存 20%，最高 32 GiB；“积极”和“极致”会给更大的压缩空间，但会占用更多 CPU。</p>
        <p>保存挡位会立即影响当前宿主机，并通过 `kvm-console-zram.service` 在宿主机重启后自动恢复。</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="zramHelpVisible = false">我知道了</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="kvmUnrestrictedGuestHelpVisible" title="KVM Unrestricted Guest 说明" width="640px" append-to-body>
      <el-alert type="warning" :closable="false" style="margin-bottom: 16px;">
        <template #title>
          这是宿主机级 KVM 参数，会影响当前宿主机上的所有 Intel KVM 虚拟机，并不是单台虚拟机独立配置。
        </template>
      </el-alert>
      <div class="settings-detail-content">
        <p>Unrestricted Guest 是 Intel KVM 的硬件辅助能力，通常保持启用即可，能让虚拟机更直接地运行早期启动阶段代码。</p>
        <p>在部分 VMware / ESXi 嵌套虚拟化环境中，该能力可能触发 QEMU 启动时报错，例如 `KVM: entry failed, hardware error 0x7`，虚拟机会进入内部错误暂停状态。</p>
        <p>遇到上述问题时，可以临时禁用该参数作为兼容性绕过。禁用后可能略微影响早期启动阶段性能，但通常比虚拟机无法启动更可接受。</p>
        <p>若当前有虚拟机正在运行或暂停，系统只会保存持久配置。需要关停所有虚拟机后重载 KVM 模块生效。</p>
        <p>若当前没有运行中的虚拟机但模块仍无法热卸载（例如系统运行在 KVM 嵌套虚拟化环境中），则需要重启宿主机后才会完全生效。</p>
      </div>
      <template #footer>
        <el-button type="primary" @click="kvmUnrestrictedGuestHelpVisible = false">我知道了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, reactive, onMounted } from 'vue'
import { Check, Connection, CopyDocument, Cpu, Delete, FirstAidKit, FolderOpened, InfoFilled, Message, Odometer, Plus, Refresh, Warning } from '@element-plus/icons-vue'
import { getHostKSMStatus, getHostKVMUnrestrictedGuestStatus, getHostZRAMStatus, getSettings, getCPUAffinityPresets, rotateJWTSecret, saveCPUAffinityPresets, testSMTP, updateHostKSMProfile, updateHostKVMUnrestrictedGuest, updateHostZRAMProfile, updateSettings } from '@/api/settings'
import { getAllISOs } from '@/api/infra'
import { ElMessage, ElMessageBox } from 'element-plus'
import { setSiteTitle } from '@/utils/site'

const defaultMaintenanceServiceUnits = 'kvm-console.service,libvirtd.service,libvirtd.socket,libvirtd-ro.socket,libvirtd-admin.socket'
const fallbackKSMProfiles = [
  { key: 'off', name: '关闭', description: '不扫描内存页，适合临时排障或 CPU 压力优先的宿主机。' },
  { key: 'conservative', name: '保守', description: '低频扫描，优先降低 CPU 开销，适合内存压力不高的虚拟化宿主机。' },
  { key: 'balanced', name: '均衡', description: '推荐挡位，启用零页合并，在节省内存和控制扫描开销之间取平衡。' },
  { key: 'aggressive', name: '积极', description: '提高扫描速度，适合 VM 密度较高且希望更快释放重复内存的宿主机。' },
  { key: 'extreme', name: '极致', description: '最大化去重速度，适合内存非常紧张的纯虚拟化宿主机，CPU 开销会更明显。' }
]
const fallbackZRAMProfiles = [
  { key: 'off', name: '关闭', description: '关闭面板管理的 zRAM swap，适合排障或宿主机内存压力很低的场景。' },
  { key: 'conservative', name: '保守', description: 'zRAM 逻辑容量为宿主机内存 10%，最高 16 GiB，优先降低压缩和换页开销。' },
  { key: 'balanced', name: '均衡', description: 'zRAM 逻辑容量为宿主机内存 20%，最高 32 GiB，适合作为纯虚拟化宿主机默认挡位。' },
  { key: 'aggressive', name: '积极', description: 'zRAM 逻辑容量为宿主机内存 35%，最高 64 GiB，适合 VM 密度高且希望优先压缩内存的宿主机。' },
  { key: 'extreme', name: '极致', description: 'zRAM 逻辑容量为宿主机内存 50%，最高 128 GiB，适合内存非常紧张且能接受更多 CPU 开销的宿主机。' }
]

const activeTab = ref('basic')
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref(null)
const ksmHelpVisible = ref(false)
const ksmLoading = ref(false)
const ksmSaving = ref(false)
const ksmSelectedProfile = ref('balanced')
const ksmStatus = ref(null)
const zramHelpVisible = ref(false)
const zramLoading = ref(false)
const zramSaving = ref(false)
const zramSelectedProfile = ref('balanced')
const zramStatus = ref(null)
const kvmUnrestrictedGuestHelpVisible = ref(false)
const kvmUnrestrictedGuestLoading = ref(false)
const kvmUnrestrictedGuestSaving = ref(false)
const kvmUnrestrictedGuestEnabled = ref(true)
const kvmUnrestrictedGuestStatus = ref(null)

const affinityPresets = ref([])
const affinityPresetsSaving = ref(false)

const form = reactive({
  port: 8080,
  template_dir: '',
  template_import_dir: '',
  template_export_dir: '',
  clone_dir: '',
  iso_dir: '/var/lib/libvirt/images/ISO',
  default_network: '',
  network_backend: 'ovs',
  ovs_bridge: 'br-ovs',
  ovs_uplink: '',
  ovs_dhcp_start: '',
  ovs_dhcp_end: '',
  subnet_prefix: '',
  auto_port_start: 10000,
  auto_port_end: 20000,
  port_forward_dir: '',
  host_ip: '',
  external_nic: '',
  max_burst_inbound: 0,
  max_burst_outbound: 0,
  default_disk_iops_total: 0,
  default_disk_iops_read: 0,
  default_disk_iops_write: 0,
  batch_clone_max_concurrency: 10,
  dynamic_memory_scheduler_enabled: true,
  dynamic_memory_interval_seconds: 30,
  dynamic_memory_host_reserve_mb: 2048,
  dynamic_memory_host_reserve_percent: 20,
  dynamic_memory_increase_threshold_percent: 15,
  dynamic_memory_reclaim_threshold_percent: 35,
  dynamic_memory_cooldown_seconds: 120,
  dynamic_memory_observation_hours: 24,
  scheduler_event_retention_hours: 168,
  port_forward_http_probe_enabled: true,
  port_forward_http_probe_interval_minutes: 60,
  port_forward_http_probe_timeout_seconds: 3,
  rescue_iso: '',
  public_base_url: '',
  site_title: 'QVMConsole',
  development_mode: false,
  maintenance_mode: false,
  maintenance_service_units: defaultMaintenanceServiceUnits,
  maintenance_vm_shutdown_timeout_seconds: 40,
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password: '',
  smtp_from_name: 'QVMConsole',
  smtp_from_address: '',
  smtp_security: 'starttls',
  smtp_timeout_seconds: 15,
  smtp_password_configured: false,
  smtp_configured: false,
  smtp_test_email: '',
  jwt_secret_rotate_hours: 24,
  jwt_secret_last_rotated: '',
})

// ISO 列表
const isoList = ref([])

const ksmProfileOptions = computed(() => ksmStatus.value?.profiles?.length ? ksmStatus.value.profiles : fallbackKSMProfiles)
const zramProfileOptions = computed(() => zramStatus.value?.profiles?.length ? zramStatus.value.profiles : fallbackZRAMProfiles)

const getKSMProfileName = (key) => {
  const profile = ksmProfileOptions.value.find(item => item.key === key)
  return profile?.name || key || '未配置'
}

const getZRAMProfileName = (key) => {
  const profile = zramProfileOptions.value.find(item => item.key === key)
  return profile?.name || key || '未配置'
}

const formatKSMValue = (value) => {
  if (value === null || value === undefined) return '-'
  return Number(value).toLocaleString()
}

const formatZRAMMB = (value) => {
  if (value === null || value === undefined) return '-'
  return `${Number(value).toLocaleString()} MB`
}

const formatKSMBool = (value) => {
  if (value === null || value === undefined) return '-'
  return value ? '开启' : '关闭'
}

const ksmSummary = computed(() => {
  const status = ksmStatus.value
  if (ksmLoading.value) return '正在读取宿主机 KSM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机 KSM 状态。'
  if (!status.supported) return status.message || '当前宿主机未提供 KSM sysfs 接口。'
  const current = getKSMProfileName(status.current_profile)
  const persistent = status.persistent_configured ? `，重启后恢复为${getKSMProfileName(status.persistent_profile)}` : '，尚未写入持久配置'
  const sharedPages = status.metrics?.pages_sharing
  const sharedText = sharedPages !== null && sharedPages !== undefined ? `，当前被共享页 ${formatKSMValue(sharedPages)}` : ''
  return `当前挡位为${current}${persistent}${sharedText}。`
})

const zramSummary = computed(() => {
  const status = zramStatus.value
  if (zramLoading.value) return '正在读取宿主机 zRAM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机 zRAM 状态。'
  if (!status.supported) return status.message || '当前宿主机缺少 zRAM 内核能力或 util-linux 相关工具。'
  const current = getZRAMProfileName(status.current_profile)
  const persistent = status.persistent_configured ? `，重启后恢复为${getZRAMProfileName(status.persistent_profile)}` : '，尚未写入持久配置'
  const sizeText = status.runtime_config?.size_mb !== null && status.runtime_config?.size_mb !== undefined ? `，当前容量 ${formatZRAMMB(status.runtime_config.size_mb)}` : ''
  return `当前挡位为${current}${persistent}${sizeText}。`
})

const applyKSMStatus = (status) => {
  ksmStatus.value = status || null
  if (!status) return
  if (status.persistent_profile) {
    ksmSelectedProfile.value = status.persistent_profile
  } else if (status.current_profile && status.current_profile !== 'custom') {
    ksmSelectedProfile.value = status.current_profile
  }
}

const applyZRAMStatus = (status) => {
  zramStatus.value = status || null
  if (!status) return
  if (status.persistent_profile) {
    zramSelectedProfile.value = status.persistent_profile
  } else if (status.current_profile && status.current_profile !== 'custom') {
    zramSelectedProfile.value = status.current_profile
  }
}

const loadKSMStatus = async () => {
  ksmLoading.value = true
  try {
    const res = await getHostKSMStatus()
    applyKSMStatus(res.data)
  } catch (err) {
    console.error('读取 KSM 状态失败', err)
  } finally {
    ksmLoading.value = false
  }
}

const loadZRAMStatus = async () => {
  zramLoading.value = true
  try {
    const res = await getHostZRAMStatus()
    applyZRAMStatus(res.data)
  } catch (err) {
    console.error('读取 zRAM 状态失败', err)
  } finally {
    zramLoading.value = false
  }
}

const kvmUnrestrictedGuestSummary = computed(() => {
  const status = kvmUnrestrictedGuestStatus.value
  if (kvmUnrestrictedGuestLoading.value) return '正在读取宿主机 KVM 参数...'
  if (!status) return '进入系统设置后会自动读取当前宿主机运行时参数。'
  if (!status.supported) return status.message || '当前宿主机未加载 kvm_intel，或不是 Intel KVM 环境。'
  const runtimeText = status.runtime_enabled ? '运行时已启用' : '运行时已禁用'
  const persistentText = status.persistent_configured
    ? `持久配置为${status.persistent_enabled ? '启用' : '禁用'}`
    : '尚未写入持久配置'
  if (status.requires_reload) {
    if (status.message && status.message.includes('重启')) {
      return `${runtimeText}，${persistentText}；模块无法热卸载，需重启宿主机后生效。`
    }
    return `${runtimeText}，${persistentText}；需要重载 KVM 模块或重启宿主机后完全生效。`
  }
  if (status.active_vm_count > 0) {
    return `${runtimeText}，${persistentText}；当前有 ${status.active_vm_count} 台虚拟机运行或暂停，切换后会先保存配置。`
  }
  return `${runtimeText}，${persistentText}。VMware 嵌套虚拟化出现 hardware error 0x7 时可尝试禁用。`
})

const applyKVMUnrestrictedGuestStatus = (status) => {
  kvmUnrestrictedGuestStatus.value = status || null
  if (!status) return
  if (status.persistent_configured) {
    kvmUnrestrictedGuestEnabled.value = !!status.persistent_enabled
  } else if (status.runtime_available) {
    kvmUnrestrictedGuestEnabled.value = !!status.runtime_enabled
  }
}

const loadKVMUnrestrictedGuestStatus = async () => {
  kvmUnrestrictedGuestLoading.value = true
  try {
    const res = await getHostKVMUnrestrictedGuestStatus()
    applyKVMUnrestrictedGuestStatus(res.data)
  } catch (err) {
    console.error('读取 KVM Unrestricted Guest 状态失败', err)
  } finally {
    kvmUnrestrictedGuestLoading.value = false
  }
}

const handleKSMProfileChange = async (profileKey) => {
  const previousProfile = ksmStatus.value?.persistent_profile || ksmStatus.value?.current_profile || 'balanced'
  const profileName = getKSMProfileName(profileKey)
  try {
    await ElMessageBox.confirm(
      `确定要将宿主机 KSM 切换到“${profileName}”挡位吗？该配置会立即影响当前宿主机上的所有虚拟机。`,
      '设置宿主机 KSM',
      {
        confirmButtonText: '应用',
        cancelButtonText: '取消',
        type: profileKey === 'off' ? 'warning' : 'info',
      }
    )
  } catch (action) {
    if (action === 'cancel' || action === 'close') {
      ksmSelectedProfile.value = previousProfile
    }
    return
  }

  ksmSaving.value = true
  try {
    const res = await updateHostKSMProfile({ profile: profileKey })
    applyKSMStatus(res.data)
    ElMessage.success(res.message || 'KSM 挡位已保存')
  } catch (err) {
    console.error('设置 KSM 挡位失败', err)
    ksmSelectedProfile.value = previousProfile
    loadKSMStatus()
  } finally {
    ksmSaving.value = false
  }
}

const handleZRAMProfileChange = async (profileKey) => {
  const previousProfile = zramStatus.value?.persistent_profile || zramStatus.value?.current_profile || 'balanced'
  const profileName = getZRAMProfileName(profileKey)
  try {
    await ElMessageBox.confirm(
      `确定要将宿主机 zRAM 切换到“${profileName}”挡位吗？该配置会立即重建面板管理的 zRAM swap，并影响当前宿主机的内存回收策略。`,
      '设置宿主机 zRAM',
      {
        confirmButtonText: '应用',
        cancelButtonText: '取消',
        type: profileKey === 'off' ? 'warning' : 'info',
      }
    )
  } catch (action) {
    if (action === 'cancel' || action === 'close') {
      zramSelectedProfile.value = previousProfile
    }
    return
  }

  zramSaving.value = true
  try {
    const res = await updateHostZRAMProfile({ profile: profileKey })
    applyZRAMStatus(res.data)
    ElMessage.success(res.message || 'zRAM 挡位已保存')
  } catch (err) {
    console.error('设置 zRAM 挡位失败', err)
    zramSelectedProfile.value = previousProfile
    loadZRAMStatus()
  } finally {
    zramSaving.value = false
  }
}

const fetchData = async () => {
  loading.value = true
  try {
    const [settingsResult, isoResult, kvmResult, ksmResult, zramResult] = await Promise.allSettled([
      getSettings(),
      getAllISOs(),
      getHostKVMUnrestrictedGuestStatus(),
      getHostKSMStatus(),
      getHostZRAMStatus()
    ])

    loadAffinityPresets()

    if (settingsResult.status !== 'fulfilled') {
      throw settingsResult.reason
    }

    Object.assign(form, settingsResult.value.data || {})
    setSiteTitle(form.site_title)
    if (!form.maintenance_service_units?.trim()) {
      form.maintenance_service_units = defaultMaintenanceServiceUnits
    }
    form.smtp_password = ''
    isoList.value = isoResult.status === 'fulfilled' ? (isoResult.value.data || []) : []
    if (kvmResult.status === 'fulfilled') {
      applyKVMUnrestrictedGuestStatus(kvmResult.value.data)
    }
    if (ksmResult.status === 'fulfilled') {
      applyKSMStatus(ksmResult.value.data)
    }
    if (zramResult.status === 'fulfilled') {
      applyZRAMStatus(zramResult.value.data)
    }
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

const handleKVMUnrestrictedGuestChange = async (enabled) => {
  const previousValue = !enabled
  const actionText = enabled ? '启用' : '禁用'
  const confirmText = enabled
    ? '确定要启用 KVM Unrestricted Guest 吗？这会恢复 Intel KVM 默认硬件辅助行为。'
    : '确定要禁用 KVM Unrestricted Guest 吗？该设置主要用于绕过 VMware 嵌套虚拟化下的 QEMU hardware error 0x7。'
  try {
    await ElMessageBox.confirm(confirmText, `${actionText}宿主机 KVM 参数`, {
      confirmButtonText: actionText,
      cancelButtonText: '取消',
      type: enabled ? 'warning' : 'error',
    })
  } catch (action) {
    if (action === 'cancel' || action === 'close') {
      kvmUnrestrictedGuestEnabled.value = previousValue
    }
    return
  }

  kvmUnrestrictedGuestSaving.value = true
  try {
    const res = await updateHostKVMUnrestrictedGuest({ enabled })
    applyKVMUnrestrictedGuestStatus(res.data)
    ElMessage.success(res.message || res.data?.message || 'KVM 参数已保存')
  } catch (err) {
    console.error('设置 KVM Unrestricted Guest 失败', err)
    kvmUnrestrictedGuestEnabled.value = previousValue
    loadKVMUnrestrictedGuestStatus()
  } finally {
    kvmUnrestrictedGuestSaving.value = false
  }
}

const handleSave = async () => {
  // 验证
  if (form.auto_port_start >= form.auto_port_end) {
    ElMessage.error('端口起始值必须小于结束值')
    return
  }
  if (form.auto_port_start < 1024 || form.auto_port_end > 65535) {
    ElMessage.error('端口范围: 1024 - 65535')
    return
  }
  if (form.smtp_port < 1 || form.smtp_port > 65535) {
    ElMessage.error('SMTP 端口范围: 1 - 65535')
    return
  }
  if (form.smtp_timeout_seconds < 5) {
    ElMessage.error('SMTP 超时时间不能小于 5 秒')
    return
  }
  if (form.dynamic_memory_interval_seconds < 10) {
    ElMessage.error('动态内存调度间隔不能小于 10 秒')
    return
  }
  if (form.dynamic_memory_host_reserve_mb < 512) {
    ElMessage.error('宿主机保留内存不能小于 512MB')
    return
  }
  if (form.dynamic_memory_host_reserve_percent < 5 || form.dynamic_memory_host_reserve_percent > 80) {
    ElMessage.error('宿主机保留比例需在 5% - 80% 之间')
    return
  }
  if (form.dynamic_memory_increase_threshold_percent < 5 || form.dynamic_memory_increase_threshold_percent > 50) {
    ElMessage.error('增长触发阈值需在 5% - 50% 之间')
    return
  }
  if (form.dynamic_memory_reclaim_threshold_percent < 10 || form.dynamic_memory_reclaim_threshold_percent > 90) {
    ElMessage.error('回收触发阈值需在 10% - 90% 之间')
    return
  }
  if (form.dynamic_memory_cooldown_seconds < 30) {
    ElMessage.error('动态内存冷却时间不能小于 30 秒')
    return
  }
  if (form.dynamic_memory_observation_hours < 0 || form.dynamic_memory_observation_hours > 168) {
    ElMessage.error('观察期需在 0 - 168 小时之间')
    return
  }
  if (form.scheduler_event_retention_hours < 1 || form.scheduler_event_retention_hours > 2160) {
    ElMessage.error('调度事件保留时长需在 1 - 2160 小时之间')
    return
  }
  if (form.port_forward_http_probe_interval_minutes < 5 || form.port_forward_http_probe_interval_minutes > 1440) {
    ElMessage.error('端口转发 HTTP 探测间隔需在 5 - 1440 分钟之间')
    return
  }
  if (form.port_forward_http_probe_timeout_seconds < 1 || form.port_forward_http_probe_timeout_seconds > 30) {
    ElMessage.error('端口转发 HTTP 探测超时需在 1 - 30 秒之间')
    return
  }
  if (form.maintenance_vm_shutdown_timeout_seconds < 5 || form.maintenance_vm_shutdown_timeout_seconds > 3600) {
    ElMessage.error('维护模式虚拟机关机等待时间需在 5 - 3600 秒之间')
    return
  }
  if (!form.maintenance_service_units.trim()) {
    form.maintenance_service_units = defaultMaintenanceServiceUnits
  }

  saving.value = true
  try {
    const res = await updateSettings(buildPayload())
    setSiteTitle(form.site_title)
    ElMessage.success(res.message || '设置已保存')
    await fetchData()
  } catch (err) {
    console.error(err)
  } finally {
    saving.value = false
  }
}

const buildPayload = () => ({
  template_dir: form.template_dir,
  template_import_dir: form.template_import_dir,
  template_export_dir: form.template_export_dir,
  clone_dir: form.clone_dir,
  iso_dir: form.iso_dir,
  default_network: form.default_network,
  network_backend: form.network_backend || 'ovs',
  ovs_bridge: form.ovs_bridge,
  ovs_uplink: form.ovs_uplink,
  ovs_dhcp_start: form.ovs_dhcp_start,
  ovs_dhcp_end: form.ovs_dhcp_end,
  subnet_prefix: form.subnet_prefix,
  auto_port_start: form.auto_port_start,
  auto_port_end: form.auto_port_end,
  host_ip: form.host_ip,
  external_nic: form.external_nic,
  max_burst_inbound: form.max_burst_inbound,
  max_burst_outbound: form.max_burst_outbound,
  default_disk_iops_total: form.default_disk_iops_total,
  default_disk_iops_read: form.default_disk_iops_read,
  default_disk_iops_write: form.default_disk_iops_write,
  dynamic_memory_scheduler_enabled: form.dynamic_memory_scheduler_enabled,
  dynamic_memory_interval_seconds: form.dynamic_memory_interval_seconds,
  dynamic_memory_host_reserve_mb: form.dynamic_memory_host_reserve_mb,
  dynamic_memory_host_reserve_percent: form.dynamic_memory_host_reserve_percent,
  dynamic_memory_increase_threshold_percent: form.dynamic_memory_increase_threshold_percent,
  dynamic_memory_reclaim_threshold_percent: form.dynamic_memory_reclaim_threshold_percent,
  dynamic_memory_cooldown_seconds: form.dynamic_memory_cooldown_seconds,
  dynamic_memory_observation_hours: form.dynamic_memory_observation_hours,
  scheduler_event_retention_hours: form.scheduler_event_retention_hours,
  port_forward_http_probe_enabled: form.port_forward_http_probe_enabled,
  port_forward_http_probe_interval_minutes: form.port_forward_http_probe_interval_minutes,
  port_forward_http_probe_timeout_seconds: form.port_forward_http_probe_timeout_seconds,
  rescue_iso: form.rescue_iso,
  public_base_url: form.public_base_url,
  site_title: form.site_title?.trim() || 'QVMConsole',
  development_mode: form.development_mode,
  maintenance_mode: form.maintenance_mode,
  maintenance_service_units: form.maintenance_service_units?.trim() || defaultMaintenanceServiceUnits,
  maintenance_vm_shutdown_timeout_seconds: form.maintenance_vm_shutdown_timeout_seconds,
  smtp_host: form.smtp_host,
  smtp_port: form.smtp_port,
  smtp_username: form.smtp_username,
  smtp_password: form.smtp_password,
  smtp_from_name: form.smtp_from_name,
  smtp_from_address: form.smtp_from_address,
  smtp_security: form.smtp_security,
  smtp_timeout_seconds: form.smtp_timeout_seconds,
  jwt_secret_rotate_hours: form.jwt_secret_rotate_hours
})

const handleTestSMTP = async () => {
  if (!form.smtp_test_email) {
    ElMessage.warning('请输入测试收件邮箱')
    return
  }
  testing.value = true
  try {
    await updateSettings(buildPayload())
    await testSMTP({ email: form.smtp_test_email })
    ElMessage.success('测试邮件已发送，请检查收件箱')
    await fetchData()
  } catch (err) {
    console.error(err)
  } finally {
    testing.value = false
  }
}

const rotatingJWT = ref(false)

const handleRotateJWT = async () => {
  try {
    await ElMessageBox.confirm(
      '轮换 JWT 密钥后所有用户 Token 将立即失效，需要重新登录。确定继续吗？',
      '轮换 JWT 密钥',
      {
        confirmButtonText: '确定轮换',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }
  rotatingJWT.value = true
  try {
    const res = await rotateJWTSecret({})
    ElMessage.success(res.message || 'JWT 密钥轮换成功')
    await fetchData()
  } catch (err) {
    console.error(err)
  } finally {
    rotatingJWT.value = false
  }
}

const loadAffinityPresets = (showMessage = false) => {
  getCPUAffinityPresets().then(res => {
    if (res.code === 200) {
      affinityPresets.value = (res.data || []).map(p => ({ ...p }))
      if (showMessage) ElMessage.success('预设已重置')
    }
  }).catch(() => {})
}

const saveAffinityPresets = async () => {
  affinityPresetsSaving.value = true
  try {
    const payload = { presets: affinityPresets.value.filter(p => p.name.trim() && p.value.trim()) }
    const res = await saveCPUAffinityPresets(payload)
    ElMessage.success(res.message || '预设已保存')
    await loadAffinityPresets()
  } catch (err) {
    console.error(err)
  } finally {
    affinityPresetsSaving.value = false
  }
}

onMounted(fetchData)
</script>

<style scoped>
.settings-container { padding: 10px; }
h2 { margin: 0 0 8px 0; font-size: 18px; color: var(--el-text-color-primary); }
.form-tip {
  display: flex; align-items: center; gap: 4px;
  margin-top: 4px; font-size: 12px; color: #909399;
}
.kvm-compat-field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
  width: 100%;
}
.host-setting-field {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
}
.host-setting-row,
.kvm-compat-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.ksm-profile-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  width: 100%;
}
.ksm-profile-item {
  min-height: 76px;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
  padding: 10px 12px;
  background: var(--el-fill-color-blank);
}
.ksm-profile-item.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}
.ksm-profile-item strong {
  display: block;
  margin-bottom: 4px;
  color: var(--el-text-color-primary);
}
.ksm-profile-item span {
  display: block;
  color: #606266;
  font-size: 12px;
  line-height: 1.5;
}
.settings-detail-content {
  color: #606266;
  font-size: 14px;
  line-height: 1.7;
}
.settings-detail-content p {
  margin: 0 0 10px;
}

@media (max-width: 768px) {
  .settings-container {
    padding: 4px;
  }

  .settings-container h2 {
    font-size: 16px;
  }

  .settings-container :deep(.el-form-item__label) {
    width: 100px !important;
    font-size: 13px;
  }

  .settings-container :deep(.el-form) {
    max-width: 100% !important;
  }
}
</style>
