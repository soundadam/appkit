//! 每个应用共用的命令行，建在 clap 上：应用用 derive 写自己的参数与子命令，
//! appkit 在同一棵命令树上补 `update [--check]`、`version` 与 `--version`。

use crate::{update, App, AutoCheck, Install};
use clap::{Arg, ArgAction, Command, CommandFactory, Parser};
use std::ffi::OsString;
use std::io::Write;

/// 应用没有自己的参数与子命令时用它：`appkit::cli::parse::<NoArgs>(&app)`。
// clap 默认把文档注释当成 --help 的说明，这里的注释是写给开发者的
#[derive(Parser, Debug)]
#[command(about = None, long_about = None)]
pub struct NoArgs {}

/// 在 `main` 最前面调用。appkit 自己的命令在这里执行并退出进程，
/// 参数有错时打印用法并以 2 退出；其余返回应用自己的参数。
pub fn parse<C: Parser>(app: &App) -> C {
    // 老版本 macOS 从 Finder 启动时会带一个 -psn_ 参数
    let args: Vec<OsString> = std::env::args_os()
        .filter(|a| !a.to_string_lossy().starts_with("-psn_"))
        .collect();
    #[cfg(target_os = "macos")]
    if args.len() == 1 && hand_off_to_launch_services() {
        std::process::exit(0);
    }
    #[cfg(windows)]
    if args.len() > 1 {
        attach_console();
    }
    match parse_from::<C>(app, args) {
        Ok(Parsed::App(c)) => c,
        Ok(Parsed::Version) => {
            println!("{} {}", app.name, app.version);
            std::process::exit(0)
        }
        Ok(Parsed::Update { check_only }) => std::process::exit(update_command(app, check_only)),
        Err(e) => e.exit(),
    }
}

#[derive(Debug)]
enum Parsed<C> {
    App(C),
    Version,
    Update { check_only: bool },
}

/// 应用的命令树加上 appkit 的几条。
fn command<C: CommandFactory>(app: &App) -> Command {
    C::command()
        .name(app.name)
        .bin_name(app.name)
        .version(app.version)
        .subcommand(
            Command::new("update")
                .about("Update to the latest release")
                .arg(
                    Arg::new("check")
                        .long("check")
                        .action(ArgAction::SetTrue)
                        .help("Only report whether a newer release exists"),
                ),
        )
        .subcommand(Command::new("version").about("Print the version"))
}

fn parse_from<C: Parser>(app: &App, args: Vec<OsString>) -> Result<Parsed<C>, clap::Error> {
    let mut cmd = command::<C>(app);
    let matches = cmd.try_get_matches_from_mut(args)?;
    match matches.subcommand() {
        Some(("update", sub)) => Ok(Parsed::Update {
            check_only: sub.get_flag("check"),
        }),
        Some(("version", _)) => Ok(Parsed::Version),
        _ => C::from_arg_matches(&matches)
            .map(Parsed::App)
            .map_err(|e| e.format(&mut cmd)),
    }
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
    // 只有 bundle 的主程序（Contents/MacOS 里）才交出去；Helpers 里的命令行照常跑
    let bundle = std::env::current_exe()
        .and_then(|p| p.canonicalize())
        .ok()
        .filter(|p| p.parent().is_some_and(|d| d.ends_with("Contents/MacOS")))
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

#[cfg(test)]
mod tests {
    use super::*;
    use clap::error::ErrorKind;

    const APP: App = App {
        name: "demo",
        display_name: "Demo",
        version: "1.2.3",
        repo: "owner/demo",
    };

    #[derive(Parser, Debug)]
    struct Cli {
        #[arg(long)]
        verbose: bool,
        #[command(subcommand)]
        cmd: Option<Cmd>,
    }

    #[derive(clap::Subcommand, Debug, PartialEq)]
    enum Cmd {
        Record { out: String },
    }

    fn parse(args: &[&str]) -> Result<Parsed<Cli>, clap::Error> {
        parse_from(&APP, args.iter().map(OsString::from).collect())
    }

    #[test]
    fn the_app_keeps_its_own_flags_and_subcommands() {
        let Ok(Parsed::App(cli)) = parse(&["demo", "--verbose", "record", "x.wav"]) else {
            panic!()
        };
        assert!(cli.verbose);
        assert_eq!(
            cli.cmd,
            Some(Cmd::Record {
                out: "x.wav".into()
            })
        );
        assert!(matches!(
            parse(&["demo"]),
            Ok(Parsed::App(Cli { cmd: None, .. }))
        ));
    }

    #[test]
    fn appkit_adds_update_and_version() {
        assert!(matches!(
            parse(&["demo", "update"]),
            Ok(Parsed::Update { check_only: false })
        ));
        assert!(matches!(
            parse(&["demo", "update", "--check"]),
            Ok(Parsed::Update { check_only: true })
        ));
        assert!(matches!(parse(&["demo", "version"]), Ok(Parsed::Version)));
        let e = parse(&["demo", "--version"]).unwrap_err();
        assert_eq!(e.kind(), ErrorKind::DisplayVersion);
        assert_eq!(e.to_string(), "demo 1.2.3\n");
    }

    #[test]
    fn an_app_without_arguments_has_no_description_of_its_own() {
        assert_eq!(command::<NoArgs>(&APP).get_about(), None);
    }

    #[test]
    fn an_app_without_arguments_rejects_them() {
        let parsed = parse_from::<NoArgs>(&APP, vec!["demo".into(), "bogus".into()]);
        assert_eq!(parsed.unwrap_err().kind(), ErrorKind::InvalidSubcommand);
    }
}
