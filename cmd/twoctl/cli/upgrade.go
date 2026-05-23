package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/two-inc/twoctl-cli/internal/httpx"
	"github.com/two-inc/twoctl-cli/internal/updater"
)

// flagNoUpgradeCheck lets a user (or scripts) suppress the auto-check
// without persisting the preference.
var flagNoUpgradeCheck bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagNoUpgradeCheck, "no-upgrade-check", false, "skip the once-per-day upgrade check for this run")

	// The PreRun fires for every subcommand. We register it on the root
	// and let cobra propagate.
	rootCmd.PersistentPreRunE = autoCheckHook

	upgrade := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for, and install, a newer twoctl release",
		Long: `Check GitHub for a newer twoctl release and install it in place.

The CLI also performs a once-per-day check on every invocation and prompts
you if a new version is available. Use --reset-skips to forget any versions
you previously chose to skip, or --disable-autocheck to turn the prompt off
entirely.`,
		RunE: runUpgrade,
	}
	upgrade.Flags().Bool("reset-skips", false, "forget all skipped versions and re-enable the prompt for them")
	upgrade.Flags().Bool("disable-autocheck", false, "stop the daily check-on-invocation")
	upgrade.Flags().Bool("enable-autocheck", false, "re-enable the daily check-on-invocation")
	upgrade.Flags().Bool("check", false, "only report whether an upgrade is available, do not install")

	register(upgrade)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	resetSkips, _ := cmd.Flags().GetBool("reset-skips")
	disable, _ := cmd.Flags().GetBool("disable-autocheck")
	enable, _ := cmd.Flags().GetBool("enable-autocheck")
	checkOnly, _ := cmd.Flags().GetBool("check")

	if resetSkips || disable || enable {
		state, err := updater.LoadState()
		if err != nil {
			return err
		}
		if resetSkips {
			state.SkippedVersions = nil
			fmt.Fprintln(cmd.OutOrStdout(), "cleared skipped versions")
		}
		if disable {
			f := false
			state.AutoCheck = &f
			fmt.Fprintln(cmd.OutOrStdout(), "auto-check disabled")
		}
		if enable {
			t := true
			state.AutoCheck = &t
			fmt.Fprintln(cmd.OutOrStdout(), "auto-check enabled")
		}
		if err := updater.SaveState(state); err != nil {
			return err
		}
		if !checkOnly && !resetSkips {
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()

	rel, err := updater.LatestRelease(ctx)
	if err != nil {
		return err
	}
	if !updater.IsNewer(httpx.Version, rel.TagName) {
		fmt.Fprintf(cmd.OutOrStdout(), "already up to date (twoctl %s)\n", httpx.Version)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "new version: %s (current: %s)\n", rel.TagName, httpx.Version)
	if checkOnly {
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "downloading...")
	if err := updater.Apply(ctx, rel); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "upgraded to %s\n", rel.TagName)
	return nil
}

// autoCheckHook runs before every command. If a newer release is available
// and the user is on a real TTY, prompt them; otherwise silently note that
// an upgrade is available in stderr.
//
// Failures (network, permissions, anything) never block the actual command.
func autoCheckHook(cmd *cobra.Command, args []string) error {
	if flagNoUpgradeCheck || os.Getenv("TWOCTL_SKIP_UPGRADE_CHECK") != "" {
		return nil
	}
	// Skip when the user is already running `twoctl upgrade` - the
	// command handles its own logic.
	if cmd.Name() == "upgrade" {
		return nil
	}
	// And skip auth subcommands since prompting during a login flow
	// would clobber the password prompt.
	if cmd.Parent() != nil && cmd.Parent().Name() == "auth" {
		return nil
	}

	// Tight context just for the metadata fetch; the prompt + download
	// path uses a longer-lived context below so a slow connection can
	// actually complete the multi-MB binary download.
	metaCtx, metaCancel := context.WithTimeout(cmd.Context(), 3*time.Second)
	defer metaCancel()

	rel, err := updater.AutoCheck(metaCtx, httpx.Version)
	if err != nil || rel == nil {
		return nil
	}
	// AutoCheck caches the full Release payload (assets included), so
	// the prompt path no longer needs a second LatestRelease call - that
	// would have silently defeated the once-per-day throttle.

	stderr := cmd.ErrOrStderr()
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		// Non-interactive: just nudge.
		fmt.Fprintf(stderr, "[twoctl] %s is available (you are on %s). Run `twoctl upgrade` to install.\n",
			rel.TagName, displayCurrent())
		return nil
	}

	// Long context for download + binary swap on slow hotel WiFi.
	applyCtx, applyCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer applyCancel()
	_, err = updater.AskAndApply(applyCtx, rel, httpx.Version, os.Stdin, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "[twoctl] upgrade prompt failed: %v\n", err)
	}
	return nil
}

func displayCurrent() string {
	if httpx.Version == "" || httpx.Version == "dev" {
		return "a development build"
	}
	return httpx.Version
}
