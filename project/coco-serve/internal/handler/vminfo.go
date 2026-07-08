package handler

import (
	"net/http"
	"strconv"

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
