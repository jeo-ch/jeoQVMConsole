<template>
  <el-dialog title="制作模板" v-model="visible" width="500px" :close-on-click-modal="false" append-to-body>
    <el-form :model="form" label-width="120px">
      <el-form-item label="源虚拟机">
        <el-input :model-value="vmName" disabled />
      </el-form-item>

      <el-form-item label="模板名称" required>
        <el-input v-model="form.name" placeholder="管理员侧名称（字母、数字、点、下划线、短横线）" />
      </el-form-item>

      <el-form-item label="用户侧显示">
        <el-input v-model="form.display_name" placeholder="从模板克隆下拉框中显示的标题" />
      </el-form-item>

      <el-form-item label="模板类型">
        <el-radio-group v-model="form.type">
          <el-radio value="linux">
            <span style="display: flex; align-items: center; gap: 4px;">🐧 Linux</span>
          </el-radio>
          <el-radio value="windows">
            <span style="display: flex; align-items: center; gap: 4px;">🪟 Windows</span>
          </el-radio>
          <el-radio value="fnos">
            <span style="display: flex; align-items: center; gap: 4px;">📦 FnOS</span>
          </el-radio>
          <el-radio value="other">
            <span style="display: flex; align-items: center; gap: 4px;">💾 Other</span>
          </el-radio>
        </el-radio-group>
      </el-form-item>

      <template v-if="form.type === 'linux' || form.type === 'windows'">
        <el-form-item :label="form.type === 'windows' ? 'Windows 分类' : 'Linux 分类'">
          <el-select
            v-model="form.category"
            filterable
            :placeholder="categoryPlaceholder"
            style="width: 100%;"
          >
            <el-option
              v-for="item in categoryOptions"
              :key="item"
              :label="item"
              :value="item"
            />
          </el-select>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            {{ categoryTip }}
          </div>
        </el-form-item>
      </template>

      <template v-if="form.type === 'linux'">
        <el-divider content-position="left" style="margin: 12px 0;">
          Linux 模板配置
        </el-divider>

        <el-form-item label="初始化方式">
          <el-radio-group v-model="form.cloud_init_mode">
            <el-radio value="nocloud">
              <span>☁️ cloud-init（推荐）</span>
            </el-radio>
            <el-radio value="">
              <span>🛠️ 仅离线初始化</span>
            </el-radio>
          </el-radio-group>
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            <span v-if="form.cloud_init_mode === 'nocloud'">
              模板内需预装 cloud-init，克隆时自动扩容磁盘、设置 hostname，无需 SSH 连接
            </span>
            <span v-else>
              仅通过 virt-customize 离线设置 hostname/密码，磁盘扩容需手动处理
            </span>
          </div>
        </el-form-item>

        <el-form-item label="模板用户名">
          <el-input v-model="form.template_user" placeholder="模板中已有的普通用户名" />
          <div class="form-tip">
            <el-icon><InfoFilled /></el-icon>
            克隆时若目标用户名与模板用户名不同，自动离线重命名
          </div>
        </el-form-item>
      </template>

      <div v-if="form.type === 'other'" style="padding: 8px 12px; background: #fdf6ec; border-radius: 4px; font-size: 13px; color: #e6a23c; margin-bottom: 12px;">
        💡 Other 类型克隆时将直接完整复制模板磁盘，不做任何初始化操作
      </div>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="loading" @click="handleSubmit">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, reactive, watch } from 'vue'
import { prepareTemplate } from '@/api/vm'
import { ElMessage } from 'element-plus'
import {
  DEFAULT_LINUX_TEMPLATE_CATEGORY,
  DEFAULT_WINDOWS_TEMPLATE_CATEGORY,
  LINUX_TEMPLATE_CATEGORY_OPTIONS,
  WINDOWS_TEMPLATE_CATEGORY_OPTIONS,
  normalizeTemplateCategory,
} from '@/utils/templateCategory'

const emit = defineEmits(['success'])

const visible = ref(false)
const loading = ref(false)
const vmName = ref('')

const form = reactive({
  name: '',
  display_name: '',
  type: 'linux',
  category: DEFAULT_LINUX_TEMPLATE_CATEGORY,
  cloud_init_mode: 'nocloud',
  template_user: '',
})

const categoryOptions = computed(() => form.type === 'windows' ? WINDOWS_TEMPLATE_CATEGORY_OPTIONS : LINUX_TEMPLATE_CATEGORY_OPTIONS)
const categoryPlaceholder = computed(() => form.type === 'windows'
  ? '默认归入 WindowsServer2022，可选择 Windows10 / WindowsServer2012R2'
  : '默认归入 Ubuntu，可选择 Debian')
const categoryTip = computed(() => form.type === 'windows'
  ? 'Windows 模板按版本分类展示，2012 R2 会保留 BIOS/SATA 等默认配置用于克隆'
  : 'Debian 模板会复用 Linux SSH 初始化流程，当前仅支持 Ubuntu / Debian')

watch(() => form.type, (type) => {
  if (type === 'windows') {
    form.category = normalizeTemplateCategory('windows', form.category || DEFAULT_WINDOWS_TEMPLATE_CATEGORY)
  } else if (type === 'linux') {
    form.category = normalizeTemplateCategory('linux', form.category || DEFAULT_LINUX_TEMPLATE_CATEGORY)
  } else {
    form.category = ''
  }
})

const open = (name) => {
  vmName.value = name
  form.name = name + '-tpl'
  form.display_name = form.name
  form.type = 'linux'
  form.category = DEFAULT_LINUX_TEMPLATE_CATEGORY
  form.cloud_init_mode = 'nocloud'
  form.template_user = ''
  visible.value = true
}

const handleSubmit = async () => {
  if (form.name.includes('..')) {
    ElMessage.warning('模板名称不能包含连续的点')
    return
  }

  if (!/^[a-zA-Z0-9._-]+$/.test(form.name)) {
    ElMessage.warning('模板名称只能包含字母、数字、点、下划线和短横线')
    return
  }

  loading.value = true
  try {
    await prepareTemplate({
      vm_name: vmName.value,
      template_name: form.name,
      display_name: form.display_name || form.name,
      type: form.type,
      category: ['linux', 'windows'].includes(form.type) ? normalizeTemplateCategory(form.type, form.category) : undefined,
      cloud_init_mode: form.type === 'linux' ? (form.cloud_init_mode || undefined) : undefined,
      template_user: form.template_user || undefined,
    })
    ElMessage.success('制作模板任务已提交，请在任务中查看进度')
    visible.value = false
    emit('success')
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

defineExpose({ open })
</script>

<style scoped>
.form-tip {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 12px;
  color: #909399;
}
</style>
