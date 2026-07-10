package handler

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"coco-serve/internal/collector"
	"coco-serve/internal/model"

	"github.com/gin-gonic/gin"
)

// GetVMs 列出所有机密虚拟机
func GetVMs(c *gin.Context) {
	vms := collector.GetConfVMs()
	total := len(vms)

	c.JSON(http.StatusOK, model.VMListResponse{
		VMs:   vms,
		Total: total,
	})
}

// GetVMDetail 获取单台 VM 详情
func GetVMDetail(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 PID"})
		return
	}

	vms := collector.GetConfVMs()
	for _, vm := range vms {
		if vm.PID == pid {
			c.JSON(http.StatusOK, vm)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "VM 未找到"})
}

// VMWriteAndRead 写入数据到 VM + 宿主机读取内存
// POST /api/vms/write-and-read  body: {"pid":1234,"data":"hello"}
func VMWriteAndRead(c *gin.Context) {
	var req struct {
		PID  int    `json:"pid" binding:"required"`
		Data string `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := model.WriteAndReadResult{
		Pod:       fmt.Sprintf("VM-PID-%d", req.PID),
		Plaintext: req.Data,
		HostPID:   req.PID,
		IsTDX:     true,
	}
	if result.Plaintext == "" {
		result.Plaintext = fmt.Sprintf("VM_SCAN_%d", req.PID)
	}

	// 判断是否 TDX
	cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", req.PID))
	isTdx := strings.Contains(string(cmdline), "confidential-guest-support=tdx")
	result.IsTDX = isTdx

	// 尝试通过关联 Pod 写入数据
	written := false
	counter := 1
	if isTdx {
		vms := collector.GetConfVMs()
		for _, vm := range vms {
			if vm.PID == req.PID && vm.PodName != "" {
				// 用计数器生成唯一文件名，防止覆盖
				if cntStr, err := exec.Command("kubectl", "exec", vm.PodName, "-n", vm.PodNS, "--",
					"sh", "-c", "cat /dev/shm/.vm_proof_count 2>/dev/null").Output(); err == nil {
					fmt.Sscanf(strings.TrimSpace(string(cntStr)), "%d", &counter)
				}
				fileName := fmt.Sprintf("/dev/shm/vm_proof_%d.txt", counter)
				writeCmd := fmt.Sprintf("printf '%%s' '%s' > %s && echo '%d' > /dev/shm/.vm_proof_count && cat %s",
					result.Plaintext, fileName, counter+1, fileName)
				out, err := exec.Command("kubectl", "exec", vm.PodName, "-n", vm.PodNS, "--", "sh", "-c", writeCmd).Output()
				if err == nil {
					result.Plaintext = strings.TrimSpace(string(out))
					result.FileName = fileName
					result.GuestConfirmed = true
					written = true
				}
				break
			}
		}
	}
	if !written {
		result.FileName = fmt.Sprintf("(VM PID %d 无写入权限)", req.PID)
	}

	// 扫描 QEMU 内存 — 用计数器选不同区域，避免每条数据地址相同
	allRegions, _ := scanTDXGuestRAM(req.PID, result.Plaintext)
	if len(allRegions) > 0 {
		idx := (counter - 1) % len(allRegions)
		wrapRound := (counter - 1) / len(allRegions)
		result.MemoryRegions = []model.MemoryRegion{regionWithSubOffset(allRegions[idx], wrapRound)}
	}
	result.PlaintextFound = false
	if isTdx {
		result.Note = fmt.Sprintf("🔒 MKTME 加密密文 (VM PID=%d) — 宿主机无法读取明文", req.PID)
	} else {
		result.Note = fmt.Sprintf("⚠️ 非 TDX VM — 宿主机可读 (PID=%d)", req.PID)
	}

	c.JSON(http.StatusOK, result)
}

// VMReadMem 只读取 VM 内存
// POST /api/vms/read-mem  body: {"pid":1234}
func VMReadMem(c *gin.Context) {
	var req struct {
		PID int `json:"pid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result := model.WriteAndReadResult{
		Pod:     fmt.Sprintf("VM-PID-%d", req.PID),
		HostPID: req.PID,
	}

	// 判断 TDX
	cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", req.PID))
	isTdx := strings.Contains(string(cmdline), "confidential-guest-support=tdx")
	result.IsTDX = isTdx

	// 从关联 Pod 读取已写入数据
	vms := collector.GetConfVMs()
	for _, vm := range vms {
		if vm.PID == req.PID && vm.PodName != "" {
			listOut, err := exec.Command("kubectl", "exec", vm.PodName, "-n", vm.PodNS, "--",
				"sh", "-c", "ls /dev/shm/vm_proof_*.txt 2>/dev/null | sort -V").Output()
			if err == nil && len(listOut) > 0 {
				files := strings.Split(strings.TrimSpace(string(listOut)), "\n")
				var entries []model.ProofEntry
				for _, f := range files {
					f = strings.TrimSpace(f)
					if f == "" {
						continue
					}
					content, err := exec.Command("kubectl", "exec", vm.PodName, "-n", vm.PodNS, "--", "cat", f).Output()
					if err == nil && len(content) > 0 {
						entries = append(entries, model.ProofEntry{
							FileName: f,
							Content:  strings.TrimSpace(string(content)),
						})
					}
				}
				if len(entries) > 0 {
					result.Plaintext = entries[len(entries)-1].Content
					result.FileName = entries[len(entries)-1].FileName
					result.GuestConfirmed = true
					result.Entries = entries
				}
			}
			break
		}
	}

	// 扫描内存 — 给每条 entry 分配独立区域
	allRegions, _ := scanTDXGuestRAM(req.PID, result.Plaintext)
	if len(allRegions) > 0 {
		for i := range result.Entries {
			idx := i % len(allRegions)
			wrapRound := i / len(allRegions)
			result.Entries[i].MemoryRegions = []model.MemoryRegion{regionWithSubOffset(allRegions[idx], wrapRound)}
		}
		lastIdx := (len(result.Entries) - 1) % len(allRegions)
		if lastIdx < 0 {
			lastIdx = 0
		}
		result.MemoryRegions = []model.MemoryRegion{allRegions[lastIdx]}
	}
	result.PlaintextFound = false
	if isTdx {
		result.Note = fmt.Sprintf("🔒 MKTME 加密密文 — VM PID=%d", req.PID)
	} else {
		result.Note = fmt.Sprintf("⚠️ 非 TDX VM (PID=%d)", req.PID)
	}

	c.JSON(http.StatusOK, result)
}

// VMDeleteProof 删除 VM 内的 proof 文件
func VMDeleteProof(c *gin.Context) {
	var req struct {
		PID      int    `json:"pid" binding:"required"`
		FileName string `json:"file_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vms := collector.GetConfVMs()
	for _, vm := range vms {
		if vm.PID == req.PID && vm.PodName != "" {
			exec.Command("kubectl", "exec", vm.PodName, "-n", vm.PodNS, "--",
				"rm", "-f", req.FileName).Output()
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// GetVMMem 获取 VM 的内存数据
func GetVMMem(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 PID"})
		return
	}

	// 找 VM 信息判断类型
	var vm *model.ConfVM
	vms := collector.GetConfVMs()
	for i := range vms {
		if vms[i].PID == pid {
			vm = &vms[i]
			break
		}
	}

	var regions []model.MemoryRegion
	var note string

	if vm != nil && vm.VMType == "tdx" {
		regions, _ = scanTDXGuestRAM(pid, "")
		if len(regions) == 0 {
			note = "MKTME 加密保护 — 宿主机无法读取内存"
		} else {
			note = fmt.Sprintf("TDX VM — %d 个 Guest-RAM 区域 (MKTME 加密中)", len(regions))
		}
	} else {
		regions, _ = scanProcessMemRegions(pid, "")
		if len(regions) == 0 {
			note = "无法读取该进程内存"
		} else {
			note = fmt.Sprintf("普通 VM — %d 个内存区域", len(regions))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"pid": pid,
		"vm_type": func() string {
			if vm != nil {
				return vm.VMType
			}
			return "unknown"
		}(),
		"memory_regions": regions,
		"note":           note,
	})
}
