# TDX 机密计算可视化面板 — 项目方案

> 定位：提供方案，非提供服务
> 产出：可复用的技术方案 + Demo 代码

---

## 一、项目目标

构建一个 TDX 机密计算环境的可视化展示平台，将当前环境的硬件能力、容器运行状态、证明链路以页面形式呈现。

## 二、技术方案

### 架构

```
┌──────────────────────────────────┐
│  Svelte + TS 前端                 │
│  5 个页面：总览/容器/编排/证明/对比 │
└──────────────┬───────────────────┘
               │ REST API
               ▼
┌──────────────────────────────────┐
│  Go Web 后端 (Gin)                │
│  ├── K8s API 数据采集             │
│  └── 系统命令数据采集              │
└──────────────────────────────────┘
```

### 部署方式

单二进制部署：Go 编译后内嵌前端静态文件，一个文件跑起来。

## 三、页面规划

| 页面 | 展示内容 | 数据来源 |
|------|---------|---------|
| 📊 **总览** | TDX 状态、节点信息、运行中 Pod 概览 | K8s API + 系统命令 |
| 🖥️ **机密容器** | kata-qemu-tdx Pod 列表、Guest 内核、QEMU 进程 | K8s API + ps |
| 🔄 **容器编排** | RuntimeClass 列表、节点亲和性、Deployment 拓扑 | K8s API |
| 🔐 **证明链** | Trustee 组件状态 (AS/KBS/RVPS)、证明流程展示 | K8s API |
| 📈 **效果对比** | 普通 vs 机密容器对比、硬件能力清单 | 静态+动态数据 |

## 四、数据收集清单

### 硬件层
- [x] TDX 模块状态 (`/sys/module/kvm_intel/parameters/tdx`)
- [x] SGX 设备 (`/dev/sgx_*`)
- [x] CPU / 内存 / 磁盘

### 容器层
- [x] 机密 Pod 列表 (`kubectl get pods -o wide`)
- [x] Pod 详情 (RuntimeClass, Guest 内核, IP)
- [x] QEMU 进程 (`ps aux | grep qemu`)

### 编排层
- [x] RuntimeClass 列表 (`kubectl get runtimeclass`)
- [x] 节点状态 (`kubectl get nodes`)
- [x] 命名空间列表

### 证明层
- [x] Trustee 组件状态
- [x] AS / KBS / RVPS 服务地址

## 五、技术栈

| 层 | 选型 | 版本 |
|----|------|------|
| 前端框架 | Svelte 5 + TypeScript | latest |
| 后端 | Go + Gin | 1.26 |
| K8s SDK | client-go | v1.36 |
| 构建 | Go embed + Vite | — |

## 六、交付物

1. 📁 完整项目源码（可复用）
2. 📄 方案文档
3. 🔧 一键部署脚本
