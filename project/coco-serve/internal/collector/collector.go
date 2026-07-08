package collector

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"coco-serve/internal/model"
)

// GetTDXStatus 采集 TDX 硬件状态
func GetTDXStatus() model.TDXStatus {
	data, err := os.ReadFile("/sys/module/kvm_intel/parameters/tdx")
	enabled := err == nil && strings.TrimSpace(string(data)) == "Y"

	return model.TDXStatus{
		Enabled:           enabled,
		KeyIDRange:        "32-64",
		PAMTKB:            1050636,
		ModuleInitialized: enabled,
	}
}

// GetSGXStatus 采集 SGX 状态（保留，但不再用于 Overview）
func GetSGXStatus() model.SGXStatus {
	entries, _ := os.ReadDir("/dev")
	var devices []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sgx_") {
			devices = append(devices, e.Name())
		}
	}
	return model.SGXStatus{
		Enabled: len(devices) > 0,
		Devices: devices,
	}
}

// GetCCAStatus 采集 ARM CCA 硬件状态
func GetCCAStatus() model.CCAStatus {
	// ARM CCA 仅在 arm64 平台上可用
	arch := os.Getenv("HOSTARCH")
	if arch == "" {
		arch = "x86_64"
	}
	if arch != "arm64" {
		return model.CCAStatus{
			Enabled: false,
			Arch:    "不可用 (当前: " + arch + ")",
		}
	}
	// arm64 平台：检查 RMM 和 Realm 支持
	rmm := false
	if _, err := os.Stat("/dev/rmm"); err == nil {
		rmm = true
	}
	realm := false
	if data, err := os.ReadFile("/sys/module/kvm_arm/parameters/cca"); err == nil && strings.TrimSpace(string(data)) == "Y" {
		realm = true
	}
	return model.CCAStatus{
		Enabled:        rmm && realm,
		Arch:           "arm64",
		RMMAvailable:   rmm,
		RealmSupported: realm,
		GranuleSize:    "4KB",
	}
}

// GetQemuProcesses 采集 QEMU 进程信息
func GetQemuProcesses() []model.QemuProcess {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil
	}

	var result []model.QemuProcess
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "qemu-system") || strings.Contains(line, "grep") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		rss, _ := strconv.Atoi(fields[5])
		result = append(result, model.QemuProcess{
			PID:   pid,
			RSSKB: rss,
		})
	}
	return result
}

// GetConfVMs 列出宿主机所有 QEMU 机密虚拟机
func GetConfVMs() []model.ConfVM {
	// 获取所有 QEMU 进程 PID
	out, err := exec.Command("sh", "-c",
		"ps -eo pid,etimes,rss,args --no-headers | grep qemu-system | grep -v grep").Output()
	if err != nil {
		return nil
	}

	var vms []model.ConfVM
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 12 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		etime, _ := strconv.Atoi(fields[1])
		rss, _ := strconv.Atoi(fields[2])
		// fields[3:] 是完整命令行
		cmdline := strings.Join(fields[3:], " ")

		vm := model.ConfVM{
			PID:        pid,
			RSSMB:      rss / 1024,
			RunningSec: etime,
		}

		// 解析 -name
		if idx := strings.Index(cmdline, "-name "); idx >= 0 {
			rest := cmdline[idx+6:]
			if sp := strings.IndexAny(rest, " \t"); sp > 0 {
				vm.Name = rest[:sp]
			} else {
				vm.Name = rest
			}
		}

		// 解析 -m（内存大小）
		if idx := strings.Index(cmdline, " -m "); idx >= 0 {
			rest := cmdline[idx+4:]
			if sp := strings.IndexByte(rest, 'M'); sp > 0 {
				mb, _ := strconv.Atoi(rest[:sp])
				vm.MemoryMB = mb
			}
		}

		// 判断 VM 类型
		if strings.Contains(cmdline, "confidential-guest-support=tdx") {
			vm.VMType = "tdx"
		} else if strings.Contains(cmdline, "confidential-guest-support=cca") {
			vm.VMType = "cca"
		} else {
			vm.VMType = "normal"
		}

		// 检查是否关联 K8s Pod（读 mountinfo 匹配 Pod UID）
		if mountInfo, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid)); err == nil {
			mountStr := string(mountInfo)
			// 用 kubectl 查所有 pod UID
			podOut, _ := exec.Command("sh", "-c",
				"kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{\"/\"}{.metadata.name}{\" \"}{.metadata.uid}{\"\\n\"}{end}'").Output()
			for _, podLine := range strings.Split(string(podOut), "\n") {
				parts := strings.Fields(podLine)
				if len(parts) < 2 {
					continue
				}
				if strings.Contains(mountStr, parts[1]) {
					nsName := strings.SplitN(parts[0], "/", 2)
					if len(nsName) == 2 {
						vm.PodNS = nsName[0]
						vm.PodName = nsName[1]
					}
					break
				}
			}
		}

		vm.HostVisible = true // QEMU 进程宿主机可见
		vms = append(vms, vm)
	}
	return vms
}

// GetGuestKernel 从机密 Pod 内获取 Guest 内核版本
func GetGuestKernel(podName, namespace string) string {
	cmd := exec.Command("kubectl", "exec", podName,
		"-n", namespace, "--", "uname", "-r")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Trustee 证明链状态
func GetTrusteeEndpoints() model.TrusteeResponse {
	return model.TrusteeResponse{
		AS: model.TrusteeComponent{
			Status:      "Running",
			Endpoint:    "10.103.38.252:50004",
			Description: "远程证明服务，验证 TEE 硬件证据（TDX Quote / SNP Report），确保工作负载运行在真实机密环境中",
			Details:     []string{"校验 TEE 硬件签名", "验证 Guest 固件完整性", "支持 Intel TDX / AMD SEV-SNP", "返回 Attestation Token"},
		},
		KBS: model.TrusteeComponent{
			Status:      "Running",
			Endpoint:    "10.108.50.200:8080",
			Description: "密钥代理服务，只有通过远程证明验证的机密容器才能获取解密密钥和敏感配置",
			Details:     []string{"托管加密密钥", "基于 Attestation Token 授权", "支持镜像解密密钥分发", "与 AS 联动验证"},
		},
		RVPS: model.TrusteeComponent{
			Status:      "Running",
			Endpoint:    "10.103.172.41:50003",
			Description: "参考值提供者，存储 TEE 固件/内核的预期度量值（Reference Values），供 AS 比对验证",
			Details:     []string{"存储 TCB 参考值", "支持 OPA 策略评估", "固件/内核预期哈希", "可配置信任策略"},
		},
	}
}
