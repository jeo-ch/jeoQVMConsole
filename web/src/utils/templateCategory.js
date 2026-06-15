export const DEFAULT_LINUX_TEMPLATE_CATEGORY = 'Ubuntu'
export const DEFAULT_WINDOWS_TEMPLATE_CATEGORY = 'WindowsServer2022'

export const LINUX_TEMPLATE_CATEGORY_OPTIONS = [
  'Ubuntu',
  'Debian',
  'CentOS',
]

export const WINDOWS_TEMPLATE_CATEGORY_OPTIONS = [
  'WindowsServer2022',
  'Windows10',
  'WindowsServer2012R2',
  '其它',
]

export const normalizeTemplateType = (type) => (type || '').toString().trim().toLowerCase()

export const templateTypeLabel = (type) => ({
  windows: 'Windows',
  fnos: 'FnOS',
}[normalizeTemplateType(type)] || 'Linux')

export const normalizeTemplateCategory = (type, category) => {
  const normalizedType = normalizeTemplateType(type)
  if (normalizedType !== 'linux' && normalizedType !== 'windows') {
    return ''
  }
  const normalized = (category || '').toString().trim()
  const options = normalizedType === 'windows' ? WINDOWS_TEMPLATE_CATEGORY_OPTIONS : LINUX_TEMPLATE_CATEGORY_OPTIONS
  const defaultCategory = normalizedType === 'windows' ? DEFAULT_WINDOWS_TEMPLATE_CATEGORY : DEFAULT_LINUX_TEMPLATE_CATEGORY
  if (!normalized) {
    return defaultCategory
  }
  const matched = options.find(item => item.toLowerCase() === normalized.toLowerCase())
  return matched || defaultCategory
}

export const templateCategoryLabel = (type, category) => {
  const normalizedType = normalizeTemplateType(type)
  if (normalizedType !== 'linux' && normalizedType !== 'windows') {
    return ''
  }
  return normalizeTemplateCategory(normalizedType, category)
}

export const templateGroupLabel = (type, category) => {
  const typeLabel = templateTypeLabel(type)
  const categoryLabel = templateCategoryLabel(type, category)
  return categoryLabel ? `${typeLabel} / ${categoryLabel}` : typeLabel
}
