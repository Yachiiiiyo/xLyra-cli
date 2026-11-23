# xlyra (`xLyra-cli`)

面向 xLyra 内置 Pi Agent 的专用控制面命令行工具（二进制命令名：`xlyra`），仅通过 **Control-Plane Unix Domain Socket（`XLYRA_CTL_SOCK`）** 与 `xlyra-server` 通信，支持配置自省、运行态只读观测、结构化 Diff 预演（`--dry-run`）与服务端审批闭环。

## 特性与安全边界

- **零外部依赖、单文件静态编译**：仅依赖 Go 标准库，`CGO_ENABLED=0` 交叉编译生成轻量二进制。
- **仅 UDS 控制面连接**：强制校验 `XLYRA_CTL_SOCK`（以及本地模式 `XLYRA_CTL_NONCE`），不提供 `--url` / `--token` 等外部 HTTP 旁路参数。
- **外部明文密钥隔离**：一期不在 CLI 层开放外部明文密钥写入（如站点上游 Key 创建、代理 URL 内嵌密码、站点敏感鉴权 Header、S3 密钥），相关操作在客户端与服务端同时按 Tier 0 拦截。
- **工作区文件越界防御**：`--file <path>` 强制执行 `cwd` `realpath` 校验与 1 MiB 大小限制，防止无沙箱模式下读取工作目录外文件。
- **确定性退出码（0–6）与统一 JSON 信封**：天然适合 LLM Agent 与自动化管道解析。

详细通信协议与命令映射见 [docs/protocol.md](docs/protocol.md)。

## 本地构建与测试

```bash
# 运行静态检查与竞态单测
make vet
make test

# 构建本地二进制到 ./bin/xlyra
make build

# 查看本地二进制版本与协议版本（无需 XLYRA_CTL_SOCK）
./bin/xlyra version
```

## Release 打包与主程序集成

执行以下命令打包 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 四个平台的压缩包与校验清单：

```bash
VERSION=v0.1.1 make release
```

输出目录 `dist/` 包含：
- `xlyra_<version>_<os>_<arch>.tar.gz`：内含根目录可执行文件 `xlyra`
- `checksums.txt`：所有压缩包的 SHA-256 校验值
- `manifest.json`：包含 `version`、`prerelease`、`protocol_version`、`commit`、`build_time` 与各平台 `filename` / `sha256` / `size`，供主程序 `xlyra-server` 下载与自动更新器校验使用

### 标准发版流程

```bash
# 1. 在干净的 main 分支运行校验并创建 SemVer annotated tag
./scripts/tag-release.sh v0.1.1

# 2. 推送标签触发 .github/workflows/release.yml 自动打包并发布到 GitHub Release
git push origin v0.1.1
```

> 含 `-` 的预发布标签（如 `v0.1.0-rc.1`）会自动标记为 GitHub Pre-release，并在 `manifest.json` 中置 `"prerelease": true`，不会覆盖 `/releases/latest`。也可通过 GitHub Actions `workflow_dispatch` 手动触发发版。
