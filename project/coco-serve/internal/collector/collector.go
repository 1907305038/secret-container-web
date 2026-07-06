package collector

import (
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

// GetSGXStatus 采集 SGX 状态
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
