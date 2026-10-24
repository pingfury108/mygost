# mygost

[gost](https://github.com/go-gost/gost) 的桌面客户端，基于 [MyGo](https://mygo.egoist.dev) 原生 UI（纯 Go、GPU 绘制、无 webview）。管理本机 gost 进程：配置档、启动/停止/重启、日志。

## 功能

- **配置档**：三种类型
  - 代理（http / socks5 / relay，可选认证、可选上游转发链）—— 表单生成 gost.yml
  - 端口转发（tcp / udp，多目标 = 负载均衡）—— 表单生成
  - 自定义 YAML —— 内置 Zed 级代码编辑器，YAML 高亮，完整 gost 能力
- **启停控制**：启动 / 停止（SIGTERM，2s 后强杀）/ 重启，运行时长，异常退出原因展示
- **日志**：环形缓冲保留最近 5000 行，关键词过滤、复制、清空
- **托盘**：图标反映运行状态（实心=运行，空心=停止），菜单快速启停
- **gost 管理**：自动从 GitHub Releases 下载（sha256 校验），支持自定义镜像模板与本地路径；可选嵌入构建
- 其他：单实例、开机自启、启动自动运行、退出时保持 gost 运行

## 开发

```bash
go tool mygo dev      # 运行（改动 .go 自动重启）
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go tool mygo build    # 打包到 build/
```

## gost 二进制的查找顺序

1. 设置中手动指定的路径
2. 嵌入构建内含的二进制（解压到数据目录）
3. 数据目录 `bin/`（此前下载过）
4. 系统 `PATH`
5. 提示下载（默认 GitHub Releases v3.3.1，可在设置改镜像模板，支持 `{version} {os} {arch} {ext}`）

## 嵌入构建（可选）

默认构建不嵌入 gost（体积小）。要开箱即用，先放对应平台的 gost 二进制到
`internal/gostbin/gost`，再带 `embedded` tag 构建：

```bash
# 示例：macOS arm64
curl -L https://github.com/go-gost/gost/releases/download/v3.3.1/gost_3.3.1_darwin_arm64.tar.gz \
  | tar -xzO gost > internal/gostbin/gost
go build -tags embedded -o build/mygost
```

## 目录结构

```
main.go               应用状态、窗口（430×660 窄窗口）、生命周期
app_view.go           根视图（主界面 + 推入式子页）+ 设计系统组件
view_main.go          主界面：连接大按钮、配置列表、互斥启停
view_profiles.go      新建/编辑表单（代理 / 端口转发 / YAML）
view_logs.go          日志
view_settings.go      设置
tray.go               托盘图标与菜单（与 app 图标同一标记）
theme.go              设计 token（间距/圆角/字号）、主题派生、语义柔色
tools/icongen/        图标生成器：go run tools/icongen/main.go
internal/settings/    设置持久化（JSON）
internal/profiles/    配置档存储 + gost.yml 生成
internal/gostbin/     二进制定位、下载、（可选）嵌入
internal/proc/        gost 子进程托管 + 日志环形缓冲
```

界面为 OpenVPN Connect 式窄窗口：顶部大连接按钮（一次只运行一个配置），
中部配置列表（行内连接/断开，⋯ 菜单编辑/删除），底部日志/设置入口。

数据目录：`os.UserConfigDir()/mygost`（macOS `~/Library/Application Support/mygost`）。
