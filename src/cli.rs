//! 每个应用共用的命令行：不带参数打开界面，另有 `--version` 与 `update [--check]`。

use crate::{update, App, AutoCheck, Install};
use std::io::Write;

/// 在 `main` 最前面调用。返回 `Some(退出码)` 表示命令已处理完，直接退出；
/// `None` 表示照常启动界面。
pub fn run(app: &App) -> Option<i32> {
    // 老版本 macOS 从 Finder 启动时会带一个 -psn_ 参数
    let args: Vec<String> = std::env::args()
        .skip(1)
        .filter(|a| !a.starts_with("-psn_"))
        .collect();
    if args.is_empty() {
        #[cfg(target_os = "macos")]
        if hand_off_to_launch_services() {
            return Some(0);
        }
        return None;
    }
    #[cfg(windows)]
    attach_console();

    let args: Vec<&str> = args.iter().map(String::as_str).collect();
    Some(match args.as_slice() {
        ["-V" | "--version"] => {
            println!("{} {}", app.name, app.version);
            0
        }
        ["-h" | "--help"] => {
            println!("{}", help(app));
            0
        }
        ["update"] => update_command(app, false),
        ["update", "--check"] => update_command(app, true),
        _ => {
            eprintln!("{}", help(app));
            2
        }
    })
}

fn help(app: &App) -> String {
    let n = app.name;
    format!(
        "{n} {}\n\n\
         Usage:\n  \
         {n}                  open {}\n  \
         {n} update           update to the latest release\n  \
         {n} update --check   only report whether a newer release exists\n  \
         {n} --version        print the version",
        app.version, app.display_name
    )
}

fn update_command(app: &App, check_only: bool) -> i32 {
    let found = match update::check(app) {
        Ok(found) => found,
        Err(e) => {
            eprintln!("{e}");
            return 1;
        }
    };
    AutoCheck::load(app).record(found.as_ref().map(|r| &r.version));
    let Some(release) = found else {
        println!("{} {} is the latest release.", app.name, app.version);
        return 0;
    };
    println!(
        "{} {} is available (installed: {}).\n{}",
        app.name, release.version, app.version, release.page
    );
    if check_only {
        return 0;
    }

    let install = Install::detect(app);
    let result = match &install {
        Install::Homebrew { brew, .. } => {
            println!("Upgrading with Homebrew…");
            update::brew_upgrade(app, brew)
        }
        Install::Dev => {
            eprintln!("This is a development build; it does not update itself.");
            return 1;
        }
        _ => {
            let mut shown = None;
            update::apply(app, &install, &release, |done, total| {
                let pct = (done * 100).checked_div(total);
                if pct.is_some() && pct != shown {
                    shown = pct;
                    eprint!("\rDownloading… {:3}%", pct.unwrap_or(0));
                    let _ = std::io::stderr().flush();
                }
            })
        }
    };
    eprintln!();
    match result {
        Ok(()) => {
            println!("Updated to {} {}.", app.name, release.version);
            0
        }
        Err(e) => {
            eprintln!("{e}");
            1
        }
    }
}

/// 从终端直接跑 `.app` 里的可执行文件时，窗口挂在终端名下：麦克风等权限记在终端头上，
/// 关掉终端程序也跟着退。改由 LaunchServices 打开 bundle，与在 Finder 里双击一样；
/// 已经开着就只是切到前台。
#[cfg(target_os = "macos")]
fn hand_off_to_launch_services() -> bool {
    // 父进程是 launchd：本来就是 LaunchServices 启动的
    if std::os::unix::process::parent_id() == 1 {
        return false;
    }
    let bundle = std::env::current_exe()
        .and_then(|p| p.canonicalize())
        .ok()
        .and_then(|p| crate::install::bundle_of(&p));
    let Some(bundle) = bundle else { return false };
    std::process::Command::new("/usr/bin/open")
        .arg(&bundle)
        .status()
        .is_ok_and(|s| s.success())
}

/// release 版是 GUI 子系统，没有自己的控制台；接到启动它的那个终端上，输出才看得见。
#[cfg(windows)]
fn attach_console() {
    use windows_sys::Win32::System::Console::{AttachConsole, ATTACH_PARENT_PROCESS};
    // 失败（已有控制台、或不是从终端启动）就照旧，没有别的影响
    unsafe {
        AttachConsole(ATTACH_PARENT_PROCESS);
    }
}
