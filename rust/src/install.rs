use crate::App;
use std::path::{Path, PathBuf};

/// 正在运行的这份程序是怎么装上的，决定更新走哪条路。
#[derive(Clone, Debug, PartialEq)]
pub enum Install {
    /// Homebrew 装的（Cask，或 Formula）。升级交给 `brew`，自己不动文件，否则 brew 的记录就错了。
    Homebrew {
        brew: PathBuf,
        /// 升级后重新启动用的路径：macOS 上 Cask 是 `.app`，其余是 `<prefix>/bin/<name>`
        /// （Caskroom、Cellar 里带版本号的目录升级后就没了）。
        launch: PathBuf,
        /// 是 Formula（在 Cellar 里）而不是 Cask。
        formula: bool,
    },
    /// macOS 上从 Releases 解压出来的 `.app`，整个 bundle 原地替换。
    Bundle(PathBuf),
    /// 单个可执行文件，原地替换。
    Executable(PathBuf),
    /// cargo 构建目录里跑的，不更新。
    Dev,
}

impl Install {
    pub fn detect(app: &App) -> Self {
        let exe = std::env::current_exe().and_then(|p| p.canonicalize());
        match exe {
            Ok(exe) => classify(app, &exe, &homebrew_prefixes()),
            Err(_) => Self::Dev,
        }
    }

    /// 能不能不借助别的工具把自己换成新版。
    pub fn self_updates(&self) -> bool {
        matches!(self, Self::Bundle(_) | Self::Executable(_))
    }
}

fn homebrew_prefixes() -> Vec<PathBuf> {
    let mut v: Vec<PathBuf> = std::env::var_os("HOMEBREW_PREFIX")
        .map(PathBuf::from)
        .into_iter()
        .collect();
    v.extend(
        ["/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"]
            .iter()
            .map(PathBuf::from),
    );
    v.extend(dirs::home_dir().map(|h| h.join(".linuxbrew")));
    v
}

/// 可执行文件所在的最外层 `.app`（`X.app/Contents/…/x`）。
/// `Contents` 下哪个目录都算：`MacOS` 里的主程序，`Helpers` 里随 `.app` 发的命令行也是。
pub(crate) fn bundle_of(exe: &Path) -> Option<PathBuf> {
    let parts: Vec<_> = exe.components().collect();
    let i = parts.windows(2).position(|w| {
        Path::new(w[0].as_os_str()).extension() == Some("app".as_ref())
            && w[1].as_os_str() == "Contents"
    })?;
    Some(parts[..=i].iter().collect())
}

fn classify(app: &App, exe: &Path, prefixes: &[PathBuf]) -> Install {
    let parts: Vec<_> = exe.components().map(|c| c.as_os_str()).collect();
    let in_target = parts
        .iter()
        .position(|c| *c == "target")
        .is_some_and(|i| parts[i..].iter().any(|c| *c == "debug" || *c == "release"));
    if in_target {
        return Install::Dev;
    }

    let bundle = bundle_of(exe);
    for prefix in prefixes {
        let caskroom = prefix.join("Caskroom").join(app.name);
        let brew = prefix.join("bin/brew");
        let launch = prefix.join("bin").join(app.name);
        // Formula 的 opt 与 bin 下的链接都解到 Cellar 里带版本号的目录
        if exe.starts_with(prefix.join("Cellar").join(app.name)) {
            return Install::Homebrew {
                brew,
                launch,
                formula: true,
            };
        }
        // Linux 上 Cask 的 binary 是指进 Caskroom 的符号链接
        if exe.starts_with(&caskroom) {
            return Install::Homebrew {
                brew,
                launch,
                formula: false,
            };
        }
        // macOS 上 Cask 把 .app 移到 Applications，Caskroom 里只留记录
        if let Some(b) = &bundle {
            let in_applications = b.parent().is_some_and(|d| d.ends_with("Applications"));
            if in_applications && caskroom.is_dir() {
                return Install::Homebrew {
                    brew,
                    launch: b.clone(),
                    formula: false,
                };
            }
        }
    }
    match bundle {
        Some(b) => Install::Bundle(b),
        None => Install::Executable(exe.to_path_buf()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const APP: App = App {
        name: "demo",
        display_name: "Demo",
        version: "1.0.0",
        repo: "owner/demo",
    };

    #[test]
    fn cargo_build_dirs_are_dev() {
        for exe in [
            "/src/demo/target/debug/demo",
            "/src/demo/target/x86_64-pc-windows-msvc/release/demo.exe",
        ] {
            assert_eq!(classify(&APP, Path::new(exe), &[]), Install::Dev);
        }
    }

    #[test]
    fn linux_cask_binary_resolves_into_caskroom() {
        let prefix = PathBuf::from("/home/linuxbrew/.linuxbrew");
        let exe = prefix.join("Caskroom/demo/1.0.0/demo-1.0.0-linux-x86_64/demo");
        assert_eq!(
            classify(&APP, &exe, std::slice::from_ref(&prefix)),
            Install::Homebrew {
                brew: prefix.join("bin/brew"),
                launch: prefix.join("bin/demo"),
                formula: false,
            }
        );
    }

    #[test]
    fn a_formula_binary_resolves_into_the_cellar() {
        let prefix = PathBuf::from("/home/linuxbrew/.linuxbrew");
        let prefixes = std::slice::from_ref(&prefix);
        assert_eq!(
            classify(&APP, &prefix.join("Cellar/demo/1.0.0/bin/demo"), prefixes),
            Install::Homebrew {
                brew: prefix.join("bin/brew"),
                launch: prefix.join("bin/demo"),
                formula: true,
            }
        );
        // 名字开头相同的另一个应用的 Cellar 不算
        let other = prefix.join("Cellar/demo-extra/1.0.0/bin/demo");
        assert_eq!(
            classify(&APP, &other, prefixes),
            Install::Executable(other.clone())
        );
    }

    #[test]
    fn app_in_applications_is_homebrew_only_with_a_caskroom_record() {
        let prefix = tempfile::tempdir().unwrap();
        let exe = Path::new("/Applications/Demo.app/Contents/MacOS/demo");
        let prefixes = [prefix.path().to_path_buf()];
        assert_eq!(
            classify(&APP, exe, &prefixes),
            Install::Bundle("/Applications/Demo.app".into())
        );

        std::fs::create_dir_all(prefix.path().join("Caskroom/demo")).unwrap();
        assert!(matches!(
            classify(&APP, exe, &prefixes),
            Install::Homebrew { .. }
        ));
        assert!(matches!(
            classify(
                &APP,
                Path::new("/Applications/Demo.app/Contents/Helpers/demo"),
                &prefixes
            ),
            Install::Homebrew { .. }
        ));
        // 解压在别处的另一份不算 brew 的
        assert_eq!(
            classify(
                &APP,
                Path::new("/tmp/x/Demo.app/Contents/MacOS/demo"),
                &prefixes
            ),
            Install::Bundle("/tmp/x/Demo.app".into())
        );
    }

    #[test]
    fn the_outermost_app_above_contents_is_the_bundle() {
        for exe in [
            "/tmp/x/Demo.app/Contents/MacOS/demo",
            "/tmp/x/Demo.app/Contents/Helpers/demo",
            "/tmp/x/Demo.app/Contents/Helpers/Login.app/Contents/MacOS/login",
        ] {
            assert_eq!(
                bundle_of(Path::new(exe)),
                Some(PathBuf::from("/tmp/x/Demo.app")),
                "{exe}"
            );
        }
        assert_eq!(bundle_of(Path::new("/home/u/my.app/bin/demo")), None);
    }

    #[test]
    fn a_loose_binary_is_executable() {
        let exe = Path::new("/home/u/bin/demo");
        assert_eq!(classify(&APP, exe, &[]), Install::Executable(exe.into()));
    }
}
