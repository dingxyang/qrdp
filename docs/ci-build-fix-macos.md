# macOS CI 构建修复过程记录

> 文件：`.github/workflows/release-macos.yaml`
> 修复周期：2026-04-03 ～ 2026-04-09，共 10 次提交

---

## 背景

初始工作流（`8546e9e`）使用 GitHub Actions `matrix` 策略，在同一个 job 中同时构建 `darwin/amd64` 和 `darwin/arm64`，依赖 `brew install freerdp`。
在 GitHub 托管的 macOS runner（ARM64 架构）上，Homebrew 只能安装 arm64 的 FreeRDP，无法直接用于 amd64 交叉编译，导致构建失败。

原始报错（用户看到的最终错误）：

```
missing go.sum entry for module providing package github.com/tea4go/gh/log4go
missing go.sum entry for module providing package github.com/wailsapp/wails/v2
...
pattern frontend/dist: no matching files found
exit status 1
```

---

## 修复步骤

### 1. `81cd9e8` (2026-04-03) — 修复 Linux/Windows 工作流依赖

**问题**：Linux 构建脚本引用了不存在的 gstreamer 包；Windows 工作流硬编码了 msys2 路径且 shell 配置有误。

**修改**：
- `release-windows.yaml` / `lib_build_linux.sh` / `wails_build_linux.sh`：删除 `libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev`
- Windows 工作流：将 msys2 路径改为动态引用 `${{ steps.msys2.outputs.msys2-location }}`，shell 从 `msys2 {0}` 改为 `cmd`

---

### 2. `544dbe5` (2026-04-03) — 拆分 arm64 / amd64 为两个独立 job

**问题**：macOS runner 是 ARM64，`brew install freerdp` 只有 arm64 库，matrix 无法同时覆盖两个架构。

**修改**：
- 删除 matrix 策略，拆分为 `release-arm64`（直接用 Homebrew FreeRDP）和 `release-amd64`（从源码交叉编译）
- 新增顶层 env 变量 `FREERDP_TAG: "3.12.0"`
- `release-amd64` job：从源码克隆并 cmake 构建 FreeRDP，目标架构 `x86_64`

---

### 3. `fb5c522` (2026-04-03) — amd64：从源码编译 OpenSSL x86_64

**问题**：`brew install openssl` 只提供 arm64 静态库；FreeRDP 链接时架构不匹配。

**修改**：
- 删除 `brew install openssl`，改为从 openssl 官方 tarball 编译静态库（`darwin64-x86_64-cc`），安装到 `$GITHUB_WORKSPACE/openssl-x86_64`
- cmake 新增 `-DOPENSSL_ROOT_DIR="${OPENSSL_INSTALL}"`

---

### 4. `dd87998` (2026-04-03) — amd64：添加 CGO 架构标志

**问题**：CGO 使用 host（arm64）工具链编译，输出的目标文件架构不对。

**修改**：
```yaml
export CC="clang -arch x86_64"
export CGO_CFLAGS="-arch x86_64"
export CGO_LDFLAGS="-arch x86_64 -L${FREERDP_INSTALL}/lib -L${OPENSSL_INSTALL}/lib"
```

---

### 5. `004e7f4` (2026-04-03) — amd64：改用 clang target triple

**问题**：`-arch x86_64` 仍会选择 arm64 版系统库；需要明确指定 target。

**修改**：
```yaml
export CC="clang -target x86_64-apple-macos11"
export CGO_LDFLAGS="-target x86_64-apple-macos11 -L${FREERDP_INSTALL}/lib -L${OPENSSL_INSTALL}/lib"
```

---

### 6. `a9aaab3` (2026-04-07) — amd64：改用 `-mmacosx-version-min`

**问题**：`-target x86_64-apple-macos11` 与 wails 内部 CGO 调用的参数冲突，导致链接错误。

**修改**：
```yaml
export CGO_CFLAGS="-mmacosx-version-min=11.0"
export CGO_LDFLAGS="-mmacosx-version-min=11.0 -L${FREERDP_INSTALL}/lib -L${OPENSSL_INSTALL}/lib"
# 移除 CC 覆盖
```

---

### 7. `d79ae46` (2026-04-07) — amd64：预生成 wails bindings

**问题**：`wails build` 在 amd64 交叉编译时尝试重新生成 Go bindings，在 arm64 runner 上生成的绑定与 amd64 目标不一致。

**修改**：
- 新增 `wails generate module` 步骤（用 host arm64 先生成绑定）
- `wails build` 加 `-skipbindings` 标志跳过重复生成
- `brew install ... freerdp` 补充安装（头文件查找需要）

---

### 8. `6001de3` (2026-04-07) — amd64：尝试 Rosetta 2 方案（后弃用）

**问题**：CGO 跨架构编译细节仍有问题，尝试用 Rosetta 2 将整个构建过程运行在 x86_64 模式下。

**修改**：
```bash
softwareupdate --install-rosetta --agree-to-license || true
arch -x86_64 cmake ...
arch -x86_64 go install ...
arch -x86_64 npm install
arch -x86_64 wails generate module
arch -x86_64 GOARCH=amd64 CGO_ENABLED=1 wails build ...
```

---

### 9. `97e6c8b` (2026-04-09) — amd64：放弃 Rosetta，改用 CMake 架构标志

**问题**：GitHub Actions macOS runner 不保证支持 Rosetta 2；`arch -x86_64` 在部分 runner 上报错；`git clone --depth 1 --branch <tag>` 对部分 tag 名称不可用。

**修改**：
- 移除所有 `arch -x86_64` 前缀和 Rosetta 安装步骤
- FreeRDP 克隆方式改为 `git init + fetch + checkout`（支持任意 tag）：
  ```bash
  git init "${FREERDP_SRC}"
  git -C "${FREERDP_SRC}" remote add origin https://github.com/FreeRDP/FreeRDP.git
  git -C "${FREERDP_SRC}" fetch --depth 1 origin "refs/tags/${FREERDP_TAG}:refs/tags/${FREERDP_TAG}"
  git -C "${FREERDP_SRC}" checkout -q "tags/${FREERDP_TAG}"
  ```
- cmake 改用 `-DCMAKE_OSX_ARCHITECTURES=x86_64` 控制输出架构（不依赖 Rosetta）
- CGO flags 恢复显式 `-arch x86_64`：
  ```yaml
  export CGO_CFLAGS="-arch x86_64 -mmacosx-version-min=11.0"
  export CGO_LDFLAGS="-arch x86_64 -mmacosx-version-min=11.0 -L${FREERDP_INSTALL}/lib -L${OPENSSL_INSTALL}/lib"
  ```

---

### 10. `931dfeb` (2026-04-09) — amd64：修复 go.sum 缺失 和 frontend/dist 不存在

**这是最终修复，对应用户看到的报错。**

**问题 1：`missing go.sum entry`**

`wails build` 依赖完整的 `go.sum`，但 CI 环境中没有运行 `go mod download`，导致：
```
missing go.sum entry for module providing package github.com/tea4go/gh/log4go
missing go.sum entry for module providing package github.com/wailsapp/wails/v2
...
```

**修复**：在 wails build 之前新增步骤：
```yaml
- name: Download Go modules
  shell: bash
  run: go mod download
```

**问题 2：`pattern frontend/dist: no matching files found`**

前端构建步骤漏掉了 `npm run build`，导致 `frontend/dist` 目录从未生成，wails 找不到前端产物。

**修复**：在 `npm install` 之后补充：
```yaml
npm run build
```

---

## 最终工作流结构

```
release-macos.yaml
├── release-arm64
│   ├── brew install freerdp          # Homebrew 直接提供 arm64 库
│   ├── go install wails
│   ├── npm install                   # 仅安装依赖（无需 build，wails build 会处理）
│   └── wails build -platform darwin/arm64
│
└── release-amd64
    ├── brew install cmake pkg-config
    ├── 从源码编译 OpenSSL (x86_64 静态库)
    ├── 从源码编译 FreeRDP (CMAKE_OSX_ARCHITECTURES=x86_64)
    ├── go install wails
    ├── npm install && npm run build   # ★ 必须 build，生成 frontend/dist
    ├── go mod download                # ★ 必须下载，填充 go.sum
    ├── wails generate module          # 在 arm64 host 上生成 Go bindings
    └── GOARCH=amd64 CGO_ENABLED=1 wails build -skipbindings -platform darwin/amd64
        # CGO_CFLAGS/LDFLAGS: -arch x86_64 -mmacosx-version-min=11.0
```

---

## 关键教训

| 问题 | 根因 | 解法 |
|---|---|---|
| `go.sum` 缺失 | CI 未运行 `go mod download` | 构建前显式执行 `go mod download` |
| `frontend/dist` 不存在 | 前端 step 只 `npm install` 没有 `npm run build` | 补充 `npm run build` |
| arm64 runner 无法直接提供 amd64 库 | Homebrew 库与 runner 架构绑定 | amd64 所有依赖从源码交叉编译 |
| Rosetta 2 方案不稳定 | runner 不保证 Rosetta 可用 | 改用 CMake `-DCMAKE_OSX_ARCHITECTURES=x86_64` |
| `git clone --depth 1 --branch <tag>` 失败 | GitHub 浅克隆不支持直接按 tag 浅拉 | 改用 `git init + fetch refs/tags/...` |
