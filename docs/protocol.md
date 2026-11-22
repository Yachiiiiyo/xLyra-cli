# `xlyra-cli` Control-Plane v1 协议规范

本文档定义 `xlyra-cli` 与 `xlyra-server` (`agentctl.Server`) 之间的 Unix Domain Socket 通信契约以及 GitHub Release 产物清单格式（供主程序实现自动下载、校验与版本更新）。

---

## 1. 传输通道与身份绑定

- **传输方式**：`HTTP/1.1 + JSON` over `AF_UNIX` Stream Socket（绝不回退 TCP/HTTP）。
- **环境变量**：
  - `XLYRA_CTL_SOCK`（必填）：Control-Plane Unix Socket 绝对路径，如 `<SessionDir>/ctl/ctl.sock`（权限 `0600`，`sun_path` ≤ 104 字节）。
  - `XLYRA_CTL_NONCE`（本地 `localBash` 模式必填）：一次性内存中绑定的 run nonce，通过 HTTP Header `X-Xlyra-Ctl-Nonce` 传递给服务端。
- **服务端核验**：
  - Linux 沙箱隔离模式下，服务端通过 `SO_PEERCRED` 强制校验对端 `uid == expectedExecUID`；
  - `localBash` 模式下，服务端强制校验 `X-Xlyra-Ctl-Nonce` 与当前活动 run 绑定一致；
  - 所有请求携带 `X-Xlyra-Ctl-Protocol: 1`。
- **超时与取消**：
  - `xlyra-cli` 不设客户端请求固定超时，允许服务端阻塞等待人工审批卡片确认（沙箱模式下由 `exec.sock` 下发 `{"t":"timer","action":"pause"|"resume"}` 暂停/恢复命令超时）；
  - 当进程收到 `SIGINT`/`SIGTERM` 时取消 HTTP 请求上下文并立即断开 UDS 连接，服务端感知 `r.Context().Done()` 后清理挂起审批。

---

## 2. 统一调用端点

- **Endpoint**：`POST http://ctl.local/v1/call`
- **Content-Type**：`application/json`

### 请求体 `CallRequest`

```json
{
  "protocol": 1,
  "resource": "site",
  "action": "update",
  "target": "openrouter",
  "sub_target": "",
  "params": {
    "enabled": true
  },
  "payload": {
    "routing_priority": 10
  },
  "options": {
    "dry_run": true,
    "fields": ["id", "slug", "routing_priority"],
    "if_match": "sha256:..."
  }
}
```

### 统一响应信封 `Envelope`

成功响应（HTTP `200`）：

```json
{
  "ok": true,
  "data": { ... },
  "diff": [
    { "path": "routing_priority", "before": 50, "after": 10 }
  ],
  "meta": {
    "tier": 2,
    "dry_run": true,
    "revision": "sha256:9f2a...",
    "truncated": false,
    "next_page": null,
    "applied_effects": ["gateway.models_cache_invalidated"]
  }
}
```

失败响应（HTTP `4xx` / `5xx`）：

```json
{
  "ok": false,
  "error": {
    "code": "policy_denied",
    "message": "agent.capabilities.* 属于 Agent 自身配置（Tier 0），禁止通过 xlyra-cli 修改",
    "field": "agent.capabilities.sandbox_enabled",
    "hint": "请让管理员在 Web 控制台的 Agent 设置中手动调整"
  }
}
```

---

## 3. 确定性退出码与错误码映射

| 退出码 | 含义 | 典型 `error.code` / HTTP 状态码 |
| --- | --- | --- |
| `0` | 成功 | `ok: true` & HTTP `2xx` |
| `1` | 内部错误 / UDS 通信失败 | `internal_error` / HTTP `5xx` / 缺失或不可连 `XLYRA_CTL_SOCK` |
| `2` | 参数或业务校验失败 | `invalid_argument`, `invalid_body`, `validation_error` / HTTP `400`, `422` |
| `3` | 目标资源不存在 | `not_found` / HTTP `404` |
| `4` | 策略拒绝 / 超限额 | `policy_denied`, `forbidden`, `rate_limited`, `quota_exceeded` / HTTP `403`, `429` |
| `5` | 审批拒绝或审批超时 | `approval_denied`, `approval_timeout`, `user_denied`, `classifier_denied` |
| `6` | 资源冲突 / 乐观锁不匹配 / 标识符不唯一 | `conflict`, `ambiguous`, `ambiguous_identifier`, `revision_mismatch`, `precondition_failed` / HTTP `409`, `412` |

> **注意**：`ExitCodeForResponse` 优先匹配 `error.code`（例如服务端以 `403` 返回 `approval_denied` / `approval_timeout` 时，CLI 仍确定性返回退出码 `5`）。

---

## 4. Resource / Action 映射表

| `resource` | `action` 列表 |
| --- | --- |
| `status` | `get` |
| `schema` | `list`, `get` |
| `config` | `list`, `describe`, `get`, `set`, `patch`, `reset` |
| `proxy` | `list`, `get`, `create`, `update`, `delete`, `test` |
| `site` | `list`, `get`, `create`, `update`, `enable`, `disable`, `set-gateway`, `set-plugin-order`, `delete`, `refresh`, `health`, `sync-models` |
| `site-group` | `list`, `get`, `create`, `update`, `delete` |
| `site-key` | `list`, `update`, `set-model`, `delete`（`create` 在客户端与服务端均按 Tier 0 拒绝） |
| `site-account` | `list`, `update`, `set-model`, `refresh` |
| `oauth` | `list`, `get`, `set-model`, `refresh` |
| `model` | `list`, `get`, `create`, `update`, `delete`, `alias-add`, `alias-delete`, `pricing-set`, `pricing-reset` |
| `site-model` | `list`, `create`, `update`, `batch-status`, `bind-canonical`, `test`, `delete` |
| `pricing` | `list`, `set`, `bulk-set`, `reset` |
| `api-key` | `list`, `get`, `create`, `update`, `reorder`, `set-sites`, `set-site-groups`, `set-site-models`, `set-mappings`, `quota-increase`, `quota-reset`, `check-model`, `rotate`, `delete` |
| `route` | `overview`, `candidates`, `select`, `failover`, `traces` |
| `cooldown` | `list`, `create`, `clear` |
| `plugin` | `list`, `get`, `settings-get`, `settings-set`, `site-settings-set`, `schedule-get`, `schedule-set`, `subscription-set`, `automation-list`, `automation-create`, `automation-update`, `automation-delete`, `bind-site`, `unbind-site`, `bind-protocol`, `enable`, `disable`, `delete`, `trusted-keys-list` |
| `plugin-market` | `status`, `list`, `get`, `sync`, `install` |
| `agents-md` | `get` |
| `skill` | `list`, `get` |
| `backup` | `sections`, `auto-test`, `auto-files`, `auto-run` |
| `cloud` | `status`, `report-preview` |
| `dashboard` | `overview`, `insights` |
| `request` | `list`, `get`, `summary`, `channel-split` |
| `usage` | `summary`, `api-keys` |

---

## 5. Release 资产与 `manifest.json` 规范（主程序下载更新对接）

每次 Release 发布以下资产：
- `xlyra-cli_<version>_<os>_<arch>.tar.gz`（包含根目录可执行文件 `xlyra-cli`，覆盖 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`）
- `checksums.txt`（标准 SHA-256 校验文件）
- `manifest.json`（机器可读元信息清单）：

```json
{
  "version": "v0.1.0",
  "protocol_version": 1,
  "commit": "abc1234",
  "build_time": "2026-11-23T02:55:00Z",
  "artifacts": {
    "linux/amd64": {
      "os": "linux",
      "arch": "amd64",
      "filename": "xlyra-cli_v0.1.0_linux_amd64.tar.gz",
      "sha256": "...",
      "size": 3145728
    }
  }
}
```

主程序下载器可通过 `xlyra-cli version`（无需 `XLYRA_CTL_SOCK`）获取本地已安装二进制的 `version` 与 `protocol_version`，再对比 Release 中的 `manifest.json` 完成按需更新与 SHA-256 校验。
