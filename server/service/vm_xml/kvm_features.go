package vm_xml

import (
	"fmt"
	"regexp"
	"strings"
)

// KVMFeatureConfig KVM 虚拟化特性配置
type KVMFeatureConfig struct {
	KVMHidden bool   `json:"kvm_hidden"` // 隐藏 KVM 标志
	VendorID  string `json:"vendor_id"`  // Hyper-V vendor_id 伪装（空字符串表示不设置）
}

var (
	// <kvm><hidden state='on'/></kvm>
	vmKVMBlockRegexp  = regexp.MustCompile(`(?s)<kvm\b[^>]*>.*?</kvm>`)
	vmKVMHiddenRegexp = regexp.MustCompile(`<hidden\b[^>]*\bstate=['"]on['"][^>]*/>`)

	// <vendor_id state="on" value="..."/>
	vmVendorIDRegexp       = regexp.MustCompile(`<vendor_id\b[^/]*/>`)
	vmFeaturesBlockExprKVM = regexp.MustCompile(`(?s)<features\b[^>]*>.*?</features>`)
)

// HasKVMFeatureValue 判断 KVM 特性配置是否有实际值
func (cfg *KVMFeatureConfig) HasValue() bool {
	if cfg == nil {
		return false
	}
	return cfg.KVMHidden || strings.TrimSpace(cfg.VendorID) != ""
}

// ParseKVMHiddenFromDomainXML 从 domain XML 解析 KVM 隐藏标志是否启用
func ParseKVMHiddenFromDomainXML(xmlStr string) bool {
	return vmKVMHiddenRegexp.MatchString(xmlStr)
}

// ParseVendorIDFromDomainXML 从 domain XML 解析 Hyper-V vendor_id 伪装值
// 返回空字符串表示未设置 vendor_id
func ParseVendorIDFromDomainXML(xmlStr string) string {
	matches := vmVendorIDRegexp.FindString(xmlStr)
	if matches == "" {
		return ""
	}
	// 提取 value 属性
	valRe := regexp.MustCompile(`value=['"]([^'"]*)['"]`)
	valMatches := valRe.FindStringSubmatch(matches)
	if len(valMatches) < 2 {
		return ""
	}
	return strings.TrimSpace(valMatches[1])
}

// renderKVMHiddenBlock 生成 <kvm><hidden state='on'/></kvm> XML 块
func renderKVMHiddenBlock() string {
	return "    <kvm>\n      <hidden state='on'/>\n    </kvm>"
}

// renderVendorIDBlock 生成 <vendor_id state="on" value="..."/> XML
func renderVendorIDBlock(vendorID string) string {
	return fmt.Sprintf("      <vendor_id state='on' value='%s'/>", vendorID)
}

// ApplyKVMHiddenToDomainXML 向 domain XML 的 <features> 中注入或移除 KVM 隐藏标志
// 传入 nil 表示不修改，true/false 分别表示启用/关闭
func ApplyKVMHiddenToDomainXML(xmlStr string, enabled *bool) (string, error) {
	if enabled == nil {
		return xmlStr, nil
	}

	// 先移除现有的 kvm 块
	updated := vmKVMBlockRegexp.ReplaceAllString(xmlStr, "")

	if !*enabled {
		return updated, nil
	}

	// 注入 <kvm><hidden state='on'/></kvm>
	block := renderKVMHiddenBlock()
	if vmFeaturesBlockExprKVM.MatchString(updated) {
		return vmFeaturesBlockExprKVM.ReplaceAllStringFunc(updated, func(featuresBlock string) string {
			return strings.Replace(featuresBlock, "</features>", block+"\n  </features>", 1)
		}), nil
	}

	// 没有 <features> 块的兜底逻辑
	featuresXML := "  <features>\n" + block + "\n  </features>\n"
	switch {
	case strings.Contains(updated, "<clock "):
		return strings.Replace(updated, "<clock ", featuresXML+"  <clock ", 1), nil
	case strings.Contains(updated, "<clock>"):
		return strings.Replace(updated, "<clock>", featuresXML+"  <clock>", 1), nil
	case strings.Contains(updated, "<devices/>"):
		return strings.Replace(updated, "<devices/>", featuresXML+"  <devices/>", 1), nil
	case strings.Contains(updated, "<devices />"):
		return strings.Replace(updated, "<devices />", featuresXML+"  <devices />", 1), nil
	case strings.Contains(updated, "<devices>"):
		return strings.Replace(updated, "<devices>", featuresXML+"  <devices>", 1), nil
	case strings.Contains(updated, "<on_poweroff>"):
		return strings.Replace(updated, "<on_poweroff>", featuresXML+"  <on_poweroff>", 1), nil
	default:
		return "", fmt.Errorf("写入 KVM 隐藏标志失败：未找到可插入 features 的位置")
	}
}

// ApplyVendorIDToHyperVBlock 向 domain XML 的 <hyperv> 块中注入或移除 vendor_id
// vendorID 为空字符串时移除 vendor_id，否则设置对应的伪装值
// 仅 x86_64 架构支持 Hyper-V vendor_id
func ApplyVendorIDToHyperVBlock(xmlStr string, vendorID string) (string, error) {
	trimmed := strings.TrimSpace(vendorID)

	// 先移除现有的 vendor_id
	updated := vmVendorIDRegexp.ReplaceAllString(xmlStr, "")

	if trimmed == "" {
		return updated, nil
	}

	// 检查是否支持（非 x86_64 架构不支持 Hyper-V）
	arch := ParseVMArchFromDomainXML(updated)
	if arch != "" && arch != "x86_64" {
		// ARM/RISC-V 等架构不支持 Hyper-V vendor_id，直接返回
		return updated, nil
	}

	// 注入 vendor_id 到 <hyperv> 块中
	block := renderVendorIDBlock(trimmed)
	if vmHyperVBlockRegexp.MatchString(updated) {
		return vmHyperVBlockRegexp.ReplaceAllStringFunc(updated, func(hypervBlock string) string {
			return strings.Replace(hypervBlock, "</hyperv>", block+"\n    </hyperv>", 1)
		}), nil
	}

	// 没有 <hyperv> 块：检查是否有 <features>
	if vmFeaturesBlockExprKVM.MatchString(updated) {
		// 在 </features> 前插入完整的 hyperv 块
		hypervXML := "    <hyperv mode='custom'>\n" + block + "\n    </hyperv>"
		return vmFeaturesBlockExprKVM.ReplaceAllStringFunc(updated, func(featuresBlock string) string {
			return strings.Replace(featuresBlock, "</features>", hypervXML+"\n  </features>", 1)
		}), nil
	}

	// 没有 <features> 块的兜底
	hypervXML := "  <features>\n" +
		"    <hyperv mode='custom'>\n" +
		block + "\n" +
		"    </hyperv>\n" +
		"  </features>\n"
	switch {
	case strings.Contains(updated, "<clock "):
		return strings.Replace(updated, "<clock ", hypervXML+"  <clock ", 1), nil
	case strings.Contains(updated, "<clock>"):
		return strings.Replace(updated, "<clock>", hypervXML+"  <clock>", 1), nil
	case strings.Contains(updated, "<devices/>"):
		return strings.Replace(updated, "<devices/>", hypervXML+"  <devices/>", 1), nil
	case strings.Contains(updated, "<devices />"):
		return strings.Replace(updated, "<devices />", hypervXML+"  <devices />", 1), nil
	case strings.Contains(updated, "<devices>"):
		return strings.Replace(updated, "<devices>", hypervXML+"  <devices>", 1), nil
	case strings.Contains(updated, "<on_poweroff>"):
		return strings.Replace(updated, "<on_poweroff>", hypervXML+"  <on_poweroff>", 1), nil
	default:
		return "", fmt.Errorf("写入 vendor_id 失败：未找到可插入 features 的位置")
	}
}
