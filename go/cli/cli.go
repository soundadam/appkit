// Package cli 给 cobra 的根命令补上每个应用共用的命令：
// --version、version、update [--check]，以及命令跑完后的新版提示。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	appkit "github.com/soundadam/appkit/go"
)

// AddCommands 设好 root 的 --version（打印 "<name> <version>"），加上 update 子命令
// （update [--check]，以及开关自动检查的 update --auto on|off）；
// root 还没有 version 子命令时也加上。
func AddCommands(root *cobra.Command, app appkit.App) {
	root.Version = app.Version
	root.SetVersionTemplate(app.Name + " {{.Version}}\n")
	root.AddCommand(updateCommand(app))
	for _, c := range root.Commands() {
		if c.Name() == "version" {
			return
		}
	}
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), app.Name, app.Version)
		},
	})
}

func updateCommand(app appkit.App) *cobra.Command {
	var checkOnly bool
	var auto string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("auto") {
				return setAuto(app, auto, cmd.OutOrStdout())
			}
			return update(cmd.Context(), app, checkOnly, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only report whether a newer release exists")
	cmd.Flags().StringVar(&auto, "auto", "", "Turn automatic update checks on or off")
	cmd.MarkFlagsMutuallyExclusive("check", "auto")
	return cmd
}

func setAuto(app appkit.App, value string, stdout io.Writer) error {
	var on bool
	switch value {
	case "on":
		on = true
	case "off":
	default:
		return fmt.Errorf(`--auto takes "on" or "off", not %q`, value)
	}
	appkit.LoadAutoCheck(app).SetEnabled(on)
	fmt.Fprintf(stdout, "Automatic update checks are %s.\n", value)
	return nil
}

func update(ctx context.Context, app appkit.App, checkOnly bool, stdout, stderr io.Writer) error {
	found, err := appkit.Check(ctx, app)
	if err != nil {
		return err
	}
	appkit.LoadAutoCheck(app).Record(found)
	if found == nil {
		fmt.Fprintf(stdout, "%s %s is the latest release.\n", app.Name, app.Version)
		return nil
	}
	fmt.Fprintf(stdout, "%s %s is available (installed: %s).\n%s\n", app.Name, found.Version, app.Version, found.Page)
	if checkOnly {
		return nil
	}

	inst := appkit.Detect(app)
	switch inst.Kind {
	case appkit.Homebrew:
		fmt.Fprintln(stdout, "Upgrading with Homebrew…")
		err = appkit.BrewUpgrade(ctx, app, inst)
	case appkit.Dev:
		return errors.New("this is a development build; it does not update itself")
	default:
		shown := int64(-1)
		err = appkit.Apply(ctx, app, inst, *found, func(done, total int64) {
			if total <= 0 {
				return
			}
			if pct := done * 100 / total; pct != shown {
				shown = pct
				fmt.Fprintf(stderr, "\rDownloading… %3d%%", pct)
			}
		})
		fmt.Fprintln(stderr)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Updated to %s %s.\n", app.Name, found.Version)
	return nil
}

// Notice 是命令跑完后往 stderr 写的一行新版提示。该自动检查时在后台查，不拖慢命令本身；
// 命令结束时还没查完就不等，下次再说。stderr 不是终端时提示不会显示，也就不查。
type Notice struct {
	app   appkit.App
	auto  *appkit.AutoCheck
	found chan *appkit.Release
}

// StartNotice 在执行根命令之前调用。
func StartNotice(app appkit.App) *Notice {
	n := &Notice{app: app, auto: appkit.LoadAutoCheck(app), found: make(chan *appkit.Release, 1)}
	if n.auto.Due(app) && terminal(os.Stderr) {
		go func() {
			if found, err := appkit.Check(context.Background(), app); err == nil {
				n.found <- found
			}
		}()
	}
	return n
}

// Print 在执行根命令之后调用，w 是 os.Stderr：知道有新版就往 w 写一行提示。
// w 不是终端时什么都不写，免得混进管道和脚本的输出。
func (n *Notice) Print(w io.Writer) {
	select {
	case found := <-n.found:
		n.auto.Record(found)
	default:
	}
	version := n.auto.Pending(n.app)
	if version == "" || appkit.Updated() || !terminal(w) {
		return
	}
	fmt.Fprintf(w, "\n%s %s is available (installed: %s). Run `%s update` to install it.\n",
		n.app.Name, version, n.app.Version, n.app.Name)
}

func terminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
