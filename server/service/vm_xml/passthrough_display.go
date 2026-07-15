package vm_xml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const primaryGPUAlias = "ua-qvm-primary-gpu"

var (
	pciHostdevBlockRegexp    = regexp.MustCompile(`(?s)<hostdev\b[^>]*\btype=['"]pci['"][^>]*>.*?</hostdev>`)
	pciHostdevSourceRegexp   = regexp.MustCompile(`(?s)<source\b[^>]*>.*?</source>`)
	pciHostdevAddressRegexp  = regexp.MustCompile(`<address\b[^>]*/>`)
	pciHostdevAliasRegexp    = regexp.MustCompile(`<alias\b[^>]*\bname=['"]([^'"]+)['"][^>]*/>`)
	qemuDeviceOverrideRegexp = regexp.MustCompile(`(?s)\s*<qemu:device\b[^>]*>.*?</qemu:device>`)
	qemuXVGAPropertyRegexp   = regexp.MustCompile(`\s*<qemu:property\b[^>]*\bname=['"]x-vga['"][^>]*/>`)
	qemuAnyPropertyRegexp    = regexp.MustCompile(`<qemu:property\b`)
	qemuEmptyFrontendRegexp  = regexp.MustCompile(`(?s)\s*<qemu:frontend>\s*</qemu:frontend>`)
	qemuOverrideBlockRegexp  = regexp.MustCompile(`(?s)<qemu:override>.*?</qemu:override>`)
	qemuEmptyOverrideRegexp  = regexp.MustCompile(`(?s)\s*<qemu:override>\s*</qemu:override>`)
	domainOpenTagRegexp      = regexp.MustCompile(`<domain\b[^>]*>`)

	pciDomainAttrRegexp   = regexp.MustCompile(`\bdomain=['"](?:0x)?([0-9a-fA-F]+)['"]`)
	pciBusAttrRegexp      = regexp.MustCompile(`\bbus=['"](?:0x)?([0-9a-fA-F]+)['"]`)
	pciSlotAttrRegexp     = regexp.MustCompile(`\bslot=['"](?:0x)?([0-9a-fA-F]+)['"]`)
	pciFunctionAttrRegexp = regexp.MustCompile(`\bfunction=['"](?:0x)?([0-9a-fA-F]+)['"]`)
)

// ParsePCIHostDeviceAddresses 从 domain XML 中读取 PCI hostdev 的宿主机地址。
// 返回值统一为 0000:00:00.0 格式，忽略来宾侧自动分配的 PCI 地址。
func ParsePCIHostDeviceAddresses(xmlStr string) []string {
	blocks := pciHostdevBlockRegexp.FindAllString(xmlStr, -1)
	addresses := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if address, ok := parsePCIHostDeviceSourceAddress(block); ok {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func parsePCIHostDeviceSourceAddress(block string) (string, bool) {
	source := pciHostdevSourceRegexp.FindString(block)
	if source == "" {
		return "", false
	}
	addressTag := pciHostdevAddressRegexp.FindString(source)
	if addressTag == "" {
		return "", false
	}

	domain, ok := parsePCIHexAttribute(addressTag, pciDomainAttrRegexp)
	if !ok {
		return "", false
	}
	bus, ok := parsePCIHexAttribute(addressTag, pciBusAttrRegexp)
	if !ok {
		return "", false
	}
	slot, ok := parsePCIHexAttribute(addressTag, pciSlotAttrRegexp)
	if !ok {
		return "", false
	}
	function, ok := parsePCIHexAttribute(addressTag, pciFunctionAttrRegexp)
	if !ok {
		return "", false
	}

	return fmt.Sprintf("%04x:%02x:%02x.%x", domain, bus, slot, function), true
}

func parsePCIHexAttribute(tag string, expr *regexp.Regexp) (uint64, bool) {
	matches := expr.FindStringSubmatch(tag)
	if len(matches) < 2 {
		return 0, false
	}
	value, err := strconv.ParseUint(matches[1], 16, 16)
	return value, err == nil
}

func normalizePCIAddress(address string) (string, error) {
	parts := regexp.MustCompile(`(?i)^([0-9a-f]{4}):([0-9a-f]{2}):([0-9a-f]{2})\.([0-7])$`).FindStringSubmatch(strings.TrimSpace(address))
	if len(parts) != 5 {
		return "", fmt.Errorf("无效的 PCI 地址格式: %s", address)
	}
	return strings.ToLower(strings.Join(parts[1:3], ":") + ":" + parts[3] + "." + parts[4]), nil
}

// ApplyPrimaryGPUXVGAToDomainXML 管理由 none 显示模型触发的直通主显卡配置。
// primaryPCIAddress 为空时仅清理由本功能管理的 x-vga 配置；非空时为对应 hostdev
// 注入稳定 alias，并通过 qemu:override 设置 x-vga=true。
func ApplyPrimaryGPUXVGAToDomainXML(xmlStr, primaryPCIAddress string) (string, error) {
	normalizedAddress := ""
	if strings.TrimSpace(primaryPCIAddress) != "" {
		var err error
		normalizedAddress, err = normalizePCIAddress(primaryPCIAddress)
		if err != nil {
			return "", err
		}
	}

	updated := removeXVGAOverrides(xmlStr)
	selectedAlias := ""
	selectedFound := false
	updated = pciHostdevBlockRegexp.ReplaceAllStringFunc(updated, func(block string) string {
		address, ok := parsePCIHostDeviceSourceAddress(block)
		if !ok {
			return block
		}

		aliasMatches := pciHostdevAliasRegexp.FindStringSubmatch(block)
		if address != normalizedAddress {
			if len(aliasMatches) >= 2 && aliasMatches[1] == primaryGPUAlias {
				return pciHostdevAliasRegexp.ReplaceAllString(block, "")
			}
			return block
		}

		selectedFound = true
		if len(aliasMatches) >= 2 {
			selectedAlias = aliasMatches[1]
			return block
		}
		selectedAlias = primaryGPUAlias
		return strings.Replace(block, "</hostdev>", "  <alias name='"+primaryGPUAlias+"'/>\n</hostdev>", 1)
	})

	if normalizedAddress == "" {
		return updated, nil
	}
	if !selectedFound || selectedAlias == "" {
		return "", fmt.Errorf("未在虚拟机 XML 中找到直通显示设备 %s", normalizedAddress)
	}

	if !strings.Contains(updated, "xmlns:qemu=") {
		updated = domainOpenTagRegexp.ReplaceAllStringFunc(updated, func(tag string) string {
			return strings.TrimSuffix(tag, ">") + " xmlns:qemu='http://libvirt.org/schemas/domain/qemu/1.0'>"
		})
	}

	deviceOverride := "    <qemu:device alias='" + selectedAlias + "'>\n" +
		"      <qemu:frontend>\n" +
		"        <qemu:property name='x-vga' type='bool' value='true'/>\n" +
		"      </qemu:frontend>\n" +
		"    </qemu:device>"
	if qemuOverrideBlockRegexp.MatchString(updated) {
		updated = qemuOverrideBlockRegexp.ReplaceAllStringFunc(updated, func(block string) string {
			return strings.Replace(block, "</qemu:override>", deviceOverride+"\n  </qemu:override>", 1)
		})
		return updated, nil
	}

	override := "  <qemu:override>\n" + deviceOverride + "\n  </qemu:override>\n"
	if !strings.Contains(updated, "</domain>") {
		return "", fmt.Errorf("写入主显卡配置失败：未找到 domain 结束标签")
	}
	return strings.Replace(updated, "</domain>", override+"</domain>", 1), nil
}

func removeXVGAOverrides(xmlStr string) string {
	updated := qemuDeviceOverrideRegexp.ReplaceAllStringFunc(xmlStr, func(block string) string {
		if !qemuXVGAPropertyRegexp.MatchString(block) {
			return block
		}
		cleaned := qemuXVGAPropertyRegexp.ReplaceAllString(block, "")
		cleaned = qemuEmptyFrontendRegexp.ReplaceAllString(cleaned, "")
		if !qemuAnyPropertyRegexp.MatchString(cleaned) && !strings.Contains(cleaned, "<qemu:backend") {
			return ""
		}
		return cleaned
	})
	return qemuEmptyOverrideRegexp.ReplaceAllString(updated, "")
}
