# CoCo Serve — 机密计算观测与操作后端

> Go 后端服务，封装 K8s API + 系统命令，为前端 Svelte 面板提供 REST API + WebSocket。

---

## 一、架构总览

```
┌─────────────────────────────────────────────┐
│  Svelte 前端 (coco/)                          │
│  REST: /api/*    WS: /ws/*                    │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────┴──────────────────────────┐
│  CoCo Serve (本服务)                          │
│                                               │
│  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ handler/ │ │collector/│ │ k8sclient/   │  │
│  │HTTP路由  │ │数据采集   │ │ K8s API封装  │  │
│  └──────────┘ └──────────┘ └──────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ system/  │ │ model/   │ │ ws/           │  │
│  │宿主机命令│ │数据模型  │ │实时推送       │  │
│  └──────────┘ └──────────┘ └──────────────┘  │
└──────────────────┬──────────────────────────┘
                   │
    ┌──────────────┼──────────────┐
    ▼              ▼              ▼
 ┌──────┐   ┌──────────┐   ┌──────────┐
 │K8s   │   │ /proc    │   │ ps/dmesg │
 │API   │   │ /sys     │   │ / 系统命令│
 └──────┘   └──────────┘   └──────────┘
```

---

## 二、目录结构

```
coco-serve/
├── main.go              # 入口：启动 HTTP + WebSocket
├── go.mod / go.sum
│
├── internal/
│   ├── handler/          # HTTP 路由 + 请求处理
│   │   ├── router.go     #   路由注册
│   │   ├── overview.go   #   GET /api/overview
│   │   ├── pods.go       #   GET /api/pods
│   │   ├── runtime.go    #   GET /api/runtimes
│   │   ├── trustee.go    #   GET /api/trustee
│   │   └── compare.go    #   GET /api/compare
│   │
│   ├── k8sclient/        # K8s API 客户端封装
│   │   └── client.go     #   client-go 初始化 + Pod/Node/RuntimeClass 查询
│   │
│   ├── collector/        # 数据采集器（定时 + 命令执行）
│   │   ├── tdx.go        #   TDX 硬件状态采集
│   │   ├── sgx.go        #   SGX 状态采集
│   │   ├── qemu.go       #   QEMU 进程采集
│   │   └── trustee.go    #   Trustee 组件采集
│   │
│   ├── system/           # 宿主机系统命令封装
│   │   └── executor.go   #   安全命令执行器
│   │
│   ├── model/            # 数据模型定义
│   │   └── types.go      #   所有请求/响应/事件结构体
│   │
│   └── ws/               # WebSocket 实时推送
│       ├── hub.go        #   连接管理（广播/订阅）
│       └── client.go     #   单个客户端管理
│
└── embedded/              # 前端静态文件（构建时嵌入）
    └── dist/              #   Svelte 构建产物
```

---

## 三、API 设计

### 3.1 总览 — `GET /api/overview`

```json
{
  "tdx": {
    "enabled": true,
    "keyid_range": "32-64",
    "pamt_kb": 1050636,
    "module_initialized": true
  },
  "sgx": {
    "enabled": true,
    "devices": ["enclave", "provision", "vepc"]
  },
  "node": {
    "name": "localhost.localdomain",
    "os": "Fedora Linux 44",
    "kernel": "7.0.6-200.fc44.x86_64",
    "cpu_cores": 12,
    "memory_gib": 251,
    "arch": "amd64"
  },
  "k8s": {
    "version": "v1.36.1",
    "runtime": "containerd://2.2.3",
    "pods_total": 12,
    "pods_running": 9
  }
}
```

### 3.2 机密容器列表 — `GET /api/pods?runtime=kata-qemu-tdx`

```json
{
  "pods": [
    {
      "name": "tdx-nginx",
      "namespace": "default",
      "status": "Running",
      "ip": "10.244.0.140",
      "runtime_class": "kata-qemu-tdx",
      "guest_kernel": "6.18.28",
      "qemu_pid": 1433262,
      "qemu_rss_mb": 165,
      "started_at": "2026-07-03T22:49:00Z"
    }
  ],
  "total": 1
}
```

### 3.3 RuntimeClass 列表 — `GET /api/runtimes`

```json
{
  "runtimes": [
    {"name": "kata-qemu-tdx", "handler": "kata-qemu-tdx", "available": true},
    {"name": "kata-qemu-snp", "handler": "kata-qemu-snp", "available": false},
    {"name": "kata-qemu-cca", "handler": "kata-qemu-cca", "available": false}
  ],
  "available_count": 1,
  "total": 10
}
```

### 3.4 Trustee 证明链 — `GET /api/trustee`

```json
{
  "as": {"status": "Running", "endpoint": "10.103.38.252:50004"},
  "kbs": {"status": "Running", "endpoint": "10.108.50.200:8080"},
  "rvps": {"status": "Running", "endpoint": "10.103.172.41:50003"}
}
```

### 3.5 效果对比 — `GET /api/compare`

```json
{
  "comparisons": {
    "kernel": {"host": "7.0.6-200.fc44", "guest": "6.18.28"},
    "isolation": {"normal": "进程隔离", "confidential": "TDX 硬件加密"},
    "root_visible": {"normal": true, "confidential": false}
  },
  "running_pods": {
    "normal": 0,
    "confidential": 1
  }
}
```

### 3.6 实时 WebSocket — `/ws/state`

```
推送事件：
  { "type": "pod_update", "data": { "name":"tdx-nginx", "status":"Running" } }
  { "type": "tdx_status", "data": { "enabled": true } }
  { "type": "qemu_process", "data": { "pid": 1433262, "rss_mb": 165 } }
```

---

## 四、采集能力清单

### 4.1 已完成 ✅

- [x] 通过命令行观测 TDX / SGX / K8s / Pod / QEMU / Trustee

### 4.2 待实现 🔲

#### 静态采集（启动时一次 + 定时刷新）

- [ ] TDX 硬件状态：`/sys/module/kvm_intel/parameters/tdx` + dmesg
- [ ] SGX 设备：`/dev/sgx_*`
- [ ] K8s 节点状态：`kubectl get nodes` → client-go
- [ ] 机密 Pod 列表：`kubectl get pods -o wide` → client-go
- [ ] Pod 详情：RuntimeClass, Guest 内核, QEMU PID
- [ ] RuntimeClass 列表
- [ ] Trustee 组件状态

#### 持续状态监听

- [ ] WebSocket 实时推送 Pod 状态变化
- [ ] QEMU 进程存活检测
- [ ] TDX 状态变化告警

#### 操作接口

- [ ] 创建机密 Pod：`POST /api/pods`
- [ ] 删除机密 Pod：`DELETE /api/pods/:name`
- [ ] Pod 日志流：`GET /api/pods/:name/logs`

---

## 五、技术栈

| 层 | 选型 | 用途 |
|----|------|------|
| HTTP 框架 | **Gin** | REST API 路由 + 中间件 |
| K8s SDK | **client-go** | Pod/Node/RuntimeClass 查询 |
| WebSocket | **gorilla/websocket** | 实时状态推送 |
| 系统命令 | **os/exec** | dmesg/ps/ls 等 |
| 构建 | **go build** | 单二进制 + embed 前端 |

---

## 六、实现计划

| 阶段 | 内容 | 预计 |
|------|------|------|
| **Phase 1** | 项目骨架：Gin 路由 + 模型定义 + 静态数据返回 | 当前 |
| **Phase 2** | K8s 采集：client-go 接入 + 真实数据返回 | 下一步 |
| **Phase 3** | 系统采集：TDX/SGX/QEMU 实时读取 | |
| **Phase 4** | WebSocket：实时推送 Pod 状态 + QEMU 进程 | |
| **Phase 5** | 操作接口：创建/删除 Pod + 日志 | |
| **Phase 6** | 构建部署：embed 前端 → 单二进制 → Docker | |

---

## 七、运行

```bash
# 开发模式
go run main.go

# 构建单二进制（含前端）
go build -o coco-serve

# 访问
http://localhost:8080/api/overview
```

---

## 八、依赖安装

```bash
go get github.com/gin-gonic/gin
go get k8s.io/client-go@v1.36.1
go get github.com/gorilla/websocket
```
