# CoCo Panel — 机密容器可视化面板

> **版本**: 1.1 | **日期**: 2026-07-06 | **语言**: Go + Svelte 5
> **一句话**: 单二进制 K8s 可视化面板，展示 Intel TDX 机密容器内存加密与隔离证明

---

## 🎯 功能一览

### 📊 系统总览 (`/`)
- K8s 集群状态（版本、节点、Pod 数量）
- Intel TDX / SGX 硬件状态
- Pod 按运行时分类（TDX vs 标准），带比例进度条
- 命名空间分布
- 所有卡片可点击跳转到对应详情页

### 🖥️ 机密容器 (`/pods`)
- Pod 列表（支持按运行时和命名空间筛选）
- **实时 WebSocket**：Pod 创建/删除/状态变更即时推送
- 创建 Pod（带表单：运行时、镜像、资源、端口、标签）
- 删除 Pod（带淡出动画）
- 📋 查看 Pod YAML 配置
- 🔍 Pod 详情展开：内核版本、内存、运行时间
- 📜 **K8s Events 生命周期时间线**（Scheduled → Pulling → Started）
- 🔬 进程隔离验证：
  - 宿主机可见进程 vs 容器内实际进程（kubectl exec 获取）
  - 点击 PID 验证内存可读性（ptrace + /proc/pid/mem）
  - TDX 容器：宿主机 PID 25 是[内核线程]，完全不是容器内的 nginx
- 🔐 **内存加密验证**（新增）：
  - 全自动模式：写入测试数据 → 宿主机读 QEMU 内存 → 密文；容器内读 → 明文
  - 半自动模式：手动指定 PID，对比宿主机与容器内内存视图
  - Hex dump 对比 + Shannon 熵值分析
  - 密文区域高熵值（~7.9），明文区域低熵值（~4.5）

### 🔄 运行时 (`/runtimes`)
- 10 个 RuntimeClass 列表
- 每个显示：Pod 数量、描述、可用状态
- 点开展示：Handler、CPU/内存开销、节点选择器

### 🔐 证明链 (`/trustee`)
- AS（Attestation Service）、KBS（Key Broker Service）、RVPS（Reference Value Provider）
- 点击展开：功能详情 + 证明流程图

---

## 🏗️ 项目结构

```
/root/yuxi-workspace/project/
├── coco-serve/                  # Go 后端
│   ├── main.go                  # 入口，embed 前端，启动服务
│   ├── dist/                    # 嵌入的前端静态文件（构建产物）
│   └── internal/
│       ├── handler/             # HTTP 处理器
│       │   ├── router.go        # 路由注册
│       │   ├── routes.go        # GetOverview, GetPods, GetRuntimes, GetPodYaml, GetPodEvents...
│       │   ├── ops.go           # CreatePod, DeletePod, GetPodLogs + WebSocket 广播
│       │   ├── podinfo.go       # GetPodSysInfo, GetProcMem, 进程隔离验证
│       │   ├── memory_proof.go  # 🔐 内存加密验证 (全自动/半自动)
│       │   ├── handler.go       # H 结构体（K8s client 注入）
│       │   └── watcher.go       # WebSocket 状态广播（pod_phase 追踪）
│       ├── k8sclient/           # K8s client-go 封装
│       │   └── client.go        # GetPods, GetRuntimeClasses, CreatePod, GetPodYAML...
│       ├── model/               # 数据模型
│       │   └── types.go         # 12 个类型（OverviewResponse, PodInfo, RuntimeInfo...）
│       ├── collector/           # 系统信息采集
│       │   └── collector.go     # TDX/SGX 状态, Trustee 端点
│       └── ws/                  # WebSocket
│           ├── hub.go           # Hub 广播
│           └── client.go        # Client 连接
│
└── coco/                        # Svelte 5 前端
    ├── svelte.config.js         # adapter-static → SPA
    ├── vite.config.ts           # Vite 配置
    ├── src/
    │   ├── routes/
    │   │   ├── +layout.svelte   # 侧边栏导航
    │   │   ├── +page.svelte     # 📊 系统总览
    │   │   ├── pods/
    │   │   │   └── +page.svelte # 🖥️ 机密容器（最复杂页面）
    │   │   ├── runtimes/
    │   │   │   └── +page.svelte # 🔄 运行时
    │   │   └── trustee/
    │   │       └── +page.svelte # 🔐 证明链
    │   └── lib/
    │       ├── types.ts         # TypeScript 类型定义
    │       └── components/
    │           └── HexDump.svelte  # 🆕 Hex dump 对比组件
    └── dist/                    # 构建产物（嵌入 Go 二进制）
```

---

## 🔌 API 端点（15 个）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/api/overview` | K8s + TDX + SGX 总览 |
| GET | `/api/pods` | Pod 列表（支持 `?runtime=` `?ns=` 筛选） |
| POST | `/api/pods/create` | 创建 Pod（支持完整参数） |
| DELETE | `/api/pods/:ns/:name` | 删除 Pod |
| GET | `/api/pods/:ns/:name/logs` | Pod 日志 |
| GET | `/api/pods/info/:ns/:name` | Pod 系统信息 + 进程对比 + 隔离证据 |
| GET | `/api/pods/:ns/:name/yaml` | Pod YAML 配置 |
| GET | `/api/pods/:ns/:name/events` | 🆕 K8s Events 生命周期时间线 |
| GET | `/api/proc/:pid/mem` | 进程内存读取验证（ptrace） |
| GET | `/api/runtimes` | RuntimeClass 列表 |
| GET | `/api/runtimes/:name` | RuntimeClass 详情 |
| GET | `/api/trustee` | Trustee 证明链状态 |
| GET | `/api/demo/memory-encrypt` | 🆕 内存加密全自动验证 |
| GET | `/api/demo/memory-compare` | 🆕 内存加密半自动验证 |
| GET | `/health` | 健康检查 |

### WebSocket 事件

| 事件类型 | 数据 | 触发条件 |
|----------|------|----------|
| `pod_created` | name, namespace, runtime, image | Pod 创建成功 |
| `pod_deleted` | name, namespace | Pod 删除成功 |
| `pod_phase` | name, namespace, phase | Pod 状态变更 |
| `pod_count` | count, message | Pod 总数变更 |
| `tdx_status` | TDXStatus | TDX 状态变更 |

---

## 🚀 运维命令

```bash
# 启动（后台守护）
nohup /root/yuxi-workspace/project/coco-serve/coco-serve > /var/log/coco-serve.log 2>&1 &

# 停止
pkill coco-serve

# 查看状态
curl -s http://localhost:8080/health
ss -tlnp | grep 8080

# 重新构建（前端改动后）
cd /root/yuxi-workspace/project/coco && npx vite build
cp -r dist /root/yuxi-workspace/project/coco-serve/
cd /root/yuxi-workspace/project/coco-serve && go build -o coco-serve .

# 查看日志
tail -f /var/log/coco-serve.log
```

---

## 🌐 访问地址

| 网络 | URL |
|---|---|
| Tailscale（推荐） | `http://100.122.57.70:8080` |
| 内网 LAN | `http://192.168.102.4:8080` |
| 本地 | `http://localhost:8080` |

---

## 🛠️ 技术栈

| 层 | 技术 | 版本 |
|----|------|------|
| 后端语言 | Go | 1.26.4 |
| Web 框架 | Gin | latest |
| K8s 客户端 | client-go | latest |
| YAML 处理 | sigs.k8s.io/yaml | 1.6.0 |
| 前端框架 | Svelte | 5 |
| 构建工具 | Vite + SvelteKit | latest |
| 部署方式 | Go embed 静态文件 → 单二进制 | — |
| 容器运行时 | kata-qemu-tdx (TDX) + runc | 3.25.0 |
| K8s | kubeadm 单节点 | 1.36.1 |
| TEE 硬件 | Intel TDX + SGX | — |

---

## 🧪 隔离证明演示流程

1. 打开 `http://IP:8080/pods`
2. 展开 TDX Pod（如 `tdx-nginx`）
3. 看到：宿主机只有 2 个 QEMU 进程，容器内 nginx(PID=25) 标注"宿主机不可见"
4. 点击 nginx 的「📖 验证」：显示"宿主机 PID 25 是[内核线程]，完全不是容器内的 nginx"
5. 展开普通 Pod（如 `mysql-demo`）对比：宿主机可见所有 containerd-shim 进程
6. 在总览页点击 TDX 硬件卡片 → 跳转到证明链页面查看 Trustee 组件详情

### 🆕 内存加密验证演示

1. 展开任意 TDX Pod（如 `tdx-nginx`）
2. 滚动到「🔐 内存加密验证」面板
3. 点击「🔄 一键验证 (全自动)」：
   - 后端自动在容器内写入测试数据 → 从宿主机读 QEMU 内存（密文） → 从容器内读（明文）
   - 左侧红色边框：宿主机视角 hex dump，高熵值（~7.9），无法找到明文
   - 右侧绿色边框：容器内视角，低熵值（~4.5），明文完整可见
4. 或使用「🔍 手动对比 (半自动)」：指定 QEMU PID，读取宿主机侧内存内容

### 🆕 创建时间线演示

1. 创建 Pod 后，实时 WebSocket 推送阶段变化（Pending → ContainerCreating → Running）
2. 展开 Pod 详情，查看「📜 生命周期」时间线
3. 每个 K8s Event 显示时间戳、原因、详细消息

---

## 📚 关联文档

| 文档 | 路径 |
|------|------|
| 环境快照 | `/root/allconfig/ENVIRONMENT.md` |
| 变更记录 | `/root/allconfig/CHANGELOG.md` |
| 项目总览 | `/root/allconfig/COCO-PANEL.md` |
| 设备总 README | `/root/README.md` |
| CoCo 能力说明 | `/root/COCO-CAPABILITIES.md` |
