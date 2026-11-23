# Changelog

## [v0.1.0] - 2026-11-23

### Added
- **Control-Plane v1 协议与统一信封**（`X-Xlyra-Ctl-Protocol: 1`）：仅通过 `XLYRA_CTL_SOCK`（及可选 `XLYRA_CTL_NONCE`）经 `AF_UNIX` HTTP/1.1 连接 `xlyra-server`，确定性退出码 `0`–`6`。
- **全量 14 大域命令树**：覆盖 `status`、`schema`、`config`、`proxy`、`site`、`site-group`、`site-key`、`site-account`、`oauth`、`model`、`site-model`、`pricing`、`api-key`、`route`、`cooldown`、`plugin`、`plugin-market`、`agents-md`、`skill`、`backup`、`cloud`、`dashboard`、`request`、`usage`。
- **安全边界与防泄露护栏**：
  - 本地 `version` / `--version` 子命令支持无 `XLYRA_CTL_SOCK` 查询二进制与协议版本。
  - `--file` 强制 `cwd` `realpath` 校验与 1 MiB 大小限制。
  - 客户端前置拦截 `site-key create`、`backup restore/import`、`agents-md`/`skill` 写入、`plugin trusted-keys` 写操作、代理 URL 内嵌密码及敏感鉴权 `request_headers`（返回 `policy_denied`，退出码 `4`）。
- **多格式渲染与跨平台 Release 打包**：支持 `json` / `yaml` / `table` 输出与 `--fields` 投影；提供 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 打包脚本及 `checksums.txt`、`manifest.json` 自动化 Release 流水线。
