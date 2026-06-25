package vm_xml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"kvm_console/service/arch"
	"kvm_console/utils"
)

const (
	VMBootTypeBIOS       = "bios"
	VMBootTypeUEFI       = "uefi"
	VMBootTypeUEFISecure = "uefi-secure"
)

var (
	vmBootTypeOSBlockRegexp       = regexp.MustCompile(`(?s)<os\b[^>]*>.*?</os>`)
	vmBootTypeOSOpenTagRegexp     = regexp.MustCompile(`(?m)^(\s*)<os\b([^>]*)>`)
	vmBootTypeFirmwareAttrRegexp  = regexp.MustCompile(`\s+firmware=['"][^'"]+['"]`)
	vmBootTypeFirmwareBlockRegexp = regexp.MustCompile(`(?s)\n?\s*<firmware\b[^>]*>.*?</firmware>`)
	vmBootTypeLoaderBlockRegexp   = regexp.MustCompile(`(?s)\n?\s*<loader\b[^>]*(?:/>|>.*?</loader>)`)
	vmBootTypeNVRAMBlockRegexp    = regexp.MustCompile(`(?s)\n?\s*<nvram\b[^>]*(?:/>|>.*?</nvram>)`)
	vmBootTypeSecureAttrRegexp    = regexp.MustCompile(`\s+secure=['"][^'"]+['"]`)
	vmBootTypeSecureFeatureRegexp = regexp.MustCompile(`(?is)<feature\b[^>]*name=['"]secure-boot['"][^>]*enabled=['"]yes['"][^>]*/?>|<feature\b[^>]*enabled=['"]yes['"][^>]*name=['"]secure-boot['"][^>]*/?>`)
	vmBootTypeArchRegexp          = regexp.MustCompile(`<type\b[^>]*\barch=['"]([^'"]+)['"]`)
	vmBootTypeMachineRegexp       = regexp.MustCompile(`<type\b[^>]*\bmachine=['"]([^'"]+)['"]`)
	vmBootTypeSMMRegexp           = regexp.MustCompile(`(?s)\n?\s*<smm\b[^>]*/>`)
	vmBootTypeFeaturesRegexp      = regexp.MustCompile(`(?s)<features\b[^>]*>.*?</features>`)
	vmBootTypeTypeCloseRegexp     = regexp.MustCompile(`</type>`)
)

// NormalizeVMBootType 规范化引导方式。
func NormalizeVMBootType(bootType string) string {
	switch strings.ToLower(strings.TrimSpace(bootType)) {
	case VMBootTypeBIOS:
		return VMBootTypeBIOS
	case VMBootTypeUEFI:
		return VMBootTypeUEFI
	case VMBootTypeUEFISecure:
		return VMBootTypeUEFISecure
	default:
		return ""
	}
}

// ParseVMBootTypeFromDomainXML 从 domain XML 中解析当前引导方式。
// 支持两种 UEFI 标识：firmware='efi' 自动选择（旧模式）和显式 pflash loader（新模式）。
func ParseVMBootTypeFromDomainXML(xmlContent string) string {
	xmlContent = strings.TrimSpace(xmlContent)
	if xmlContent == "" {
		return ""
	}
	isUEFI := strings.Contains(xmlContent, "firmware='efi'") ||
		strings.Contains(xmlContent, `firmware="efi"`) ||
		DomainUsesPflashNVRAM(xmlContent)
	if !isUEFI {
		return VMBootTypeBIOS
	}
	if vmBootTypeSecureFeatureRegexp.MatchString(xmlContent) || vmBootTypeSecureAttrRegexp.MatchString(xmlContent) {
		return VMBootTypeUEFISecure
	}
	return VMBootTypeUEFI
}

// ParseVMArchFromDomainXML 从 domain XML 中解析架构。
func ParseVMArchFromDomainXML(xmlContent string) string {
	matches := vmBootTypeArchRegexp.FindStringSubmatch(xmlContent)
	if len(matches) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(matches[1]))
}

// ParseVMMachineTypeFromDomainXML 从 domain XML 中解析并归一化机器类型。
func ParseVMMachineTypeFromDomainXML(xmlContent string) string {
	matches := vmBootTypeMachineRegexp.FindStringSubmatch(xmlContent)
	if len(matches) < 2 {
		return ""
	}
	return normalizeVMMachineType(matches[1])
}

func normalizeVMMachineType(machine string) string {
	value := strings.ToLower(strings.TrimSpace(machine))
	switch {
	case strings.Contains(value, "q35"):
		return "q35"
	case strings.Contains(value, "i440fx"):
		return "i440fx"
	case strings.HasPrefix(value, "virt"):
		return "virt"
	default:
		return value
	}
}

func resolveVMNVRAMPath(name, xmlContent string) string {
	if path := strings.TrimSpace(ExtractDomainNVRAMPath(xmlContent)); path != "" {
		return path
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = "vm"
	}
	return fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", cleanName)
}

// ResolveOVMFLoaderPath 根据是否启用安全引导选择相应的 OVMF Code 固件路径。
func ResolveOVMFLoaderPath(secure bool) string {
	candidates := []string{
		"/usr/share/OVMF/OVMF_CODE_4M.fd",
		"/usr/share/OVMF/OVMF_CODE.fd",
	}
	fallback := "/usr/share/OVMF/OVMF_CODE_4M.fd"
	if secure {
		candidates = []string{
			"/usr/share/OVMF/OVMF_CODE_4M.ms.fd",
			"/usr/share/OVMF/OVMF_CODE_4M.secboot.fd",
			"/usr/share/OVMF/OVMF_CODE.secboot.fd",
		}
		fallback = "/usr/share/OVMF/OVMF_CODE_4M.ms.fd"
	}
	return pickFirstExistingPath(candidates, fallback)
}

func ResolveOVMFVarsTemplatePath(secure bool) string {
	candidates := []string{
		"/usr/share/OVMF/OVMF_VARS_4M.fd",
		"/usr/share/OVMF/OVMF_VARS.fd",
	}
	fallback := "/usr/share/OVMF/OVMF_VARS_4M.fd"
	if secure {
		candidates = []string{
			"/usr/share/OVMF/OVMF_VARS_4M.ms.fd",
			"/usr/share/OVMF/OVMF_VARS.ms.fd",
			"/usr/share/OVMF/OVMF_VARS_4M.secboot.fd",
		}
		fallback = "/usr/share/OVMF/OVMF_VARS_4M.ms.fd"
	}
	return pickFirstExistingPath(candidates, fallback)
}

func pickFirstExistingPath(candidates []string, fallback string) string {
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return fallback
}

func replaceOSOpenTagFirmware(osBlock string, useEFI bool) string {
	return vmBootTypeOSOpenTagRegexp.ReplaceAllStringFunc(osBlock, func(tag string) string {
		matches := vmBootTypeOSOpenTagRegexp.FindStringSubmatch(tag)
		if len(matches) < 3 {
			return tag
		}
		indent := matches[1]
		attrs := vmBootTypeFirmwareAttrRegexp.ReplaceAllString(matches[2], "")
		attrs = strings.TrimSpace(attrs)
		if useEFI {
			if attrs == "" {
				attrs = "firmware='efi'"
			} else {
				attrs += " firmware='efi'"
			}
		}
		if attrs == "" {
			return indent + "<os>"
		}
		return indent + "<os " + attrs + ">"
	})
}

// buildUEFIFirmwareFeatureXML 生成 UEFI 固件特性块（仅 <firmware> 特性声明）。
// 注意：此函数仅在使用 firmware='efi' 自动选择模式时有意义（已废弃）。
// 当前推荐方案为显式 loader/nvram，通过 loader 的 secure='yes' 属性来启用安全引导。
func buildUEFIFirmwareFeatureXML(secure bool) string {
	if !secure {
		return ""
	}
	return `    <firmware>
      <feature enabled='yes' name='enrolled-keys'/>
      <feature enabled='yes' name='secure-boot'/>
    </firmware>`
}

// buildUEFILoaderNVRAMXML 生成显式 loader 和 nvram 元素（不使用 firmware='efi' 自动选择时使用）。
func buildUEFILoaderNVRAMXML(secure bool, loaderPath, varsTemplate, nvramPath string) string {
	loaderAttrs := " readonly='yes' type='pflash'"
	if secure {
		loaderAttrs = " readonly='yes' secure='yes' type='pflash'"
	}
	return fmt.Sprintf("    <loader%s>%s</loader>\n    <nvram template='%s' templateFormat='raw' format='qcow2'>%s</nvram>",
		loaderAttrs, loaderPath, varsTemplate, nvramPath)
}

func insertUEFIFirmwareXML(osBlock, firmwareXML string) string {
	if strings.TrimSpace(firmwareXML) == "" {
		return osBlock
	}
	if vmBootTypeTypeCloseRegexp.MatchString(osBlock) {
		return vmBootTypeTypeCloseRegexp.ReplaceAllString(osBlock, "</type>\n"+firmwareXML)
	}
	if strings.Contains(osBlock, "</os>") {
		return strings.Replace(osBlock, "</os>", firmwareXML+"\n  </os>", 1)
	}
	return osBlock
}

func ensureVMSecureBootSMM(xmlContent string) string {
	if vmBootTypeSMMRegexp.MatchString(xmlContent) {
		return vmBootTypeSMMRegexp.ReplaceAllStringFunc(xmlContent, func(node string) string {
			if strings.Contains(node, "state='on'") || strings.Contains(node, `state="on"`) {
				return node
			}
			node = strings.ReplaceAll(node, `state="off"`, `state="on"`)
			node = strings.ReplaceAll(node, `state='off'`, `state="on"`)
			if !strings.Contains(node, "state='") && !strings.Contains(node, `state="`) {
				node = strings.Replace(node, "/>", " state='on'/>", 1)
			}
			return node
		})
	}
	if vmBootTypeFeaturesRegexp.MatchString(xmlContent) {
		return vmBootTypeFeaturesRegexp.ReplaceAllStringFunc(xmlContent, func(block string) string {
			return strings.Replace(block, "</features>", "    <smm state='on'/>\n  </features>", 1)
		})
	}
	featuresXML := "  <features>\n    <smm state='on'/>\n  </features>\n"
	switch {
	case strings.Contains(xmlContent, "<clock "):
		return strings.Replace(xmlContent, "<clock ", featuresXML+"  <clock ", 1)
	case strings.Contains(xmlContent, "<clock>"):
		return strings.Replace(xmlContent, "<clock>", featuresXML+"  <clock>", 1)
	case strings.Contains(xmlContent, "<devices/>"):
		return strings.Replace(xmlContent, "<devices/>", featuresXML+"  <devices/>", 1)
	case strings.Contains(xmlContent, "<devices />"):
		return strings.Replace(xmlContent, "<devices />", featuresXML+"  <devices />", 1)
	case strings.Contains(xmlContent, "<devices>"):
		return strings.Replace(xmlContent, "<devices>", featuresXML+"  <devices>", 1)
	case strings.Contains(xmlContent, "<on_poweroff>"):
		return strings.Replace(xmlContent, "<on_poweroff>", featuresXML+"  <on_poweroff>", 1)
	default:
		return xmlContent
	}
}

// ApplyVMBootTypeToDomainXML 将引导方式写入 domain XML。
// 使用显式 <loader> + <nvram format='qcow2'> 模式，不使用 firmware='efi' 自动选择，
// 以避免 libvirt 自动填充 nvram format='raw' 导致与 qcow2 实际格式不匹配（黑屏），
// 同时避免不同环境缺少 firmware descriptor 导致 "Unable to find 'efi' firmware" 错误。
func ApplyVMBootTypeToDomainXML(name, xmlContent, bootType string) (string, error) {
	normalized := NormalizeVMBootType(bootType)
	if normalized == "" {
		return "", fmt.Errorf("不支持的引导方式: %s", bootType)
	}

	vmArch := ParseVMArchFromDomainXML(xmlContent)
	machineType := ParseVMMachineTypeFromDomainXML(xmlContent)
	if normalized == VMBootTypeBIOS && vmArch == "aarch64" {
		return "", fmt.Errorf("ARM 架构虚拟机不支持 BIOS 引导")
	}
	if normalized == VMBootTypeUEFISecure {
		if vmArch == "aarch64" || vmArch == "riscv64" {
			return "", fmt.Errorf("当前架构暂不支持 UEFI 安全引导")
		}
		if machineType == "i440fx" {
			return "", fmt.Errorf("i440fx 机型不支持 UEFI 安全引导")
		}
	}

	osBlock := vmBootTypeOSBlockRegexp.FindString(xmlContent)
	if strings.TrimSpace(osBlock) == "" {
		return "", fmt.Errorf("未找到虚拟机的 <os> 配置段")
	}

	// 清除所有 UEFI 相关元素：firmware 属性、firmware 特性块、loader、nvram
	cleanedOS := vmBootTypeFirmwareBlockRegexp.ReplaceAllString(osBlock, "")
	cleanedOS = vmBootTypeLoaderBlockRegexp.ReplaceAllString(cleanedOS, "")
	cleanedOS = vmBootTypeNVRAMBlockRegexp.ReplaceAllString(cleanedOS, "")
	// 始终移除 firmware='efi' 属性，改用显式 loader/nvram
	cleanedOS = replaceOSOpenTagFirmware(cleanedOS, false)

	if normalized != VMBootTypeBIOS {
		secure := normalized == VMBootTypeUEFISecure
		if vmArch == "" {
			vmArch = "x86_64"
		}
		profile := arch.GetProfile(vmArch)
		loaderPath := profile.UEFIFirmwarePath(secure)
		varsTemplate := profile.UEFIVarsTemplatePath(secure)
		nvramPath := resolveVMNVRAMPath(name, xmlContent)
		loaderNVRAMXML := buildUEFILoaderNVRAMXML(secure, loaderPath, varsTemplate, nvramPath)
		cleanedOS = insertUEFIFirmwareXML(cleanedOS, loaderNVRAMXML)
	}

	updated := strings.Replace(xmlContent, osBlock, cleanedOS, 1)
	if normalized == VMBootTypeUEFISecure {
		updated = ensureVMSecureBootSMM(updated)
	}
	return updated, nil
}

// EnsureVMUEFINVRAMFile 确保 UEFI NVRAM 文件存在且格式正确。
func EnsureVMUEFINVRAMFile(name, xmlContent, bootType string) error {
	normalized := NormalizeVMBootType(bootType)
	if normalized != VMBootTypeUEFI && normalized != VMBootTypeUEFISecure {
		return nil
	}

	nvramPath := resolveVMNVRAMPath(name, xmlContent)
	if nvramPath == "" {
		return fmt.Errorf("未找到可用的 UEFI NVRAM 路径")
	}
	if _, err := os.Stat(nvramPath); err == nil {
		if DetectQemuImageFormat(nvramPath) == "qcow2" {
			return nil
		}
		return ConvertExistingNVRAMToQCOW2(nvramPath)
	}

	vmArch := ParseVMArchFromDomainXML(xmlContent)
	if vmArch == "" {
		vmArch = "x86_64"
	}
	profile := arch.GetProfile(vmArch)
	templatePath := profile.UEFIVarsTemplatePath(normalized == VMBootTypeUEFISecure)
	if err := CreateQCOW2NVRAMFromTemplate(templatePath, nvramPath); err != nil {
		return fmt.Errorf("创建 UEFI NVRAM 文件失败: %w", err)
	}
	return nil
}

func DetectQemuImageFormat(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	result := utils.ExecCommand("qemu-img", "info", "-U", "--output=json", path)
	if result.Error != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parseQemuInfoStr(result.Stdout, "format")))
}

func CreateQCOW2NVRAMFromTemplate(templatePath, nvramPath string) error {
	templatePath = strings.TrimSpace(templatePath)
	nvramPath = strings.TrimSpace(nvramPath)
	if templatePath == "" || nvramPath == "" {
		return fmt.Errorf("NVRAM 模板路径或目标路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(nvramPath), 0755); err != nil {
		return fmt.Errorf("创建 UEFI NVRAM 目录失败: %w", err)
	}
	sourceFormat := DetectQemuImageFormat(templatePath)
	if sourceFormat == "" {
		sourceFormat = "raw"
	}
	_ = os.Remove(nvramPath)
	result := utils.ExecCommand("qemu-img", "convert", "-f", sourceFormat, "-O", "qcow2", templatePath, nvramPath)
	if result.Error != nil {
		return fmt.Errorf("转换 NVRAM 为 qcow2 失败: %s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := os.Chmod(nvramPath, 0600); err != nil {
		return fmt.Errorf("设置 NVRAM 文件权限失败: %w", err)
	}
	if err := utils.ChownLibvirtQEMU(nvramPath); err != nil {
		return fmt.Errorf("设置 NVRAM 文件权限失败: %w", err)
	}
	return nil
}

func ConvertExistingNVRAMToQCOW2(nvramPath string) error {
	nvramPath = strings.TrimSpace(nvramPath)
	if nvramPath == "" {
		return fmt.Errorf("NVRAM 路径为空")
	}
	if DetectQemuImageFormat(nvramPath) == "qcow2" {
		return nil
	}
	tmpPath := nvramPath + ".qcow2.tmp"
	backupPath := nvramPath + ".raw.bak"
	for i := 1; ; i++ {
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			break
		}
		backupPath = fmt.Sprintf("%s.raw.bak.%d", nvramPath, i)
	}
	_ = os.Remove(tmpPath)
	result := utils.ExecCommand("qemu-img", "convert", "-f", "raw", "-O", "qcow2", nvramPath, tmpPath)
	if result.Error != nil {
		return fmt.Errorf("转换 NVRAM 为 qcow2 失败: %s", firstNonEmpty(result.Stderr, result.Error.Error()))
	}
	if err := os.Rename(nvramPath, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("备份原 NVRAM 文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, nvramPath); err != nil {
		_ = os.Rename(backupPath, nvramPath)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("替换 NVRAM 文件失败: %w", err)
	}
	if err := os.Chmod(nvramPath, 0600); err != nil {
		return fmt.Errorf("设置 NVRAM 文件权限失败: %w", err)
	}
	if err := utils.ChownLibvirtQEMU(nvramPath); err != nil {
		return fmt.Errorf("设置 NVRAM 文件权限失败: %w", err)
	}
	return nil
}

func DomainUsesPflashNVRAM(xmlContent string) bool {
	return strings.Contains(xmlContent, "type='pflash'") ||
		strings.Contains(xmlContent, `type="pflash"`)
}

func ExtractDomainNVRAMFormat(xmlContent string) string {
	matches := regexp.MustCompile(`(?s)<nvram\b([^>]*)>`).FindStringSubmatch(xmlContent)
	if len(matches) < 2 {
		return ""
	}
	attrMatches := regexp.MustCompile(`\bformat=['"]([^'"]+)['"]`).FindStringSubmatch(matches[1])
	if len(attrMatches) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(attrMatches[1]))
}

func ExtractDomainNVRAMPath(xmlContent string) string {
	matches := regexp.MustCompile(`(?s)<nvram[^>]*>\s*([^<]+?)\s*</nvram>`).FindStringSubmatch(xmlContent)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func SetDomainNVRAMFormat(xmlContent, format string) string {
	format = strings.TrimSpace(format)
	if strings.TrimSpace(xmlContent) == "" || format == "" {
		return xmlContent
	}
	re := regexp.MustCompile(`(?s)<nvram\b([^>]*)>`)
	return re.ReplaceAllStringFunc(xmlContent, func(tag string) string {
		attrRe := regexp.MustCompile(`\bformat=['"][^'"]+['"]`)
		if attrRe.MatchString(tag) {
			return attrRe.ReplaceAllString(tag, "format='"+format+"'")
		}
		return strings.Replace(tag, "<nvram", "<nvram format='"+format+"'", 1)
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// parseQemuInfoStr 从 qemu-img info JSON 中解析字符串值（仅读取顶层字段）
func parseQemuInfoStr(output, key string) string {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return ""
	}
	raw, ok := data[key]
	if !ok {
		return ""
	}
	var val string
	if err := json.Unmarshal(raw, &val); err != nil {
		return ""
	}
	return val
}
