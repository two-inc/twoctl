package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/two-inc/twoctl-cli/internal/config"
)

func init() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage API contexts (named env + key combos)",
		Long: `Manage named contexts. Each context bundles a base URL and a keychain
entry, so you can switch between sandbox, prod, staging, cyber, perf,
release - or any custom environment - without copy-pasting URLs or keys.

  twoctl config set-context sandbox --url https://api.sandbox.two.inc --key secret_test_...
  twoctl config use-context sandbox
  twoctl config get-contexts        # show all contexts (* marks current)
  twoctl config current-context     # print the current name
  twoctl config delete-context temp

  twoctl --context cyber ...        # use a context for one command without switching
  twoctl --env prod ...             # alias of --context`,
	}
	configCmd.AddCommand(
		configSetContextCmd(),
		configUseContextCmd(),
		configGetContextsCmd(),
		configCurrentContextCmd(),
		configDeleteContextCmd(),
	)
	register(configCmd)
}

func configSetContextCmd() *cobra.Command {
	var urlFlag, keyFlag string
	cmd := &cobra.Command{
		Use:   "set-context <name>",
		Short: "Create or update a context",
		Long: `Create or update a context. --url defaults to https://api.<name>.two.inc
unless the name matches a built-in (prod, sandbox, staging, cyber, perf,
release). --key, when set, is written to the OS keychain immediately.

If no context is currently active, the new context becomes current.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetContext(args[0], urlFlag, keyFlag); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "context %q saved\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "API base URL (default: inferred from name)")
	cmd.Flags().StringVar(&keyFlag, "key", "", "API key to store in the keychain for this context")
	return cmd
}

func configUseContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use-context <name>",
		Short: "Set the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.UseContext(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q.\n", args[0])
			return nil
		},
	}
}

func configGetContextsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get-contexts",
		Short: "List all contexts (* marks the current one)",
		RunE: func(cmd *cobra.Command, args []string) error {
			contexts, current, err := config.ListContexts()
			if err != nil {
				return err
			}
			if len(contexts) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no contexts - run `twoctl config set-context <name> --url <url>` to create one")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "CURRENT\tNAME\tURL\tHAS-KEY")
			for _, c := range contexts {
				mark := ""
				if c.Name == current {
					mark = "*"
				}
				has := "no"
				if config.HasStoredKey(c.Name) {
					has = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, c.Name, c.BaseURL, has)
			}
			return tw.Flush()
		},
	}
}

func configCurrentContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current-context",
		Short: "Print the current context name",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadFile()
			if err != nil {
				return err
			}
			if cfg.CurrentContext == "" {
				return fmt.Errorf("no current context set")
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.CurrentContext)
			return nil
		},
	}
}

func configDeleteContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-context <name>",
		Short: "Remove a context and its stored key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.DeleteContext(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed context %q\n", args[0])
			return nil
		},
	}
}
