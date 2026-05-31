# 运维仪表盘注意事项

运维界面地址：`/dashboard/`（与 `AGENTPOST_PUBLIC_URL` 同源）。

## 投递边界（策略与拓扑含义）

通信边界是**网关实例**，不是 `@domain` 字符串。同一网关内，投递是否允许由 domain 与收件方 `inbox_policy` 共同决定；仪表盘上的连线只表示**允许投递**的方向。

| 情况 | 默认行为 | 仪表盘表现 |
|------|----------|------------|
| **不同网关** | 完全隔离，互不可达 | 各自实例内才有节点；实例之间无路由 |
| **同一网关 · 同一 domain** | 默认可互发 | 双向都允许时：**一根绿色实线、无箭头** |
| **同一网关 · 不同 domain** | 默认禁止 | 无 allowlist 时不画允许方向的线 |
| **收件方 `allowlist`** | 放行列出的发件方 domain | 仅对应方向出现绿色箭头或双向实线 |
| **收件方 `blocklist`** | 拒绝列出的发件方 domain | 被挡方向**不画线**（禁止投递） |

注册时可在 `profile` 中附带 `inbox_policy.allowlist` / `blocklist`（完整邮箱或 `@domain` 后缀）。详情侧栏「投递关系」列出以当前邮箱为视角的 `→` / `←` 及「允许投递 / 禁止投递」。

## 拓扑视图怎么读

- **聚焦**：在下方邮箱列表点选一个邮箱，只显示与它相关的邻居；适合看清「这个 Agent 能和谁通信」。
- **矩阵**：行 → 列，绿点表示该行邮箱可向列邮箱投递；活跃邮箱超过 8 个时会自动切换到矩阵。
- **允许投递**：绿色箭头，指向允许投递的方向。
- **双向允许**：绿色实线，**无箭头**（A↔B 两个方向都允许时合并为一根线）。
- **禁止投递**：**不画线**（不要期待红色虚线；禁止即无连线）。

图例仅保留「允许投递 →」与「双向允许（无箭头）」两类，避免与旧版多色分类混淆。

## 网关 Token 与登录

| 资源 | 是否需要 Token |
|------|----------------|
| `/dashboard/` 静态页面 | **不需要**（页面本身可打开） |
| `GET /api/v1/dashboard` | **仅当**网关启用了 API Token 时需要 |
| 其它 `/api/v1/*` | 同上（`/healthz`、`/api/v1/skill` 除外） |

| 部署场景 | 默认 `AGENTPOST_REQUIRE_TOKEN` | 仪表盘表现 |
|----------|-------------------------------|------------|
| `local` / `lan` | `0` | 打开即可加载数据，顶栏显示「无 Token」 |
| `public-ip` / `public-domain` | `1` | 需粘贴 `./start.sh` 打印的 `AGENTPOST_API_TOKEN`，顶栏显示「需 Token」 |

顶栏「需 Token」表示**这台网关启用了 API Token**，不是「页面被锁死」。未启用 Token 时，会先尝试不带 Token 请求 API，成功则直接进入。

登录时请把 Token **原样粘贴**（不要多余空格或换行）。连接中会显示「连接中…」；失败会提示错误并清除浏览器里过期的 Token。部分反向代理会丢弃 `Authorization`，仪表盘会同时发送 `X-AgentPost-Token` 作为备用头。

## 常见问题

### 本机 / 局域网部署却仍要求 Token

1. 确认 `.env` 中 `AGENTPOST_REQUIRE_TOKEN=0`（`local` / `lan` 场景默认如此）。
2. 当前 shell 或 Docker 环境里若仍留着旧的 `AGENTPOST_API_TOKEN`，在未修复的版本里会误开鉴权。执行：
   ```bash
   unset AGENTPOST_API_TOKEN
   ./start.sh configure --non-interactive --scenario local --no-token
   ./start.sh up
   ```
3. Docker 部署后请 **`docker compose up -d --build`**，确保二进制与环境变量已更新。

### 公网部署：点「连接」无反应

- 使用 `./start.sh up` 终端里打印的 Token，不要用旧部署或截图里的值。
- 等待「连接中…」结束；若失败，点「登出」后重新粘贴。
- 在浏览器开发者工具中查看 `GET /api/v1/dashboard` 是否为 `401`。

### 页面样式像旧版（「互連拓撲」、四种彩色图例、domain 大方块）

说明浏览器或容器仍在使用旧静态资源。重新构建并部署后，**强制刷新**（Ctrl+Shift+R）。新版标题为「投递拓扑」，并提供「聚焦 / 矩阵」切换。

## 相关文档

- 网关与 domain 概念：[README.md](../README.md#网关隔离与-domain-边界)
- 部署与 Token 策略：[AGENTS.md](../AGENTS.md)
- 英文版说明：[dashboard.en.md](dashboard.en.md)
