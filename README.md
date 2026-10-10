# appkit

soundadam 应用的公共底座：版本、安装来源、检查更新与自更新、统一的命令行。
这份 README 写的是约定本身，与语言无关；`rust/` 与 `go/` 是它的两份实现，接入一个应用只是加依赖、在入口处调一次。

参考实现：图形界面看 [tonescope](https://github.com/soundadam/tonescope)（Rust + egui），
打包脚本、release workflow、Cask 模板、图标都可以照它抄。

## 接入

### Rust

```toml
[dependencies]
appkit = { git = "https://github.com/soundadam/appkit", tag = "rust/v0.2.0", features = ["egui"] }
clap = { version = "4", features = ["derive"] }
```

`Cargo.toml` 的 `repository` 必须是 `https://github.com/<owner>/<repo>`。

```rust
/// 应用自己的参数与子命令照常用 clap derive 写；没有就用 appkit::cli::NoArgs。
#[derive(clap::Parser)]
struct Cli {
    #[command(subcommand)]
    command: Option<Command>,
}

fn main() -> eframe::Result {
    let app = appkit::app!("Tonescope"); // macOS 上的 Tonescope.app，也是窗口标题
    let cli: Cli = appkit::cli::parse(&app); // update、version 在这里跑完就退出
    // ……没有子命令就照常启动界面，构造时 appkit::ui::Updater::new(app, &cc.egui_ctx)
}
```

界面里调 `updater.top_bar(ui)`（顶栏右侧，有新版才出现）和 `updater.settings(ui)`（设置里的 Updates 一节）。
不用 egui 的应用不开 `egui` feature，直接用 `appkit::check` / `apply` / `brew_upgrade` / `relaunch`。

### Go

```bash
go get github.com/soundadam/appkit/go@go/v0.1.0
```

```go
var version = "dev" // 发布构建用 -ldflags "-X main.version=<ver>" 注入

func main() {
	app := appkit.App{Name: "demo", Version: version, Repo: "soundadam/demo"}
	root := newRootCommand() // 应用自己的 cobra 命令树
	cli.AddCommands(root, app)
	notice := cli.StartNotice(app)
	err := root.Execute()
	notice.Print(os.Stderr)
	// ……
}
```

`go.mod` 的 `toolchain` 写一个仍受支持、`govulncheck` 报不出可达标准库漏洞的补丁版本：
`go` 一行只是语言版本下限，发布出去的二进制用哪个 Go 编译看 `toolchain`。
CI 与 release workflow 用 `actions/setup-go@v7` 的 `go-version-file: go.mod`，它优先读 `toolchain`。

bubbletea 界面里嵌一个 `tui.New(app)`：model 的 `Init`、`Update` 转给它，`View` 里放它的那一行。
它有新版时提示、按 `u`（`Updater.Key`）在后台更新并显示进度。

## 版本

- 版本号只有一处：Rust 是 `Cargo.toml` 的 `version`；Go 是构建时注入的 `main.version`，取自 tag。
  发布推 `v<version>` tag，release workflow 先核对两者一致。
- 更新只看 GitHub 的 `releases/latest`：草稿和预发布（tag 里带 `-`）不会推给用户。

## 发布产物

每个 Release 按平台各带一份，名字由 `<name>`（可执行文件名）与版本拼成：

| 平台 | 文件 | 内容 |
| --- | --- | --- |
| macOS，带 `.app` | `<name>-<ver>-macos-universal.zip` | `<显示名>.app`，通用二进制，ad-hoc 签名，`ditto -c -k` 打包 |
| macOS，单个可执行文件 | `<name>-<ver>-macos-<arch>.tar.gz` | `<name>-<ver>-macos-<arch>/<name>` |
| Linux | `<name>-<ver>-linux-<arch>.tar.gz` | `<name>-<ver>-linux-<arch>/<name>` |
| Windows | `<name>-<ver>-windows-<arch>.zip` | `<name>-<ver>-windows-<arch>/<name>.exe` |

`<arch>` 是 `x86_64` 或 `aarch64`（Rust 的 `std::env::consts::ARCH`，Go 的 `amd64` / `arm64` 照此换名）。
更新时按 GitHub 给每个产物算的 SHA-256（API 里的 `digest`）校验，对不上或没有就不装。

## Homebrew

`<owner>/homebrew-tap` 里的同名 Cask，`brew install --cask <owner>/tap/<name>`。
macOS 带 `.app` 的装 `.app`，再用 `binary` 把 bundle 里的可执行文件链到 PATH；其余装 `binary`。
Cask 模板放在应用仓的 `packaging/homebrew/<name>.rb`，release workflow 填好版本与 SHA-256 后推到 tap。

## 命令行

| 命令 | 作用 |
| --- | --- |
| `<name>` | 应用的默认行为：图形界面打开窗口，其余由应用自己定 |
| `<name> update` | 查最新版并更新 |
| `<name> update --check` | 只报告有没有新版 |
| `<name> version`、`<name> --version` | 打印 `<name> <version>`；应用自己有 `version` 子命令的用它自己的 |

应用自己的参数与子命令和这几条在同一棵命令树上（Rust 用 clap，Go 用 cobra）。

- 带 `.app` 的应用在 macOS 上从终端不带参数运行时，交给 LaunchServices 打开 `.app`
  （与在 Finder 里双击相同，麦克风等权限不记在终端名下）。
- Windows 的图形界面 release 构建没有控制台窗口，带参数运行时接到启动它的终端上输出。
- 命令行应用在命令跑完后，知道有新版就往 stderr 写一行提示；stderr 不是终端时不写。

## 更新

怎么更新取决于这份程序是怎么装的（Rust `Install::detect`，Go `appkit.Detect`）：

| 安装方式 | 判断依据 | 更新 |
| --- | --- | --- |
| Homebrew | 可执行文件在 `<prefix>/Caskroom/<name>/` 下，或 `.app` 在 Applications 且有 Caskroom 记录 | `brew update` 后 `brew upgrade --cask <owner>/tap/<name>` |
| macOS `.app` | 可执行文件在 `X.app/Contents/MacOS/` 下 | 下载、校验、解压到同目录，整个 bundle 原地替换 |
| 单个可执行文件 | 其余 | 同上，替换可执行文件本身 |
| 开发构建 | Rust：路径里有 `target/debug` 或 `target/release`；Go：版本号不是 semver（没注入，是 `dev`） | 不更新 |

- 新版在重启后生效；图形界面上的 Restart 按钮启动新的一份再退出旧的。
- Windows 上运行中的 exe 删不掉，只能改名让位；换下来的旧文件晚些再删（Rust 在程序退出后，Go 在下次更新时）。
- 自动检查默认开着，至多一天一次；查到新版但还没装上时每次启动都查，让提示一直挂着。
  开关、上次检查时间与查到的版本存在系统配置目录下的 `<name>/update.json`。
- 联网只有两种请求：`GET api.github.com/repos/<owner>/<repo>/releases/latest`，以及用户要更新之后下载产物。
  代理取自 `https_proxy` 等环境变量。

## 发版

两份实现各自打 tag：Rust 是 `rust/v<version>`（与 `rust/Cargo.toml` 一致），Go 是 `go/v<version>`（Go 子模块的 tag 格式）。
约定变了，同一个改动里改这份 README 与两份实现。

## 本地检查

```bash
(cd rust && cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test --all-features) &&
(cd go && test -z "$(gofmt -l .)" && go vet ./... && go test ./...)
```

## 许可证

[MIT](LICENSE-MIT) 或 [Apache-2.0](LICENSE-APACHE)，任选其一。
