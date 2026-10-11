//! soundadam 应用的公共底座：版本、安装来源、检查更新与自更新、统一的命令行。
//! 发布产物命名、Cask、命令行与更新的约定见仓库根目录的 README。

pub mod cli;
mod install;
mod release;
#[cfg(feature = "egui")]
pub mod ui;
mod update;

pub use install::Install;
pub use release::{Asset, Release};
pub use semver::Version;
pub use update::{apply, brew_upgrade, check, relaunch, AutoCheck};

use std::fmt;

/// 一个应用的身份。用 [`app!`] 从 Cargo 元数据构造。
#[derive(Clone, Copy, Debug)]
pub struct App {
    /// 可执行文件名，也是 crate 名、Cask 名、产物文件名前缀。
    pub name: &'static str,
    /// macOS 的 `<display_name>.app`，也是窗口标题。
    pub display_name: &'static str,
    pub version: &'static str,
    /// GitHub 上的 `owner/repo`。
    pub repo: &'static str,
}

/// `appkit::app!("Tonescope")`：名字、版本、仓库取自调用方的 Cargo.toml。
#[macro_export]
macro_rules! app {
    ($display_name:expr) => {
        $crate::App::new(
            env!("CARGO_PKG_NAME"),
            $display_name,
            env!("CARGO_PKG_VERSION"),
            env!("CARGO_PKG_REPOSITORY"),
        )
    };
}

impl App {
    /// `repository` 必须是 `https://github.com/<owner>/<repo>`。
    pub fn new(
        name: &'static str,
        display_name: &'static str,
        version: &'static str,
        repository: &'static str,
    ) -> Self {
        let repo = repository
            .strip_prefix("https://github.com/")
            .expect("Cargo.toml repository must be https://github.com/<owner>/<repo>")
            .trim_end_matches('/');
        Self {
            name,
            display_name,
            version,
            repo,
        }
    }

    pub fn version(&self) -> Version {
        Version::parse(self.version).expect("CARGO_PKG_VERSION is semver")
    }

    /// 仓库主的 tap 里的同名 Cask：`<owner>/tap/<name>`（Formula 装的也是这个全名）。
    pub fn cask(&self) -> String {
        let owner = self.repo.split('/').next().unwrap_or(self.repo);
        format!("{owner}/tap/{}", self.name)
    }
}

/// 给人看的错误：一句话说清哪一步失败、为什么。
#[derive(Debug, Clone)]
pub struct Error(String);

impl Error {
    fn new(what: impl fmt::Display, why: impl fmt::Display) -> Self {
        Self(format!("{what}: {why}"))
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for Error {}

pub type Result<T> = std::result::Result<T, Error>;
