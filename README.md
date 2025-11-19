## Repeater Xray PoC 去重工具

一个 Go CLI，用来扫描 Xray PoC 文件目录，根据 PoC 中任意位置的 `path` 字段识别重复项，并可选择自动删除较旧的副本。（AI写的）

### 功能亮点
- 递归扫描 `.yml`、`.yaml`、`.json` 格式的 PoC 文件。
- 解析 `name` 并遍历整个文件收集所有 `path` 字段。
- 将相同 `path` 的文件归为同一组，集中展示。
- 输出每个重复组的文件路径与修改时间。
- `-delete` 参数可删除重复组中较旧的文件，仅保留修改时间最新的一个。
- `-out` 参数可将去重后的 PoC 复制到指定目录，方便单独归档。

### 环境要求
- Go 1.21+（或 `go.mod` 已声明的版本）。

### 构建
```bash
go build ./...
```

### 编译说明
- 确认 Go 版本满足 `go.mod` 要求，执行 `go env GOPATH` 检查环境。
- 首次在本机使用可运行 `go mod download` 拉取依赖。
- 日常开发可通过 `go test ./...` 快速验证无语法错误。
- 生成二进制：`go build -o repeaterxray.exe .`（Windows）或 `go build -o repeaterxray .`（macOS/Linux）。
- 可选：`go install ./...` 将程序安装到 `$GOBIN`/`$GOPATH/bin` 便于全局调用。

### 用法
```bash
# 基本语法
go run . -dir <path-to-pocs> [-delete] [-out <output-dir>]

# 仅输出重复报告
go run . -dir ./pocs

# 删除重复但保留最新版本
go run . -dir ./pocs -delete

# 导出去重后的新文件夹
go run . -dir ./pocs -out ./deduped

# 一次性删除并导出(多余功能)
go run . -dir ./pocs -delete -out ./deduped
```

- `-dir` 默认为当前目录，可输入相对或绝对路径。
- `-delete` 删除重复组中较旧文件，最终仅保留修改时间最新的一份。
- `-out` 会自动创建目录并保留相对 `-dir` 的目录结构，如存在同名文件将被覆盖。
- 删除操作不可逆，执行前请确认已备份或处于版本控制下。

### 输出示例
```
Detected 2 duplicated path groups:

Path: poc/linux/xxx
  - name="Example Vuln" file=./foo.yml modified=2024-05-12T10:03:27Z
  - name="Example Vuln" file=./bar.yml modified=2023-12-01T08:15:55Z
  * keep: ./foo.yml
```

### 输出目录说明
- `-out` 会将每个唯一 `path` 的最新 PoC 复制到输出目录。
- 输出目录会按相对 `-dir` 的路径结构创建，便于直接替换原 PoC 树。
- 若输出目录已存在同名文件，会被最新的去重结果覆盖。
- 复制过程对无重复的 PoC 同样适用，可当作“精选集”导出。

### 开发说明
- 全部逻辑集中在 `main.go`。
- 如需支持更多文件格式或自定义数据校验，可扩展 `isSupportedExt`、`loadPoC` 等函数。

