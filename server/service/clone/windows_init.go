package clone

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/vm/memory"
	"kvm_console/service/vm_xml"
	"kvm_console/utils"
)

func windowsSystemDiskTargetDev(bus string) string {
	switch D.NormalizeVMDiskBus(bus) {
	case "sata", "scsi":
		return "sda"
	case "ide":
		return "hda"
	default:
		return "vda"
	}
}

func windowsDiskControllerXML(bus string) string {
	switch D.NormalizeVMDiskBus(bus) {
	case "sata":
		return "    <controller type='sata' index='0'/>\n"
	case "scsi":
		return "    <controller type='scsi' index='0' model='virtio-scsi'/>\n"
	default:
		return ""
	}
}

func buildWindowsUnattendXML(hostname, password string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">
  <settings pass="specialize">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <ComputerName>%s</ComputerName>
    </component>
  </settings>
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideLocalAccountScreen>true</HideLocalAccountScreen>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <ProtectYourPC>3</ProtectYourPC>
        <SkipMachineOOBE>true</SkipMachineOOBE>
        <SkipUserOOBE>true</SkipUserOOBE>
      </OOBE>
      <UserAccounts>
        <AdministratorPassword>
          <Value>%s</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <AutoLogon>
        <Enabled>true</Enabled>
        <Username>Administrator</Username>
        <Password>
          <Value>%s</Value>
          <PlainText>true</PlainText>
        </Password>
        <LogonCount>1</LogonCount>
      </AutoLogon>
    </component>
    <component name="Microsoft-Windows-International-Core" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <InputLocale>zh-CN</InputLocale>
      <SystemLocale>zh-CN</SystemLocale>
      <UILanguage>zh-CN</UILanguage>
      <UserLocale>zh-CN</UserLocale>
    </component>
  </settings>
</unattend>`, hostname, password, password)
}

func injectWindowsUnattendFile(vmName, cloneDisk, hostname, password string, progressFn func(int, string)) {
	if progressFn == nil {
		progressFn = func(int, string) {}
	}
	if password == "" {
		password = "Qwert333"
	}

	progressFn(35, "注入 Windows 应答文件...")
	unattendXML := buildWindowsUnattendXML(hostname, password)
	unattendPath := fmt.Sprintf("/tmp/_unattend-%s.xml", vmName)
	_ = os.WriteFile(unattendPath, []byte(unattendXML), 0600)

	injectResult := utils.ExecCommandLongRunning("virt-customize", "-a", cloneDisk, "--no-network",
		"--upload", unattendPath+":/Windows/Panther/unattend.xml",
		"--quiet")
	_ = os.Remove(unattendPath)

	if injectResult.Error != nil {
		progressFn(38, "Windows 应答文件注入失败，首次启动可能需要手动 OOBE")
	}
}

// cloneWindows Windows 克隆逻辑
func cloneWindows(ctx context.Context, params *CloneParams, cloneDisk string, ramMB int, memoryMeta *memory.VMMemoryMetadata, needUEFI bool, progressFn func(int, string)) error {
	templateDir := config.GlobalConfig.TemplateDir

	password := params.Password
	if password == "" {
		password = "Qwert333"
	}

	injectWindowsUnattendFile(params.Name, cloneDisk, params.Hostname, password, progressFn)

	nvramClone := ""
	if needUEFI {
		nvramTemplate := filepath.Join(templateDir, "win2k22-nvram.fd")
		nvramClone = fmt.Sprintf("/var/lib/libvirt/qemu/nvram/%s_VARS.fd", params.Name)

		if utils.FileExists(nvramTemplate) {
			if err := vm_xml.CreateQCOW2NVRAMFromTemplate(nvramTemplate, nvramClone); err != nil {
				return err
			}
		} else {
			if err := vm_xml.CreateQCOW2NVRAMFromTemplate("/usr/share/OVMF/OVMF_VARS_4M.ms.fd", nvramClone); err != nil {
				return err
			}
		}
	}

	progressFn(40, "生成 Windows VM XML...")

	macResult := utils.ExecShell(`printf '52:54:00:%02x:%02x:%02x' $((RANDOM%256)) $((RANDOM%256)) $((RANDOM%256))`)
	macAddr := strings.TrimSpace(macResult.Stdout)
	if macAddr == "" {
		macAddr = "52:54:00:aa:bb:cc"
	}

	ramKiB := ramMB * 1024
	diskBus := D.NormalizeVMDiskBus(params.DiskBus)
	if diskBus == "" {
		diskBus = "virtio"
	}
	diskTargetDev := windowsSystemDiskTargetDev(diskBus)
	diskControllerXML := windowsDiskControllerXML(diskBus)
	osXML := `  <os>
    <type arch='x86_64' machine='pc-q35-noble'>hvm</type>
    <boot dev='hd'/>
  </os>`
	smmXML := ""
	tpmXML := ""
	if needUEFI {
		osXML = fmt.Sprintf(`  <os firmware='efi'>
    <type arch='x86_64' machine='pc-q35-noble'>hvm</type>
    <firmware>
      <feature enabled='yes' name='enrolled-keys'/>
      <feature enabled='yes' name='secure-boot'/>
    </firmware>
    <loader readonly='yes' secure='yes' type='pflash'>/usr/share/OVMF/OVMF_CODE_4M.ms.fd</loader>
    <nvram template='/usr/share/OVMF/OVMF_VARS_4M.ms.fd' templateFormat='raw' format='qcow2'>%s</nvram>
    <boot dev='hd'/>
  </os>`, nvramClone)
		smmXML = "<smm state='on'/>"
		tpmXML = "    <tpm model='tpm-crb'><backend type='emulator' version='2.0'/></tpm>\n"
	}

	rtcOffset := D.ResolveRTCOffset(params.RTCOffset, "windows")
	rtcStartDate := D.NormalizeRTCStartDate(params.RTCStartDate)
	clockOpenTag := fmt.Sprintf("<clock offset='%s'>", rtcOffset)
	if rtcStartDate != D.VMRTCStartDateNow {
		epoch, err := D.ParseRTCStartDateToEpoch(rtcStartDate)
		if err != nil {
			return err
		}
		rtcOffset = D.VMRTCOffsetAbsolute
		clockOpenTag = fmt.Sprintf("<clock offset='%s' start='%s'>", rtcOffset, epoch)
	}
	vmXML := fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <memory unit='KiB'>%d</memory>
%s
%s
  <features>
    <acpi/><apic/>
    <hyperv mode='custom'>
      <relaxed state='on'/><vapic state='on'/><spinlocks state='on' retries='8191'/>
    </hyperv>
    <vmport state='off'/>%s
  </features>
  <cpu mode='host-passthrough' check='none' migratable='on'/>
  %s
    <timer name='rtc' tickpolicy='catchup'/><timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/><timer name='hypervclock' present='yes'/>
  </clock>
  <on_poweroff>destroy</on_poweroff><on_reboot>restart</on_reboot><on_crash>destroy</on_crash>
  <pm><suspend-to-mem enabled='no'/><suspend-to-disk enabled='no'/></pm>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2' discard='unmap' detect_zeroes='unmap'/>
      <source file='%s'/><target dev='%s' bus='%s'/>
    </disk>
    <controller type='usb' index='0' model='qemu-xhci' ports='15'/>
    <controller type='virtio-serial' index='0'/>
%s
%s
    <input type='tablet' bus='usb'/>
%s
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'>
      <listen type='address' address='0.0.0.0'/>
    </graphics>
    <video><model type='virtio' heads='1' primary='yes'/></video>
    <watchdog model='itco' action='reset'/>
    <memballoon model='virtio' freePageReporting='on'><stats period='5'/></memballoon>
  </devices>
</domain>`,
		params.Name, ramKiB, D.BuildVCPUTag(params.VCPU, params.MaxVCPU), osXML, smmXML, clockOpenTag, cloneDisk, diskTargetDev, diskBus, diskControllerXML, D.BuildOVSInterfaceXML(macAddr, params.NicModel), tpmXML)
	var err error
	if memoryMeta != nil {
		vmXML, err = memory.ApplyMemoryMetadataToDomainXML(vmXML, memoryMeta, false)
		if err != nil {
			return err
		}
	}
	vmXML, err = vm_xml.ApplyVMGuestAgentConfigToDomainXML(vmXML, params.GuestAgent)
	if err != nil {
		return err
	}
	vmXML, err = vm_xml.ApplySMBIOS1ConfigToDomainXML(vmXML, params.SMBIOS1, true)
	if err != nil {
		return err
	}
	vmXML, err = D.ApplyVMAPICToDomainXML(vmXML, params.APIC)
	if err != nil {
		return err
	}
	vmXML, err = vm_xml.ApplyVMPAEToDomainXML(vmXML, params.PAE)
	if err != nil {
		return err
	}
	vmXML = vm_xml.ApplyVMVideoModelToDomainXML(vmXML, params.VideoModel, "windows")
	vmXML = vm_xml.ApplyWindowsGuestOptimizationsToDomainXML(vmXML)
	topoVCPU := D.EffectiveTopologyVCPU(params.VCPU, params.MaxVCPU)
	vmXML = D.ApplyCPUTopologyModeToDomainXML(vmXML, params.CPUTopologyMode, "windows", topoVCPU)
	vmXML = D.ApplyVMCPULimitToDomainXML(vmXML, params.VCPU, params.CPULimitPercent)
	if params.CPUAffinity != "" {
		var affErr error
		vmXML, affErr = D.ApplyCPUAffinityIfSet(vmXML, topoVCPU, params.CPUAffinity)
		if affErr != nil {
			return affErr
		}
	}
	firstBootColdReboot := D.ShouldUseWindowsFirstBootColdReboot(params.FirstBootRebootMode, "windows")
	if firstBootColdReboot {
		vmXML = D.ApplyFirstBootRebootModeToDomainXML(vmXML, params.FirstBootRebootMode)
	}
	vmXML, err = D.ApplyVPCSwitchToDomainXML(vmXML, params.SwitchID)
	if err != nil {
		return err
	}

	if _, err := libvirt_rpc.DefineDomainXMLRPC(vmXML); err != nil {
		return fmt.Errorf("定义虚拟机失败: %w", err)
	}
	if memoryMeta != nil {
		if err := memory.WriteVMMemoryMetadata(params.Name, memoryMeta); err != nil {
			return err
		}
	}
	if err := D.WriteVMTemplateSource(params.Name, params.Template, "linked"); err != nil {
		logger.App.Warn("写入VM模板源信息失败", "error", err)
	}
	if err := D.SetVMRemark(params.Name, params.Remark); err != nil {
		logger.App.Warn("设置VM备注失败", "error", err)
	}

	if err := D.SetVMFreeze(params.Name, params.Freeze); err != nil {
		logger.App.Warn("设置VM冻结配置失败", "error", err)
	}

	startFn := D.StartVM
	if firstBootColdReboot {
		startFn = D.StartVMPreserveRebootAction
	}
	if err := startFn(params.Name); err != nil {
		return err
	}
	if firstBootColdReboot {
		if err := D.CompleteWindowsFirstBootColdReboot(ctx, params.Name, progressFn); err != nil {
			return err
		}
	}

	return nil
}
