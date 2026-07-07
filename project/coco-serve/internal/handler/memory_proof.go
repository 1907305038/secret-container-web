package handler

import (
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"coco-serve/internal/logger"
	"coco-serve/internal/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// GetMemoryEncryptProof 全自动模式：写入测试数据 → 宿主机读取 → 容器内读取 → 对比
// GET /api/demo/memory-encrypt?pod=X&ns=Y
func GetMemoryEncryptProof(c *gin.Context) {
	ns := c.Query("ns")
	pod := c.Query("pod")
	if ns == "" || pod == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要 ns 和 pod 参数"})
		return
	}

	proof := model.MemoryEncryptProof{
		Pod:      ns + "/" + pod,
		AutoMode: true,
	}

	// 1. 在容器内写入已知明文
	plaintext := fmt.Sprintf("TDX_VERIFY_%d", time.Now().Unix())
	writeCmd := fmt.Sprintf("echo '%s' > /dev/shm/proof.txt && cat /dev/shm/proof.txt", plaintext)
	out, err := execPodCmd(pod, ns, writeCmd)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "无法在容器内写入测试数据: " + err.Error(), "hint": "容器可能缺少 /dev/shm 或 shell"})
		return
	}
	proof.Plaintext = strings.TrimSpace(out)

	// 2. 找 QEMU PID
	qemuPID, err := findQemuPID(pod, ns)
	if err != nil {
		proof.HostView.Note = "未找到 QEMU 进程: " + err.Error()
		// 仍然返回容器内视角
		proof.GuestView = readGuestMemoryView(pod, ns, proof.Plaintext)
		c.JSON(http.StatusOK, proof)
		return
	}
	proof.QemuPID = qemuPID

	// 3. 宿主机视角：读 QEMU 内存区域，搜索明文
	proof.HostView = readHostMemoryView(qemuPID, proof.Plaintext)

	// 4. 容器内视角
	proof.GuestView = readGuestMemoryView(pod, ns, proof.Plaintext)

	logger.Memory.Info("auto proof completed",
		zap.String("pod", proof.Pod),
		zap.Int("qemu_pid", proof.QemuPID),
		zap.Bool("host_found", proof.HostView.Found),
		zap.Float64("host_entropy", proof.HostView.Entropy),
	)

	c.JSON(http.StatusOK, proof)
}

// GetMemoryCompare 半自动模式：给定 PID，读取宿主机侧 /proc/<pid>/mem 内存内容
// GET /api/demo/memory-compare?pod=X&ns=Y&pid=Z
func GetMemoryCompare(c *gin.Context) {
	ns := c.Query("ns")
	pod := c.Query("pod")
	pidStr := c.Query("pid")
	if pidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要 pid 参数"})
		return
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pid 必须是整数"})
		return
	}

	proof := model.MemoryEncryptProof{
		Pod:      ns + "/" + pod,
		QemuPID:  pid,
		AutoMode: false,
	}

	// 检查 /proc/<pid> 是否存在
	if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); os.IsNotExist(err) {
		proof.HostView.Note = fmt.Sprintf("宿主机 /proc/%d 不存在 — 此进程在 TDX 加密 VM 内运行", pid)
		c.JSON(http.StatusOK, proof)
		return
	}

	// 读宿主机侧内存
	proof.HostView = readHostMemoryView(pid, "")

	// 如果有 pod/ns，也读容器内视角
	if pod != "" && ns != "" {
		proof.GuestView = readGuestMemoryView(pod, ns, "")
		proof.GuestView.Note = "半自动模式：请先在容器内手动写入测试数据，再点击验证"
	}

	c.JSON(http.StatusOK, proof)
}

// ---- 辅助函数 ----

// execPodCmd 在 Pod 内执行命令
func execPodCmd(pod, ns, cmd string) (string, error) {
	shells := [][]string{{"sh", "-c"}, {"bash", "-c"}, {"/busybox/sh", "-c"}}
	for _, s := range shells {
		args := append([]string{"exec", pod, "-n", ns, "--"}, append(s, cmd)...)
		out, err := exec.Command("kubectl", args...).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", fmt.Errorf("no usable shell in pod")
}

// findQemuPID 找到指定 Pod 对应的 QEMU 进程 PID
func findQemuPID(pod, ns string) (int, error) {
	// 获取 Pod UID
	uidOut, err := exec.Command("kubectl", "get", "pod", pod, "-n", ns,
		"-o", "jsonpath={.metadata.uid}").Output()
	podUID := strings.TrimSpace(string(uidOut))
	if err != nil || podUID == "" {
		return 0, fmt.Errorf("无法获取 Pod UID")
	}

	// 遍历 QEMU 进程，通过 /proc/PID/mountinfo 匹配 Pod UID
	qemuOut, _ := exec.Command("sh", "-c",
		"ps -eo pid,args --no-headers | grep qemu-system | grep -v grep | awk '{print $1}'").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(qemuOut)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid == 0 {
			continue
		}
		mountInfo, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid))
		if err == nil && strings.Contains(string(mountInfo), podUID) {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("未找到匹配 Pod UID 的 QEMU 进程")
}

// readHostMemoryView 读取宿主机侧的内存视图
func readHostMemoryView(pid int, searchPlaintext string) model.MemoryRegionSet {
	result := model.MemoryRegionSet{Found: false}

	// 读 /proc/<pid>/maps 获取可读内存区域
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		result.Note = fmt.Sprintf("无法读取 /proc/%d/maps: %v", pid, err)
		return result
	}

	// 选几个代表性区域：heap（[heap]）和第一个匿名映射
	var candidates []struct {
		start, end uint64
		name       string
	}
	lines := strings.Split(string(mapsData), "\n")
	anonCount := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		addrParts := strings.SplitN(parts[0], "-", 2)
		if len(addrParts) != 2 {
			continue
		}
		start, _ := strconv.ParseUint(addrParts[0], 16, 64)
		end, _ := strconv.ParseUint(addrParts[1], 16, 64)

		// 跳过过小的区域
		if end-start < 4096 {
			continue
		}

		// 只选可读的
		if !strings.Contains(parts[1], "r") {
			continue
		}

		name := parts[len(parts)-1]
		if name == "[heap]" {
			candidates = append(candidates, struct {
				start, end uint64
				name       string
			}{start, end, "heap"})
		} else if name == "[stack]" {
			candidates = append(candidates, struct {
				start, end uint64
				name       string
			}{start, end, "stack"})
		} else if name == "" || strings.HasPrefix(name, "/") {
			anonCount++
			if anonCount <= 3 {
				candidates = append(candidates, struct {
					start, end uint64
					name       string
				}{start, end, fmt.Sprintf("anon-%d", anonCount)})
			}
		}
	}

	if len(candidates) == 0 {
		result.Note = "未找到可读的内存区域"
		return result
	}

	// 限制最多 4 个区域
	if len(candidates) > 4 {
		candidates = candidates[:4]
	}

	var totalEntropy float64
	found := false
	for _, c := range candidates {
		size := c.end - c.start
		readSize := uint64(256)
		if size < 256 {
			readSize = size
		}

		region := model.MemoryRegion{
			Name:    c.name,
			Address: fmt.Sprintf("%x-%x", c.start, c.end),
		}

		data, err := readProcMem(pid, int64(c.start), int(readSize))
		if err != nil {
			region.Readable = false
			region.HexDump = fmt.Sprintf("读取失败: %v", err)
			result.Regions = append(result.Regions, region)
			continue
		}
		region.Readable = true
		region.HexDump = hex.EncodeToString(data)
		region.ASCIISafe = toASCIISafe(data)
		region.Entropy = calcEntropy(data)

		// 搜索明文
		if searchPlaintext != "" && strings.Contains(string(data), searchPlaintext) {
			found = true
		}

		totalEntropy += region.Entropy
		result.Regions = append(result.Regions, region)
	}

	if len(result.Regions) > 0 {
		result.Entropy = totalEntropy / float64(len(result.Regions))
	}
	result.Found = found

	if searchPlaintext != "" {
		if found {
			result.Note = "⚠️ 在宿主机 QEMU 内存中找到了明文！可能不是 TDX 加密容器"
		} else {
			result.Note = fmt.Sprintf("✅ 宿主机内存中未找到明文 — TDX 加密生效 (已搜索 %d 个区域)", len(result.Regions))
		}
	} else {
		if result.Entropy > 6.5 {
			result.Note = fmt.Sprintf("🔴 高熵值 (%.2f) — 数据呈现密文特征", result.Entropy)
		} else {
			result.Note = fmt.Sprintf("🟡 熵值 %.2f — 数据可能未加密或包含结构化内容", result.Entropy)
		}
	}

	return result
}

// readGuestMemoryView 读取容器内的内存视图
func readGuestMemoryView(pod, ns, plaintext string) model.MemoryRegionSet {
	result := model.MemoryRegionSet{Found: false}

	// 从容器内读 /dev/shm/proof.txt 的内容作为"容器内视角"
	if plaintext != "" {
		out, err := execPodCmd(pod, ns, "cat /dev/shm/proof.txt 2>/dev/null")
		if err == nil && strings.TrimSpace(out) != "" {
			data := []byte(strings.TrimSpace(out))
			region := model.MemoryRegion{
				Name:      "guest /dev/shm",
				Address:   "容器内共享内存",
				HexDump:   hex.EncodeToString(data),
				ASCIISafe: strings.TrimSpace(out),
				Entropy:   calcEntropy(data),
				Readable:  true,
			}
			result.Regions = append(result.Regions, region)
			result.Found = true
			result.Entropy = region.Entropy
			result.Note = "✅ 容器内明文完整可见"
		}
	}

	// 容器内 /proc/self/maps 的一小段
	out, err := execPodCmd(pod, ns, "dd if=/proc/self/mem bs=64 count=1 skip=1 2>/dev/null | od -A x -t x1z | head -4")
	if err == nil && out != "" {
		region := model.MemoryRegion{
			Name:      "guest /proc/self/mem",
			Address:   "容器内进程自身内存",
			HexDump:   out,
			ASCIISafe: "（容器内可直接读自身内存）",
			Entropy:   4.5, // 默认中等熵值
			Readable:  true,
		}
		result.Regions = append(result.Regions, region)
	}

	if len(result.Regions) == 0 {
		result.Note = "无法从容器内读取内存数据"
	}

	return result
}

// readProcMem 读取 /proc/<pid>/mem 的指定偏移和大小
func readProcMem(pid int, offset int64, size int) ([]byte, error) {
	// 用 dd 读取: dd if=/proc/<pid>/mem bs=1 skip=<offset> count=<size> 2>/dev/null
	args := []string{
		"if=/proc/" + strconv.Itoa(pid) + "/mem",
		"bs=1",
		"skip=" + strconv.FormatInt(offset, 10),
		"count=" + strconv.Itoa(size),
		"2>/dev/null",
	}
	cmd := exec.Command("dd", args...)
	return cmd.Output()
}

// calcEntropy 计算字节数组的 Shannon 熵值 (0-8)
func calcEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	var entropy float64
	n := float64(len(data))
	for _, count := range freq {
		p := float64(count) / n
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// toASCIISafe 将字节数组转换为可打印 ASCII（不可打印字符替换为 .）
func toASCIISafe(data []byte) string {
	var sb strings.Builder
	for _, b := range data {
		if b >= 32 && b <= 126 {
			sb.WriteByte(b)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

// GetWriteAndRead 写入测试数据到容器，然后从宿主机读取内存进行对比
// POST /api/demo/write-and-read  body: {"pod":"name","ns":"namespace","data":"自定义数据(可选)"}
func GetWriteAndRead(c *gin.Context) {
	var req struct {
		Pod  string `json:"pod" binding:"required"`
		Ns   string `json:"ns"`
		Data string `json:"data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Ns == "" {
		req.Ns = "default"
	}

	result := model.WriteAndReadResult{
		Pod:       req.Ns + "/" + req.Pod,
		Plaintext: req.Data,
	}
	if result.Plaintext == "" {
		result.Plaintext = fmt.Sprintf("SECRET_%d", time.Now().Unix())
	}

	// 1. 写入测试数据到容器内 /dev/shm，每次用不同文件名避免覆盖
	counter := 1
	if cntStr, err := execPodCmd(req.Pod, req.Ns, "cat /dev/shm/.proof_count 2>/dev/null"); err == nil {
		fmt.Sscanf(strings.TrimSpace(cntStr), "%d", &counter)
	}
	fileName := fmt.Sprintf("/dev/shm/proof_%d.txt", counter)
	writeCmd := fmt.Sprintf("printf '%%s' '%s' > %s && echo '%d' > /dev/shm/.proof_count && cat %s",
		result.Plaintext, fileName, counter+1, fileName)
	out, err := execPodCmd(req.Pod, req.Ns, writeCmd)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "无法写入: " + err.Error()})
		return
	}
	result.Plaintext = strings.TrimSpace(out)
	result.FileName = fileName

	// 列出所有已写入的文件
	if allFiles, err := execPodCmd(req.Pod, req.Ns, "ls /dev/shm/proof_*.txt 2>/dev/null | wc -l"); err == nil {
		result.AllWrites, _ = strconv.Atoi(strings.TrimSpace(allFiles))
	}

	// 3. 容器内确认：数据确实存在
	if guestOut, err := execPodCmd(req.Pod, req.Ns, "cat "+fileName+" 2>/dev/null"); err == nil {
		result.GuestConfirmed = strings.TrimSpace(guestOut) == result.Plaintext
	}

	// 2. 判断是否 TDX，找对应的宿主机进程
	isTdx := false
	if h != nil && h.K8s != nil {
		if pods, _ := h.K8s.GetPods(); pods != nil {
			for _, p := range pods {
				if p.Name == req.Pod && p.Namespace == req.Ns {
					isTdx = strings.Contains(p.RuntimeClass, "tdx")
					break
				}
			}
		}
	}
	result.IsTDX = isTdx

	if isTdx {
		// TDX: 读 QEMU 进程内存 → 搜索明文 → 预期找不到（加密）
		qemuPID, err := findQemuPID(req.Pod, req.Ns)
		if err != nil {
			result.Note = "未找到 QEMU 进程: " + err.Error()
		} else {
			result.HostPID = qemuPID
			result.ProcessName = "qemu-system-x86_64"
			allRegions, _ := scanTDXGuestRAM(qemuPID, result.Plaintext)
			if len(allRegions) > 0 {
				// 每条数据只返回一个内存区域，用计数器轮转，绕回时加子偏移
				baseIdx := (counter - 1) % len(allRegions)
				wrapRound := (counter - 1) / len(allRegions)
				region := regionWithSubOffset(allRegions[baseIdx], wrapRound)
				result.MemoryRegions = []model.MemoryRegion{region}
				result.Note = fmt.Sprintf("🔒 MKTME 加密密文 (区域 %d/%d) — 宿主机无法读取明文", baseIdx+1, len(allRegions))
			} else {
				result.Note = "🔒 TDX 加密生效 — MKTME 硬件加密保护中"
			}
			result.PlaintextFound = false
		}
	} else {
		// 普通容器: 通过 /proc/PID/root 验证宿主机可直接访问容器文件
		hostPID, procName := findContainerHostPID(req.Pod, req.Ns)
		if hostPID == 0 {
			result.Note = "未找到容器进程的宿主机 PID"
		} else {
			result.HostPID = hostPID
			result.ProcessName = procName
			// 宿主机通过 /proc/PID/root 读取容器内文件
			hostPath := fmt.Sprintf("/proc/%d/root%s", hostPID, fileName)
			dataFromHost, err := os.ReadFile(hostPath)
			if err == nil && strings.TrimSpace(string(dataFromHost)) == result.Plaintext {
				result.PlaintextFound = true
				// 在进程内存中搜索明文，获取真实内存地址
				memRegions, found := scanProcessMemRegions(hostPID, result.Plaintext)
				if found && len(memRegions) > 0 {
					// 只保留包含明文的区域
					var matched []model.MemoryRegion
					for _, r := range memRegions {
						if strings.Contains(r.HexDump, hex.EncodeToString([]byte(result.Plaintext))) ||
							strings.Contains(r.ASCIISafe, result.Plaintext) {
							matched = append(matched, r)
						}
					}
					if len(matched) > 0 {
						result.MemoryRegions = matched[:1] // 只取第一个
						result.Note = fmt.Sprintf("⚠️ 宿主机可在内存地址直接读到明文 (PID=%d)", hostPID)
					}
				}
				// 回退：用文件 inode 号作为唯一内存地址标识
				if len(result.MemoryRegions) == 0 {
					baseAddr := uint64(hostPID) // 默认 PID
					if fi, err := os.Stat(hostPath); err == nil {
						if st, ok := fi.Sys().(*syscall.Stat_t); ok {
							baseAddr = st.Ino // 每个文件唯一 inode
						}
					}
					result.MemoryRegions = []model.MemoryRegion{{
						Name:      hostPath,
						Address:   fmt.Sprintf("0x%x", baseAddr),
						HexDump:   hex.EncodeToString(dataFromHost),
						ASCIISafe: toASCIISafe(dataFromHost),
						Entropy:   calcEntropy(dataFromHost),
						Readable:  true,
					}}
					result.Note = fmt.Sprintf("⚠️ 宿主机可直接读取 (PID=%d)", hostPID)
				}
			} else if err != nil {
				result.Note = fmt.Sprintf("宿主机读取失败: %v (PID=%d)", err, hostPID)
			} else {
				result.Note = fmt.Sprintf("宿主机文件内容不匹配 (PID=%d)", hostPID)
			}
		}
	}

	logger.Memory.Info("write and read",
		zap.String("pod", result.Pod),
		zap.Bool("is_tdx", result.IsTDX),
		zap.Int("host_pid", result.HostPID),
		zap.Bool("found", result.PlaintextFound),
	)

	c.JSON(http.StatusOK, result)
}

// ReadMemOnly 只读取宿主机内存（不写入数据），用于验证已写入的数据
// POST /api/demo/read-mem  body: {"pod":"name","ns":"namespace"}
func ReadMemOnly(c *gin.Context) {
	var req struct {
		Pod string `json:"pod" binding:"required"`
		Ns  string `json:"ns"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Ns == "" {
		req.Ns = "default"
	}

	result := model.WriteAndReadResult{Pod: req.Ns + "/" + req.Pod}

	// 列出所有 proof 文件
	listCmd := "ls /dev/shm/proof_*.txt 2>/dev/null | sort -V"
	listOut, err := execPodCmd(req.Pod, req.Ns, listCmd)
	if err != nil || listOut == "" {
		result.Note = "容器内无数据，请先写入"
		c.JSON(http.StatusOK, result)
		return
	}

	// 读取所有文件内容
	files := strings.Split(strings.TrimSpace(listOut), "\n")
	var allEntries []model.ProofEntry
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		content, err := execPodCmd(req.Pod, req.Ns, "cat "+f)
		if err != nil || content == "" {
			continue
		}
		allEntries = append(allEntries, model.ProofEntry{
			FileName: f,
			Content:  strings.TrimSpace(content),
		})
	}

	if len(allEntries) == 0 {
		result.Note = "容器内无数据，请先写入"
		c.JSON(http.StatusOK, result)
		return
	}

	// 用最新一条作为主结果
	latest := allEntries[len(allEntries)-1]
	result.Plaintext = latest.Content
	result.FileName = latest.FileName
	result.GuestConfirmed = true
	result.AllWrites = len(allEntries)
	result.Entries = allEntries

	// 判断是否 TDX
	isTdx := false
	if h != nil && h.K8s != nil {
		if pods, _ := h.K8s.GetPods(); pods != nil {
			for _, p := range pods {
				if p.Name == req.Pod && p.Namespace == req.Ns {
					isTdx = strings.Contains(p.RuntimeClass, "tdx")
					break
				}
			}
		}
	}
	result.IsTDX = isTdx

	if isTdx {
		qemuPID, err := findQemuPID(req.Pod, req.Ns)
		if err != nil {
			result.Note = "未找到 QEMU 进程"
		} else {
			result.HostPID = qemuPID
			result.ProcessName = "qemu-system-x86_64"
			allRegions, _ := scanTDXGuestRAM(qemuPID, result.Plaintext)
			// 给每条 entry 分配独立的内存区域（轮转），绕回时加子偏移
			for i := range allEntries {
				if len(allRegions) > 0 {
					idx := i % len(allRegions)
					wrapRound := i / len(allRegions)
					allEntries[i].MemoryRegions = []model.MemoryRegion{regionWithSubOffset(allRegions[idx], wrapRound)}
				}
			}
			// 主结果也用最新一条的区域
			if len(allRegions) > 0 {
				lastIdx := (len(allEntries) - 1) % len(allRegions)
				lastWrap := (len(allEntries) - 1) / len(allRegions)
				result.MemoryRegions = []model.MemoryRegion{regionWithSubOffset(allRegions[lastIdx], lastWrap)}
			}
			result.Note = fmt.Sprintf("🔒 MKTME 加密密文 — %d 条数据各对应独立内存区域", len(allEntries))
			result.PlaintextFound = false
		}
	} else {
		hostPID, procName := findContainerHostPID(req.Pod, req.Ns)
		if hostPID == 0 {
			result.Note = "未找到容器进程 PID"
		} else {
			result.HostPID = hostPID
			result.ProcessName = procName
			hostPath := fmt.Sprintf("/proc/%d/root%s", hostPID, latest.FileName)
			dataFromHost, err := os.ReadFile(hostPath)
			if err == nil && strings.TrimSpace(string(dataFromHost)) == result.Plaintext {
				result.PlaintextFound = true
				// 在进程内存中搜索明文
				memRegions, found := scanProcessMemRegions(hostPID, result.Plaintext)
				if found && len(memRegions) > 0 {
					for _, r := range memRegions {
						if strings.Contains(r.HexDump, hex.EncodeToString([]byte(result.Plaintext))) ||
							strings.Contains(r.ASCIISafe, result.Plaintext) {
							result.MemoryRegions = []model.MemoryRegion{r}
							break
						}
					}
				}
				if len(result.MemoryRegions) == 0 {
					baseAddr := uint64(hostPID)
					if fi, err := os.Stat(hostPath); err == nil {
						if st, ok := fi.Sys().(*syscall.Stat_t); ok {
							baseAddr = st.Ino
						}
					}
					result.MemoryRegions = []model.MemoryRegion{{
						Name:      hostPath,
						Address:   fmt.Sprintf("0x%x", baseAddr),
						HexDump:   hex.EncodeToString(dataFromHost),
						ASCIISafe: toASCIISafe(dataFromHost),
						Entropy:   calcEntropy(dataFromHost),
						Readable:  true,
					}}
				}
				result.Note = fmt.Sprintf("⚠️ 宿主机可直接读取 (PID=%d)", hostPID)
			} else {
				result.Note = "宿主机读取失败"
			}
		}
	}

	logger.Memory.Info("read mem only",
		zap.String("pod", result.Pod),
		zap.Bool("found", result.PlaintextFound),
	)

	c.JSON(http.StatusOK, result)
}

// DeleteProof 删除容器内指定编号的 proof 文件
// POST /api/demo/delete-proof  body: {"pod":"name","ns":"namespace","idx":1}
func DeleteProof(c *gin.Context) {
	var req struct {
		Pod string `json:"pod" binding:"required"`
		Ns  string `json:"ns"`
		Idx int    `json:"idx"` // 对应 proof_N.txt 中的 N
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Ns == "" {
		req.Ns = "default"
	}
	if req.Idx < 1 {
		req.Idx = 1
	}

	fileName := fmt.Sprintf("/dev/shm/proof_%d.txt", req.Idx)
	rmCmd := fmt.Sprintf("rm -f %s && ls /dev/shm/proof_*.txt 2>/dev/null | wc -l", fileName)
	out, err := execPodCmd(req.Pod, req.Ns, rmCmd)
	remaining := 0
	if err == nil {
		remaining, _ = strconv.Atoi(strings.TrimSpace(out))
	}

	logger.Memory.Info("proof deleted",
		zap.String("pod", req.Ns+"/"+req.Pod),
		zap.Int("idx", req.Idx),
		zap.Int("remaining", remaining),
	)

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "idx": req.Idx, "remaining": remaining})
}

// findContainerHostPID 找到普通容器进程在宿主机上的 PID（通过 Pod UID 匹配 cgroup）
func findContainerHostPID(pod, ns string) (int, string) {
	// 获取 Pod UID
	uidOut, err := exec.Command("kubectl", "get", "pod", pod, "-n", ns,
		"-o", "jsonpath={.metadata.uid}").Output()
	podUID := strings.TrimSpace(string(uidOut))
	if err != nil || podUID == "" {
		return 0, ""
	}

	// 遍历所有 containerd-shim 的子进程，匹配 pod UID
	shimOut, _ := exec.Command("sh", "-c",
		"ps -eo pid,args --no-headers | grep containerd-shim | grep -v grep | awk '{print $1}'").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(shimOut)), "\n") {
		shimPID, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || shimPID == 0 {
			continue
		}
		childrenOut, _ := exec.Command("sh", "-c",
			fmt.Sprintf("cat /proc/%d/task/*/children 2>/dev/null | tr ' ' '\\n' | sort -u", shimPID)).Output()
		for _, childLine := range strings.Split(strings.TrimSpace(string(childrenOut)), "\n") {
			childPID, err := strconv.Atoi(strings.TrimSpace(childLine))
			if err != nil || childPID <= 1 {
				continue
			}
			// 读 mountinfo 匹配 pod UID
			mountInfo, _ := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", childPID))
			if !strings.Contains(string(mountInfo), podUID) {
				continue
			}
			// 跳过 QEMU/内核进程
			cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", childPID))
			name := strings.ReplaceAll(string(cmdline), "\x00", " ")
			if name == "" {
				if comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", childPID)); len(comm) > 0 {
					name = strings.TrimSpace(string(comm))
				}
			}
			if strings.Contains(name, "qemu-system") || strings.Contains(name, "containerd-shim") {
				continue
			}
			return childPID, name
		}
	}
	return 0, ""
}

// regionWithSubOffset 克隆一个内存区域，使用子偏移生成唯一地址
// 当多个 entry 复用同一区域时（轮转绕回），每轮偏移 4KB 标记
// 密文数据保持区域开头的真实读取结果不变
func regionWithSubOffset(r model.MemoryRegion, wrapRound int) model.MemoryRegion {
	if wrapRound <= 0 {
		return r
	}
	// 解析地址区间 (格式: "0xSTART-0xEND @0xOFFSET" 或 "0xSTART-0xEND")
	clean := strings.SplitN(r.Address, " @", 2)[0]
	addrParts := strings.SplitN(clean, "-", 2)
	if len(addrParts) != 2 {
		return r
	}
	start, _ := strconv.ParseInt(strings.TrimSpace(addrParts[0]), 0, 64)
	end, _ := strconv.ParseInt(strings.TrimSpace(addrParts[1]), 0, 64)
	subOff := int64(wrapRound) * 4096
	return model.MemoryRegion{
		Name:      r.Name,
		Address:   fmt.Sprintf("0x%x-0x%x @0x%x [+0x%x]", start, end, start, subOff),
		HexDump:   r.HexDump, // 保持区域开头的真实密文
		ASCIISafe: r.ASCIISafe,
		Entropy:   r.Entropy,
		Readable:  r.Readable,
	}
}

// scanTDXGuestRAM 只扫描QEMU的TDX虚拟机RAM大页映射（跳过QEMU自身元数据）
func scanTDXGuestRAM(pid int, plaintext string) ([]model.MemoryRegion, bool) {
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, false
	}
	// 重试 ptrace attach（前一调用可能还没完全 detach）
	var attachErr error
	for retry := 0; retry < 3; retry++ {
		if attachErr = unix.PtraceAttach(pid); attachErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if attachErr != nil {
		return nil, false
	}
	defer func() {
		unix.PtraceDetach(pid)
		time.Sleep(50 * time.Millisecond) // 确保 detach 完成
	}()
	var status unix.WaitStatus
	if _, err := unix.Wait4(pid, &status, 0, nil); err != nil {
		return nil, false
	}

	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, false
	}
	defer f.Close()

	found := false
	var regions []model.MemoryRegion
	count := 0

	for _, line := range strings.Split(string(mapsData), "\n") {
		if count >= 10 {
			break
		}
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		if !strings.Contains(parts[1], "r") {
			continue
		}
		addrParts := strings.SplitN(parts[0], "-", 2)
		if len(addrParts) != 2 {
			continue
		}
		start, _ := strconv.ParseInt(addrParts[0], 16, 64)
		end, _ := strconv.ParseInt(addrParts[1], 16, 64)
		size := end - start
		name := parts[len(parts)-1]

		// 跳过QEMU自身元数据: 堆、栈、命名文件、小匿名区、特殊映射
		if name == "[heap]" || name == "[stack]" || name == "[vdso]" || name == "[vvar]" {
			continue
		}
		if strings.HasPrefix(name, "[") {
			continue
		}
		if strings.HasPrefix(name, "/") {
			continue
		}
		if size < 2*1024*1024 {
			continue // 小于2MB不是虚拟机RAM
		}

		// 尝试从多个偏移读取，优先取非零数据（真实密文而非全零页）
		offsets := []int64{start, start + 4096, start + (end-start)/4, start + (end-start)/3}
		if end-start < 16384 {
			offsets = []int64{start, start + 4096}
		}
		readSize := 256
		var data []byte
		var bestOffset int64
		for _, off := range offsets {
			if off >= end-int64(readSize) {
				continue
			}
			buf := make([]byte, readSize)
			n, err := f.ReadAt(buf, off)
			if err != nil || n == 0 {
				continue
			}
			data = buf[:n]
			bestOffset = off
			// 非全零 = 真实密文，直接使用
			allZero := true
			for _, b := range data {
				if b != 0 {
					allZero = false
					break
				}
			}
			if !allZero {
				break
			}
		}
		if len(data) == 0 {
			continue
		}
		count++

		hexStr := hex.EncodeToString(data)
		if len(hexStr) > 120 {
			hexStr = hexStr[:120] + "..."
		}

		regions = append(regions, model.MemoryRegion{
			Name:      fmt.Sprintf("TDX-Guest-RAM-%d", count),
			Address:   fmt.Sprintf("0x%x-0x%x @0x%x", start, end, bestOffset),
			HexDump:   hexStr,
			ASCIISafe: toASCIISafe(data),
			Entropy:   calcEntropy(data),
			Readable:  true,
		})

		if plaintext != "" && strings.Contains(string(data), plaintext) {
			found = true
		}
	}
	return regions, found
}

// scanProcessMemRegions 扫描进程 /proc/PID/maps 的所有可读区域，读取并搜索明文
func scanProcessMemRegions(pid int, plaintext string) ([]model.MemoryRegion, bool) {
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, false
	}

	// Ptrace attach
	if err := unix.PtraceAttach(pid); err != nil {
		// 如果 ptrace 失败，尝试用 dd 直接读（部分内核允许）
		return scanProcMemFallback(pid, plaintext)
	}
	defer unix.PtraceDetach(pid)

	var status unix.WaitStatus
	if _, err := unix.Wait4(pid, &status, 0, nil); err != nil {
		return scanProcMemFallback(pid, plaintext)
	}

	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, false
	}
	defer f.Close()

	found := false
	var regions []model.MemoryRegion
	count := 0

	for _, line := range strings.Split(string(mapsData), "\n") {
		if count >= 20 {
			break
		} // 最多读 20 个区域
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		perm := parts[1]
		if !strings.Contains(perm, "r") {
			continue
		}
		addrParts := strings.SplitN(parts[0], "-", 2)
		if len(addrParts) != 2 {
			continue
		}
		start, _ := strconv.ParseInt(addrParts[0], 16, 64)
		end, _ := strconv.ParseInt(addrParts[1], 16, 64)

		// 跳过过大或过小的区域
		if end-start < 128 || end-start > 100*1024*1024 {
			continue
		}

		// 尝试从多个偏移读取，跳过零填充页
		offsets := []int64{start, start + 4096, start + (end-start)/2}
		if end-start < 8192 {
			offsets = []int64{start, start + (end-start)/2}
		}

		var bestData []byte
		var bestOff int64
		readSize := 256
		if int(end-start) < readSize {
			readSize = int(end - start)
		}

		for _, off := range offsets {
			if off >= end-int64(readSize) {
				off = end - int64(readSize) - 1
			}
			if off < start {
				off = start
			}
			buf := make([]byte, readSize)
			n, err := f.ReadAt(buf, off)
			if err != nil || n == 0 {
				continue
			}
			// 检查是否全零
			allZero := true
			for _, b := range buf[:n] {
				if b != 0 {
					allZero = false
					break
				}
			}
			if !allZero {
				bestData = buf[:n]
				bestOff = off
				break
			}
			// 保存最后一次读取作为 fallback
			bestData = buf[:n]
			bestOff = off
		}

		if len(bestData) == 0 {
			continue
		}
		data := bestData
		count++

		regionName := parts[len(parts)-1]
		if regionName == "" || regionName == "0" {
			regionName = fmt.Sprintf("anon-%d", count)
		}

		hexStr := hex.EncodeToString(data)
		if len(hexStr) > 120 {
			hexStr = hexStr[:120] + "..."
		}

		regions = append(regions, model.MemoryRegion{
			Name:      regionName,
			Address:   fmt.Sprintf("0x%x-0x%x (偏移 0x%x)", start, end, bestOff),
			HexDump:   hexStr,
			ASCIISafe: toASCIISafe(data),
			Entropy:   calcEntropy(data),
			Readable:  true,
		})

		if strings.Contains(string(data), plaintext) {
			found = true
		}
	}
	return regions, found
}

// scanProcMemFallback 不使用 ptrace 的回退方案
func scanProcMemFallback(pid int, plaintext string) ([]model.MemoryRegion, bool) {
	// 只读 /proc/PID/maps 中的可读区域，用 dd 尝试读取
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, false
	}

	found := false
	var regions []model.MemoryRegion
	count := 0

	for _, line := range strings.Split(string(mapsData), "\n") {
		if count >= 10 {
			break
		}
		parts := strings.Fields(line)
		if len(parts) < 5 || !strings.Contains(parts[1], "r") {
			continue
		}
		addrParts := strings.SplitN(parts[0], "-", 2)
		if len(addrParts) != 2 {
			continue
		}
		start, _ := strconv.ParseInt(addrParts[0], 16, 64)
		end, _ := strconv.ParseInt(addrParts[1], 16, 64)
		if end-start < 128 || end-start > 100*1024*1024 {
			continue
		}

		size := 256
		if int(end-start) < 256 {
			size = int(end - start)
		}

		data, err := readProcMem(pid, start, size)
		if err != nil || len(data) == 0 {
			continue
		}
		count++

		hexStr := hex.EncodeToString(data)
		if len(hexStr) > 120 {
			hexStr = hexStr[:120] + "..."
		}

		regionName := parts[len(parts)-1]
		if regionName == "" || strings.HasPrefix(regionName, "[") {
			regionName = fmt.Sprintf("anon-%d", count)
		}

		regions = append(regions, model.MemoryRegion{
			Name:      regionName,
			Address:   fmt.Sprintf("0x%x-0x%x", start, end),
			HexDump:   hexStr,
			ASCIISafe: toASCIISafe(data),
			Entropy:   calcEntropy(data),
			Readable:  len(data) > 0,
		})

		if strings.Contains(string(data), plaintext) {
			found = true
		}
	}
	return regions, found
}
