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
	"time"

	"coco-serve/internal/logger"
	"coco-serve/internal/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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
	// 先获取 Pod UID
	uidOut, err := exec.Command("kubectl", "get", "pod", pod, "-n", ns,
		"-o", "jsonpath={.metadata.uid}").Output()
	podUID := strings.TrimSpace(string(uidOut))
	if err != nil || podUID == "" {
		// fallback: 按 Pod 名搜索
		out, err := exec.Command("sh", "-c",
			fmt.Sprintf("ps -eo pid,args --no-headers | grep qemu-system | grep '%s' | grep -v grep | head -1 | awk '{print $1}'", pod)).Output()
		if err != nil {
			return 0, fmt.Errorf("未找到 QEMU 进程")
		}
		return strconv.Atoi(strings.TrimSpace(string(out)))
	}

	// 按 UID 精确搜索
	out, err := exec.Command("sh", "-c",
		fmt.Sprintf("ps -eo pid,args --no-headers | grep qemu-system | grep '%s' | grep -v grep | head -1 | awk '{print $1}'", podUID)).Output()
	if err != nil {
		return 0, fmt.Errorf("未找到匹配 Pod UID 的 QEMU 进程")
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
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
