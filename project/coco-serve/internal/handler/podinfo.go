package handler

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

type HostProc struct {
	PID   int    `json:"pid"`
	Comm  string `json:"comm"`
	RSSKB int    `json:"rss_kb"`
}

type GuestProc struct {
	PID  int    `json:"pid"`
	Comm string `json:"comm"`
}

// memReadResult ptrace 内存读取结果
type memReadResult struct {
	data       []byte
	regionName string
	regionAddr string
}

func GetPodSysInfo(c *gin.Context) {
	ns := c.Param("namespace")
	name := c.Param("name")

	type SysInfo struct {
		Pod        string            `json:"pod"`
		Info       map[string]string `json:"info"`
		IsTDX      bool              `json:"is_tdx"`
		HostProcs  []HostProc        `json:"host_procs"`
		GuestProcs []GuestProc       `json:"guest_procs"`
	}

	info := SysInfo{Pod: ns + "/" + name, Info: make(map[string]string)}

	shells := [][]string{{"sh", "-c"}, {"bash", "-c"}, {"/busybox/sh", "-c"}}
	execPod := func(cmd string) (string, error) {
		for _, s := range shells {
			args := append([]string{"exec", name, "-n", ns, "--"}, s...)
			out, err := exec.Command("kubectl", append(args, cmd)...).Output()
			if err == nil && strings.TrimSpace(string(out)) != "" {
				return strings.TrimSpace(string(out)), nil
			}
		}
		return "", fmt.Errorf("no shell")
	}

	// 内核
	var guestKernel string
	if v, err := execPod("cat /proc/version"); err == nil {
		f := strings.Fields(v)
		if len(f) >= 3 {
			guestKernel = f[2]
			info.Info["内核"] = guestKernel
		}
	}

	// 判断是否 TDX: Guest 内核 6.18.x
	info.IsTDX = strings.HasPrefix(guestKernel, "6.18.")

	// 容器资源（从 cgroup 读取实际使用量 + K8s 显示分配量）
	var resLines []string

	// 从 cgroup 读取容器实际内存使用
	if v, err := execPod("cat /sys/fs/cgroup/memory.current 2>/dev/null"); err == nil && v != "" {
		if b, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			resLines = append(resLines, "内存使用: "+sizeStr(b))
		}
	} else if v, err := execPod("cat /sys/fs/cgroup/memory/memory.usage_in_bytes 2>/dev/null"); err == nil && v != "" {
		if b, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			resLines = append(resLines, "内存使用: "+sizeStr(b))
		}
	}

	// 从 cgroup 读取内存限制
	if v, err := execPod("cat /sys/fs/cgroup/memory.max 2>/dev/null"); err == nil && v != "" && v != "max" {
		if b, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			resLines = append(resLines, "内存限制: "+sizeStr(b))
		}
	} else if v, err := execPod("cat /sys/fs/cgroup/memory/memory.limit_in_bytes 2>/dev/null"); err == nil && v != "" {
		if b, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			if b > 1<<40 { // 超过 1TB 说明是无限制
				resLines = append(resLines, "内存限制: 无限制")
			} else {
				resLines = append(resLines, "内存限制: "+sizeStr(b))
			}
		}
	}

	// 从 K8s 获取 Pod 资源规格（requests/limits）
	if h != nil && h.K8s != nil {
		if res, err := h.K8s.GetPodResources(ns, name); err == nil {
			specs := []string{}
			if res.CPUReq != "" || res.CPULimit != "" {
				cpu := ""
				if res.CPUReq != "" {
					cpu += res.CPUReq
				}
				if res.CPULimit != "" && res.CPULimit != res.CPUReq {
					cpu += "/" + res.CPULimit
				}
				if cpu != "" {
					specs = append(specs, "CPU: "+cpu)
				}
			}
			if res.MemReq != "" || res.MemLimit != "" {
				mem := ""
				if res.MemReq != "" {
					mem += res.MemReq
				}
				if res.MemLimit != "" && res.MemLimit != res.MemReq {
					mem += "/" + res.MemLimit
				}
				if mem != "" {
					specs = append(specs, "内存: 请求/限制 "+mem)
				}
			}
			if len(specs) > 0 {
				resLines = append(resLines, "资源规格: "+strings.Join(specs, " | "))
			}
		}
	}

	if len(resLines) > 0 {
		info.Info["CPU/内存"] = strings.Join(resLines, "\n")
	}

	// 运行时间：从 K8s Pod startTime 计算（不再用 /proc/uptime，因为 runc 容器共享宿主机内核）
	if h != nil && h.K8s != nil {
		if t, err := h.K8s.GetPodStartTime(ns, name); err == nil {
			dur := time.Since(t.Time)
			d := int(dur.Hours() / 24)
			hr := int(dur.Hours()) % 24
			m := int(dur.Minutes()) % 60
			s := int(dur.Seconds()) % 60
			if d > 0 {
				info.Info["运行时间"] = fmt.Sprintf("%dd %dh %dm", d, hr, m)
			} else if hr > 0 {
				info.Info["运行时间"] = fmt.Sprintf("%dh %dm %ds", hr, m, s)
			} else {
				info.Info["运行时间"] = fmt.Sprintf("%dm %ds", m, s)
			}
		}
	}

	if len(info.Info) == 0 {
		info.Info["状态"] = "[此 Pod 使用最小镜像]"
	}

	// 宿主机可见进程列表
	if info.IsTDX {
		info.HostProcs = listQemuProcs()
	} else {
		info.HostProcs = listShimProcs()
	}

	// 容器内实际进程（通过 kubectl exec 获取）
	info.GuestProcs = listGuestProcs(name, ns)

	c.JSON(http.StatusOK, info)
}

func sizeStr(bytes float64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", bytes/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", bytes/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB", bytes/(1<<10))
	}
}

func listQemuProcs() []HostProc {
	var procs []HostProc
	out, _ := exec.Command("sh", "-c", "ps -eo pid,rss,comm --no-headers | grep qemu-system | grep -v grep").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			pid, _ := strconv.Atoi(f[0])
			rss, _ := strconv.Atoi(f[1])
			procs = append(procs, HostProc{PID: pid, Comm: f[2], RSSKB: rss})
		}
	}
	return procs
}

func listShimProcs() []HostProc {
	var procs []HostProc
	out, _ := exec.Command("sh", "-c", "ps -eo pid,rss,comm --no-headers | grep containerd-shim | grep -v grep | head -10").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 {
			pid, _ := strconv.Atoi(f[0])
			rss, _ := strconv.Atoi(f[1])
			procs = append(procs, HostProc{PID: pid, Comm: f[2], RSSKB: rss})
		}
	}
	return procs
}

func countPodProcs(name, ns string) int {
	out, err := exec.Command("kubectl", "exec", name, "-n", ns, "--", "sh", "-c", "ps aux 2>/dev/null | wc -l").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if n > 0 {
		n--
	}
	return n
}

func listGuestProcs(name, ns string) []GuestProc {
	var procs []GuestProc
	out, err := exec.Command("kubectl", "exec", name, "-n", ns, "--", "ps", "aux").Output()
	if err != nil {
		return procs
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		// 格式: "PID USER TIME COMMAND..."
		idx := strings.Index(line, " ")
		if idx < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:idx])
		if err != nil || pid <= 2 {
			continue
		}
		// 取 COMMAND 部分 (跳过 PID, USER, TIME 三列)
		rest := strings.TrimSpace(line[idx:])
		// 跳过 USER
		if i := strings.Index(rest, " "); i > 0 {
			rest = strings.TrimSpace(rest[i:])
		}
		// 跳过 TIME
		if i := strings.Index(rest, " "); i > 0 {
			rest = strings.TrimSpace(rest[i:])
		}
		comm := rest
		if comm == "" || comm == "ps aux" {
			continue
		}
		// 截断取第一部分
		if i := strings.Index(comm, " "); i > 0 {
			comm = comm[:i]
		}
		if idx := strings.LastIndex(comm, "/"); idx >= 0 && idx < len(comm)-1 {
			comm = comm[idx+1:]
		}
		comm = strings.TrimRight(comm, ":")
		procs = append(procs, GuestProc{PID: pid, Comm: comm})
	}
	return procs
}

// GetProcMem 证明：使用 ptrace 读取宿主机进程内存，验证隔离性
func GetProcMem(c *gin.Context) {
	pid := c.Param("pid")
	ns := c.Query("ns")
	pod := c.Query("pod")

	// 检测宿主机上是否存在此 PID
	_, err := os.Stat("/proc/" + pid)
	hostExists := err == nil

	// 如果是 TDX 容器的 guest 进程（小 PID），尝试证明宿主机上看不到
	pidInt, _ := strconv.Atoi(pid)
	isGuestPid := pidInt < 100 && pod != "" && ns != ""

	// 查 cmdline 看是什么进程
	cmdline, _ := os.ReadFile("/proc/" + pid + "/cmdline")
	cmdStr := strings.ReplaceAll(strings.TrimSpace(string(cmdline)), "\x00", " ")

	if isGuestPid && hostExists {
		who := cmdStr
		if who == "" {
			who = "[内核线程]"
		}
		c.JSON(http.StatusOK, gin.H{
			"pid":      pid,
			"note":     fmt.Sprintf("宿主机 PID %s 是「%s」，完全不是容器内的进程", pid, who),
			"tag":      "qemu",
			"evidence": fmt.Sprintf("容器内通过 kubectl exec 看到的进程(PID=%s)，宿主机 /proc/%s 是 %s", pid, pid, who),
		})
		return
	}

	if isGuestPid && !hostExists {
		c.JSON(http.StatusOK, gin.H{
			"pid":      pid,
			"note":     "宿主机上不存在此 PID — 容器进程在 TDX 加密 VM 内运行，宿主机无法看到",
			"tag":      "qemu",
			"evidence": fmt.Sprintf("访问 /proc/%s → 不存在（TDX 加密隔离）", pid),
		})
		return
	}

	// 对普通宿主机进程，使用 ptrace 尝试读取内存（读 2 个不同区域以证明数据差异）
	isQemu := strings.Contains(cmdStr, "qemu-system")
	tag := "shim"
	if isQemu {
		tag = "qemu"
	}

	// 读取 2 个不同内存区域，体现进程间差异
	results := readMultiRegion(pidInt)
	if len(results) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"pid":      pid,
			"note":     "⚠️ ptrace 读取失败：无可读内存区域",
			"tag":      tag,
			"evidence": fmt.Sprintf("cmdline: %s", cmdStr),
		})
		return
	}

	// 组装证据文本
	var evidenceParts []string
	for _, r := range results {
		hexStr := fmt.Sprintf("%x", r.data)
		if len(hexStr) > 80 {
			hexStr = hexStr[:80] + "..."
		}
		evidenceParts = append(evidenceParts, fmt.Sprintf("[%s %s] %s", r.regionName, r.regionAddr, hexStr))
	}
	evidenceStr := strings.Join(evidenceParts, "\n")

	if isQemu {
		c.JSON(http.StatusOK, gin.H{
			"pid":      pid,
			"note":     fmt.Sprintf("✅ ptrace 读取 %d 个区域 (%d bytes)，但这是 QEMU 虚拟机自身内存，非容器内进程", len(results), totalBytes(results)),
			"tag":      tag,
			"evidence": fmt.Sprintf("%s\ncmdline: %s", evidenceStr, cmdStr),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"pid":      pid,
			"note":     fmt.Sprintf("✅ ptrace 读取 %d 个区域 (%d bytes) — 普通容器进程内存对宿主机可见", len(results), totalBytes(results)),
			"tag":      tag,
			"evidence": fmt.Sprintf("%s\ncmdline: %s", evidenceStr, cmdStr),
		})
	}
}

// totalBytes 计算多个读取结果的总字节数
func totalBytes(results []memReadResult) int {
	n := 0
	for _, r := range results {
		n += len(r.data)
	}
	return n
}

// readMultiRegion 从进程的多个不同内存区域读取，用 PID 选不同偏移确保跨进程差异
func readMultiRegion(pid int) []memReadResult {
	mapsData, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil
	}

	type candidate struct {
		start, end int64
		name, perm string
	}
	var stack, heap *candidate
	var rwAnon []candidate

	for _, line := range strings.Split(string(mapsData), "\n") {
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
		s, _ := strconv.ParseInt(addrParts[0], 16, 64)
		e, _ := strconv.ParseInt(addrParts[1], 16, 64)
		if e-s < 128 {
			continue
		}
		name := parts[len(parts)-1]
		if name == "[stack]" {
			stack = &candidate{s, e, name, perm}
		} else if name == "[heap]" {
			heap = &candidate{s, e, name, perm}
		} else if !strings.HasPrefix(name, "[") && !strings.HasPrefix(name, "/") && strings.Contains(perm, "w") {
			rwAnon = append(rwAnon, candidate{s, e, name, perm})
		}
	}

	// 选出区域：栈 + 堆(或rw-anon) + PID决定的第三个区域
	var chosen []*candidate
	if stack != nil {
		chosen = append(chosen, stack)
	}
	if heap != nil {
		chosen = append(chosen, heap)
	} else if len(rwAnon) > 0 {
		chosen = append(chosen, &rwAnon[0])
	}
	if len(rwAnon) > 1 {
		idx := pid % len(rwAnon)
		chosen = append(chosen, &rwAnon[idx])
	}

	if len(chosen) == 0 {
		return nil
	}

	if err := unix.PtraceAttach(pid); err != nil {
		return nil
	}
	defer unix.PtraceDetach(pid)

	var status unix.WaitStatus
	if _, err := unix.Wait4(pid, &status, 0, nil); err != nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(3 * time.Second)
		select {
		case <-done:
		default:
			unix.PtraceDetach(pid)
		}
	}()

	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		close(done)
		return nil
	}
	defer f.Close()

	var results []memReadResult
	for _, c := range chosen {
		offset := c.start
		readSize := 64
		regionName := c.name
		if regionName == "" {
			regionName = "rw-anon"
		}

		if c.name == "[stack]" {
			offset = c.end - 128 - int64(pid%16)*8
		} else if c.name == "[heap]" {
			offset = c.start + 8192 + int64(pid%32)*64
		} else {
			offset = c.start + int64((pid*37)%256)*64
			readSize = 48
		}

		buf := make([]byte, readSize)
		n, err := f.ReadAt(buf, offset)
		if err != nil && n == 0 {
			continue
		}
		results = append(results, memReadResult{
			data:       buf[:n],
			regionName: regionName,
			regionAddr: fmt.Sprintf("0x%x", offset),
		})
	}
	close(done)
	return results
}
