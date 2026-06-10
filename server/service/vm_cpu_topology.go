package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kvm_console/utils"
)

const (
	VMCPUTopologyAuto         = "auto"
	VMCPUTopologySingleSocket = "single_socket"
	VMCPUTopologyHostDefault  = "host_default"
)

var (
	vmCPUBlockRegexp       = regexp.MustCompile(`(?s)<cpu\b[^>]*(?:/>|>.*?</cpu>)`)
	vmCPUTopologyRegexp    = regexp.MustCompile(`(?s)<topology\b[^>]*/>`)
	vmVCPUValueRegexp      = regexp.MustCompile(`(?s)<vcpu\b[^>]*>\s*([0-9]+)\s*</vcpu>`)
	vmSelfClosingCPUExpr   = regexp.MustCompile(`^<cpu\b[^>]*/>$`)
	vmTopologySocketsRegex = regexp.MustCompile(`\bsockets=['"]([0-9]+)['"]`)
	vmTopologyDiesRegex    = regexp.MustCompile(`\bdies=['"]([0-9]+)['"]`)
	vmTopologyCoresRegex   = regexp.MustCompile(`\bcores=['"]([0-9]+)['"]`)
	vmTopologyThreadsRegex = regexp.MustCompile(`\bthreads=['"]([0-9]+)['"]`)
)

// NormalizeVMCPUTopologyMode 规范化 CPU 拓扑模式。
func NormalizeVMCPUTopologyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case VMCPUTopologySingleSocket, VMCPUTopologyHostDefault:
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return VMCPUTopologyAuto
	}
}

// ApplyCPUTopologyModeToDomainXML 按模式写入 domain XML 的 CPU 拓扑。
func ApplyCPUTopologyModeToDomainXML(xmlStr, mode, osType string, vcpu int) string {
	switch NormalizeVMCPUTopologyMode(mode) {
	case VMCPUTopologySingleSocket:
		return ApplyWindowsCPUTopologyToDomainXML(xmlStr, vcpu)
	case VMCPUTopologyHostDefault:
		return RemoveCPUTopologyFromDomainXML(xmlStr)
	default:
		if strings.EqualFold(strings.TrimSpace(osType), "windows") {
			return ApplyWindowsCPUTopologyToDomainXML(xmlStr, vcpu)
		}
		return xmlStr
	}
}

// RemoveCPUTopologyFromDomainXML 移除显式 CPU 拓扑，让 libvirt/QEMU 使用默认拓扑。
func RemoveCPUTopologyFromDomainXML(xmlStr string) string {
	return vmCPUTopologyRegexp.ReplaceAllString(xmlStr, "")
}

// ParseVMCPUTopologyModeFromDomainXML 从 domain XML 中识别可回填的 CPU 拓扑模式。
func ParseVMCPUTopologyModeFromDomainXML(xmlStr string) string {
	topology := vmCPUTopologyRegexp.FindString(xmlStr)
	if strings.TrimSpace(topology) == "" {
		return VMCPUTopologyAuto
	}
	sockets := parseTopologyAttr(topology, vmTopologySocketsRegex)
	dies := parseTopologyAttr(topology, vmTopologyDiesRegex)
	cores := parseTopologyAttr(topology, vmTopologyCoresRegex)
	threads := parseTopologyAttr(topology, vmTopologyThreadsRegex)
	vcpu := ParseVCPUCountFromDomainXML(xmlStr)
	if sockets == 1 && (dies <= 0 || dies == 1) && threads == 1 && (vcpu <= 0 || cores == vcpu) {
		return VMCPUTopologySingleSocket
	}
	return VMCPUTopologyHostDefault
}

// ApplyWindowsCPUTopologyToDomainXML 将 Windows 来宾的 vCPU 暴露为单插槽多核心。
func ApplyWindowsCPUTopologyToDomainXML(xmlStr string, vcpu int) string {
	if vcpu <= 0 {
		vcpu = ParseVCPUCountFromDomainXML(xmlStr)
	}
	if vcpu <= 0 {
		return xmlStr
	}

	topology := fmt.Sprintf("<topology sockets='1' dies='1' cores='%d' threads='1'/>", vcpu)
	if vmCPUBlockRegexp.MatchString(xmlStr) {
		return vmCPUBlockRegexp.ReplaceAllStringFunc(xmlStr, func(cpuBlock string) string {
			return applyTopologyToCPUBlock(cpuBlock, topology)
		})
	}

	cpuBlock := fmt.Sprintf("  <cpu mode='host-passthrough' check='none' migratable='on'>\n    %s\n  </cpu>", topology)
	if strings.Contains(xmlStr, "</features>") {
		return strings.Replace(xmlStr, "</features>", "</features>\n"+cpuBlock, 1)
	}
	if strings.Contains(xmlStr, "<devices>") {
		return strings.Replace(xmlStr, "<devices>", cpuBlock+"\n  <devices>", 1)
	}
	return xmlStr
}

// ParseVCPUCountFromDomainXML 从 domain XML 中读取 vCPU 当前数量。
// 支持 <vcpu>N</vcpu> 和 <vcpu placement='static' current='N'>M</vcpu> 两种格式，
// 有 current 属性时返回 current 值，否则返回标签体内的值。
func ParseVCPUCountFromDomainXML(xmlStr string) int {
	// 先尝试匹配带 current 属性的格式
	currentRegex := regexp.MustCompile(`(?s)<vcpu\b[^>]*\bcurrent\s*=\s*["']([0-9]+)["'][^>]*>`)
	if matches := currentRegex.FindStringSubmatch(xmlStr); len(matches) >= 2 {
		value, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && value > 0 {
			return value
		}
	}
	// 回退：匹配标签体内的数字
	matches := vmVCPUValueRegexp.FindStringSubmatch(xmlStr)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil {
		return 0
	}
	return value
}

// ParseMaxVCPUFromDomainXML 从 domain XML 中读取 vCPU 最大值。
// <vcpu>N</vcpu> 返回 N；<vcpu current='C'>M</vcpu> 返回 M（标签体内的值）。
func ParseMaxVCPUFromDomainXML(xmlStr string) int {
	matches := vmVCPUValueRegexp.FindStringSubmatch(xmlStr)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil {
		return 0
	}
	return value
}

// BuildVCPUTag 构建 vCPU XML 标签。
// current: 当前 vCPU 数量；maxVCPU: 热添加上限。
// 当 maxVCPU <= 0 或 maxVCPU == current 时，不启用热添加，返回 <vcpu>N</vcpu>。
// 否则返回 <vcpu placement='static' current='N'>M</vcpu>。
func BuildVCPUTag(current, maxVCPU int) string {
	if maxVCPU <= 0 || maxVCPU <= current {
		return fmt.Sprintf("  <vcpu>%d</vcpu>", current)
	}
	return fmt.Sprintf("  <vcpu placement='static' current='%d'>%d</vcpu>", current, maxVCPU)
}

// EffectiveTopologyVCPU 返回用于拓扑计算的 vCPU 数量（使用最大值以确保热添加后拓扑仍然匹配）。
func EffectiveTopologyVCPU(current, maxVCPU int) int {
	if maxVCPU > current {
		return maxVCPU
	}
	return current
}

func applyTopologyToCPUBlock(cpuBlock, topology string) string {
	trimmed := strings.TrimSpace(cpuBlock)
	if vmSelfClosingCPUExpr.MatchString(trimmed) {
		openTag := strings.TrimSuffix(trimmed, "/>")
		openTag = strings.TrimRight(openTag, " ")
		indent := leadingWhitespace(cpuBlock)
		return fmt.Sprintf("%s>\n%s  %s\n%s</cpu>", openTag, indent, topology, indent)
	}

	if vmCPUTopologyRegexp.MatchString(cpuBlock) {
		return vmCPUTopologyRegexp.ReplaceAllString(cpuBlock, topology)
	}
	if strings.Contains(cpuBlock, "</cpu>") {
		indent := leadingWhitespace(cpuBlock)
		return strings.Replace(cpuBlock, "</cpu>", "  "+topology+"\n"+indent+"</cpu>", 1)
	}
	return cpuBlock
}

func leadingWhitespace(value string) string {
	for i, r := range value {
		if r != ' ' && r != '\t' {
			return value[:i]
		}
	}
	return ""
}

func parseTopologyAttr(topology string, pattern *regexp.Regexp) int {
	matches := pattern.FindStringSubmatch(topology)
	if len(matches) < 2 {
		return 0
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return value
}

// setVMCPUWithTopologySync 设置虚拟机 vCPU 数量，同时同步 CPU topology。
// 当 domain XML 中存在 topology 时，必须同时修改 vcpu 和 topology 后 define，
// 因为 virsh setvcpus 和 virsh define 都会校验 sockets×dies×cores×threads == vcpu。
// 不存在 topology 时，使用 virsh setvcpus 命令。
// 注意：必须同时检查 inactive 和 live XML，因为运行时 virsh setvcpus --config
// 也会校验当前 live 域的 topology 一致性。
func setVMCPUWithTopologySync(name string, vcpu, maxVCPU int) error {
	inactiveResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if inactiveResult.Error != nil {
		return fmt.Errorf("获取虚拟机持久化 XML 失败: %s", inactiveResult.Stderr)
	}
	xmlStr := inactiveResult.Stdout

	hasTopology := vmCPUTopologyRegexp.MatchString(xmlStr)

	if !hasTopology {
		// 持久化配置没有 topology，再检查在线配置（运行时 virsh setvcpus 可能校验在线域的拓扑）
		liveResult := utils.ExecCommand("virsh", "dumpxml", name)
		if liveResult.Error == nil {
			hasTopology = vmCPUTopologyRegexp.MatchString(liveResult.Stdout)
			if hasTopology {
				// 在线配置有 topology 但持久化没有，以在线配置的 topology 为准来重建持久化
				// 需要将在线 topology 信息合并到持久化 XML 中
				xmlStr = mergeTopologyFromLiveToInactive(xmlStr, liveResult.Stdout)
			}
		}
	}

	if !hasTopology {
		// 无 topology，使用传统的 virsh setvcpus 命令
		// 设置最大 vCPU（热添加上限），默认与当前 vCPU 相同
		maxArg := vcpu
		if maxVCPU > vcpu {
			maxArg = maxVCPU
		}
		result := utils.ExecCommand("virsh", "setvcpus", name, strconv.Itoa(maxArg), "--config", "--maximum")
		if result.Error != nil {
			return fmt.Errorf("设置 CPU 最大值失败: %s", result.Stderr)
		}
		result = utils.ExecCommand("virsh", "setvcpus", name, strconv.Itoa(vcpu), "--config")
		if result.Error != nil {
			return fmt.Errorf("设置 CPU 失败: %s", result.Stderr)
		}
		return nil
	}

	// 有 topology：同时修改 vcpu 和 topology，然后 define
	xmlStr = vmVCPUValueRegexp.ReplaceAllString(xmlStr, BuildVCPUTag(vcpu, maxVCPU))

	topoVCPU := EffectiveTopologyVCPU(vcpu, maxVCPU)
	mode := ParseVMCPUTopologyModeFromDomainXML(xmlStr)
	osType := detectVMOSType("", xmlStr)
	xmlStr = ApplyCPUTopologyModeToDomainXML(xmlStr, mode, osType, topoVCPU)

	// 兜底：如果 ApplyCPUTopologyModeToDomainXML 未修改 topology（auto 模式非 Windows），
	// 需要直接按现有 topology 更新 cores 以保证 sockets×dies×cores×threads == topoVCPU
	if vmCPUTopologyRegexp.MatchString(xmlStr) {
		topology := vmCPUTopologyRegexp.FindString(xmlStr)
		sockets := parseTopologyAttr(topology, vmTopologySocketsRegex)
		dies := parseTopologyAttr(topology, vmTopologyDiesRegex)
		threads := parseTopologyAttr(topology, vmTopologyThreadsRegex)
		if sockets <= 0 {
			sockets = 1
		}
		if dies <= 0 {
			dies = 1
		}
		if threads <= 0 {
			threads = 1
		}
		multiplier := sockets * dies * threads
		if multiplier > 0 && sockets*dies*parseTopologyAttr(topology, vmTopologyCoresRegex)*threads != topoVCPU {
			cores := topoVCPU / multiplier
			if cores > 0 && multiplier*cores == topoVCPU {
				newTopology := fmt.Sprintf("<topology sockets='%d' dies='%d' cores='%d' threads='%d'/>", sockets, dies, cores, threads)
				xmlStr = vmCPUTopologyRegexp.ReplaceAllString(xmlStr, newTopology)
			} else {
				return fmt.Errorf("CPU 拓扑无法匹配目标 vCPU 数量：当前拓扑 sockets=%d dies=%d threads=%d，无法整除 vcpu=%d", sockets, dies, threads, topoVCPU)
			}
		}
	}

	_, err := defineDomainXMLRPC(xmlStr)
	if err != nil {
		return fmt.Errorf("设置 CPU 拓扑失败: %w", err)
	}
	return nil
}

// mergeTopologyFromLiveToInactive 将在线 XML 中的 CPU topology 信息合并到持久化 XML 中。
// 用于持久化配置没有 topology 但在线配置有的情况。
func mergeTopologyFromLiveToInactive(inactiveXML, liveXML string) string {
	liveTopology := vmCPUTopologyRegexp.FindString(liveXML)
	if strings.TrimSpace(liveTopology) == "" {
		return inactiveXML
	}

	// 查找或创建 <cpu> 块来承载 topology
	if vmCPUBlockRegexp.MatchString(inactiveXML) {
		return vmCPUBlockRegexp.ReplaceAllStringFunc(inactiveXML, func(cpuBlock string) string {
			return applyTopologyToCPUBlock(cpuBlock, liveTopology)
		})
	}

	// 没有 <cpu> 块，创建一个
	cpuBlock := fmt.Sprintf("  <cpu mode='host-passthrough' check='none' migratable='on'>\n    %s\n  </cpu>", liveTopology)
	if strings.Contains(inactiveXML, "</features>") {
		return strings.Replace(inactiveXML, "</features>", "</features>\n"+cpuBlock, 1)
	}
	if strings.Contains(inactiveXML, "<devices>") {
		return strings.Replace(inactiveXML, "<devices>", cpuBlock+"\n  <devices>", 1)
	}
	return inactiveXML
}

// SetVMCPUTopologyMode 设置虚拟机 CPU 拓扑模式。运行中的虚拟机需要先关机后再修改。
func SetVMCPUTopologyMode(name, mode string) error {
	stateResult := utils.ExecCommand("virsh", "domstate", name)
	if stateResult.Error != nil {
		return fmt.Errorf("获取虚拟机状态失败: %s", stateResult.Stderr)
	}
	state := strings.TrimSpace(stateResult.Stdout)
	if state == "running" || state == "paused" {
		return fmt.Errorf("请先关机后再修改 CPU 拓扑")
	}

	xmlResult := utils.ExecCommand("virsh", "dumpxml", name, "--inactive")
	if xmlResult.Error != nil {
		return fmt.Errorf("获取虚拟机 XML 失败: %s", xmlResult.Stderr)
	}

	xmlStr := xmlResult.Stdout
	osType := detectVMOSType("", xmlStr)
	updated := ApplyCPUTopologyModeToDomainXML(xmlStr, mode, osType, ParseVCPUCountFromDomainXML(xmlStr))

	xmlPath := fmt.Sprintf("/tmp/_cpu-topology-%s.xml", name)
	utils.ExecShell(fmt.Sprintf("cat > %s << 'XMLEOF'\n%s\nXMLEOF", utils.ShellSingleQuote(xmlPath), updated))
	defineResult := utils.ExecCommand("virsh", "define", xmlPath)
	utils.ExecShell(fmt.Sprintf("rm -f %s", utils.ShellSingleQuote(xmlPath)))
	if defineResult.Error != nil {
		return fmt.Errorf("修改 CPU 拓扑失败: %s", defineResult.Stderr)
	}
	return nil
}
