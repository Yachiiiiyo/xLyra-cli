#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "用法: $0 <vMAJOR.MINOR.PATCH[-prerelease]>" >&2
  exit 2
fi

VERSION="$1"
if ! echo "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  echo "error: 版本号格式非法 '$VERSION'，请使用 vMAJOR.MINOR.PATCH[-prerelease]（例如 v0.1.0 或 v0.1.0-rc.1）" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$BRANCH" != "main" ]; then
  echo "error: 当前位于分支 '$BRANCH'，请在 'main' 分支上打发版标签" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "error: 工作区存在未提交改动，请先提交或清理后再打标签" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
  echo "error: 本地已存在标签 '$VERSION'" >&2
  exit 1
fi

echo "Running go vet and race tests before tagging $VERSION..."
go vet ./...
go test -race ./...

git tag -a "$VERSION" -m "Release $VERSION"
echo "Created annotated tag $VERSION."
echo "Next step: git push origin $VERSION"
