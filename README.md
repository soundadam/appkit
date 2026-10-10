# appkit

soundadam 桌面应用的公共底座：版本、安装来源、检查更新与自更新、统一的命令行，外加可选的 egui 控件。
下面的约定对每个接入它的应用都成立；[tonescope](https://github.com/soundadam/tonescope) 是参考实现，
打包脚本、release workflow、Cask 模板、图标都可以照它抄。

## 接入

```toml
[dependencies]
appkit = { git = "https://github.com/soundadam/appkit", tag = "v0.1.0", features = ["egui"] }
```

`Cargo.toml` 的 `repository` 必须是 `https://github.com/<owner>/<repo>`。

```rust
fn main() -> eframe::Result {
    let app = appkit::app!("Tonescope"); // macOS 上的 Tonescope.app，也是窗口标题
    if let Some(code) = appkit::cli::run(&app) {
        std::process::exit(code);
    }
    // ……照常启动界面，构造时 appkit::ui::Updater::new(app, &cc.egui_ctx)
}
```

界面里调 `updater.top_bar(ui)`（顶栏右侧，有新版才出现）和 `updater.settings(ui)`（设置里的 Updates 一节）。
不用 egui 的应用不开 `egui` feature，直接用 `appkit::check` / `apply` / `brew_upgrade` / `relaunch`。

## 版本

- `Cargo.toml` 的 `version` 是唯一的版本号；发布推 `v<version>` tag，release workflow 先核对两者一致。
- 更新只看 GitHub 的 `releases/latest`：草稿和预发布（tag 里带 `-`）不会推给用户。

## 发布产物

每个 Release 带三份，名字由 `<name>`（crate 名）与版本拼成：

| 平台 | 文件 | 内容 |
| --- | --- | --- |
| macOS | `<name>-<ver>-macos-universal.zip` | `<显示名>.app`，通用二进制，ad-hoc 签名，`ditto -c -k` 打包 |
| Linux | `<name>-<ver>-linux-<arch>.tar.gz` | `<name>-<ver>-linux-<arch>/<name>` |
| Windows | `<name>-<ver>-windows-<arch>.zip` | `<name>-<ver>-windows-<arch>/<name>.exe` |

`<arch>` 是 `uname -m` / Rust 的 `std::env::consts::ARCH`（`x86_64`、`aarch64`）。
更新时按 GitHub 给每个产物算的 SHA-256（API 里的 `digest`）校验，对不上或没有就不装。

## Homebrew

`<owner>/homebrew-tap` 里的同名 Cask，`brew install --cask <owner>/tap/<name>`。
macOS 装 `.app`，再用 `binary` 把 bundle 里的可执行文件链到 PATH；Linux 装 `binary`。
Cask 模板放在应用仓的 `packaging/homebrew/<name>.rb`，release workflow 填好版本与 SHA-256 后推到 tap。

## 命令行

| 命令 | 作用 |
| --- | --- |
| `<name>` | 打开界面。macOS 上从终端运行时交给 LaunchServices 打开 `.app`（与在 Finder 里双击相同，麦克风等权限不记在终端名下） |
| `<name> update` | 查最新版并更新 |
| `<name> update --check` | 只报告有没有新版 |
| `<name> --version` | 打印 `<name> <version>` |

Windows 的 release 构建没有控制台窗口，带参数运行时接到启动它的终端上输出。

## 更新

怎么更新取决于这份程序是怎么装的（`Install::detect`）：

| 安装方式 | 判断依据 | 更新 |
| --- | --- | --- |
| Homebrew | 可执行文件在 `<prefix>/Caskroom/<name>/` 下，或 `.app` 在 Applications 且有 Caskroom 记录 | `brew update` 后 `brew upgrade --cask <owner>/tap/<name>` |
| macOS `.app` | 可执行文件在 `X.app/Contents/MacOS/` 下 | 下载、校验、解压到同目录，整个 bundle 原地替换 |
| 单个可执行文件 | 其余 | 同上，替换可执行文件本身（Windows 上运行中也能换） |
| 开发构建 | 路径里有 `target/debug` 或 `target/release` | 不更新 |

新版在重启后生效；界面上的 Restart 按钮启动新的一份再退出旧的。

自动检查默认开着，至多一天一次；查到新版但还没装上时每次启动都查，让提示一直挂着。
开关与上次检查时间存在系统配置目录下的 `<name>/update.json`。
联网只有两种请求：`GET api.github.com/repos/<owner>/<repo>/releases/latest`，以及用户点了更新之后下载产物。
代理取自 `https_proxy` 等环境变量。

## 本地检查

```bash
cargo fmt --check && cargo clippy --all-targets --all-features -- -D warnings && cargo test --all-features
```

## 许可证

[MIT](LICENSE-MIT) 或 [Apache-2.0](LICENSE-APACHE)，任选其一。
