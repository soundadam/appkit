package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	appkit "github.com/soundadam/appkit/go"
)

var demo = appkit.App{Name: "demo", Version: "1.2.3", Repo: "owner/demo"}

func run(root *cobra.Command, args ...string) (string, error) {
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestVersionFlagAndCommand(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"version"}} {
		root := &cobra.Command{Use: "demo", Run: func(*cobra.Command, []string) {}}
		AddCommands(root, demo)
		if out, err := run(root, args...); err != nil || out != "demo 1.2.3\n" {
			t.Errorf("%v: %q, %v", args, out, err)
		}
	}
}

func TestTheAppsOwnVersionCommandWins(t *testing.T) {
	root := &cobra.Command{Use: "demo"}
	root.AddCommand(&cobra.Command{Use: "version", Run: func(cmd *cobra.Command, _ []string) {
		cmd.Print("build identity")
	}})
	AddCommands(root, demo)
	if out, _ := run(root, "version"); out != "build identity" {
		t.Errorf("version printed %q", out)
	}
}

func TestDevBuildsRefuseToUpdate(t *testing.T) {
	root := &cobra.Command{Use: "demo", SilenceUsage: true}
	AddCommands(root, appkit.App{Name: "demo", Version: "dev", Repo: "owner/demo"})
	if out, err := run(root, "update", "--check"); err == nil || !strings.Contains(err.Error(), "development build") {
		t.Errorf("update --check on a dev build: %q, %v", out, err)
	}
}

func TestAutoTurnsAutomaticChecksOnAndOff(t *testing.T) {
	// os.UserConfigDir 在各平台读的变量都指到临时目录
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
	for _, value := range []string{"off", "on"} {
		root := &cobra.Command{Use: "demo"}
		AddCommands(root, demo)
		if out, err := run(root, "update", "--auto", value); err != nil {
			t.Fatalf("--auto %s: %q, %v", value, out, err)
		}
		if got := appkit.LoadAutoCheck(demo).Enabled(); got != (value == "on") {
			t.Errorf("after --auto %s, enabled = %v", value, got)
		}
	}
	root := &cobra.Command{Use: "demo", SilenceUsage: true}
	AddCommands(root, demo)
	if _, err := run(root, "update", "--auto", "maybe"); err == nil {
		t.Error("--auto maybe was accepted")
	}
}
