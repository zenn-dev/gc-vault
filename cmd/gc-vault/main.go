package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/zenn-dev/gc-vault/internal/config"
	"github.com/zenn-dev/gc-vault/internal/doctor"
	"github.com/zenn-dev/gc-vault/internal/runner"
	"github.com/zenn-dev/gc-vault/internal/version"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "gc-vault",
		Short:         "Secure GCP credential management via service account impersonation",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	cmd.AddCommand(
		newExecCmd(),
		newListCmd(),
		newShellCmd(),
		newDoctorCmd(),
		newVersionCmd(),
	)
	return cmd
}

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec PROFILE -- COMMAND [ARGS...]",
		Short: "Execute a command with impersonated credentials",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Exec(args[0], args[1:])
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Profiles))
			for n := range cfg.Profiles {
				names = append(names, n)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PROFILE\tPROJECT\tTARGET SA\tLIFETIME")
			for _, n := range names {
				p := cfg.Profiles[n]
				fmt.Fprintf(w, "%s\t%s\t%s\t%ds\n", n, p.Project, p.TargetSA, p.Lifetime)
			}
			return w.Flush()
		},
	}
}

func newShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell PROFILE",
		Short: "Start a subshell with impersonated credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Shell(args[0])
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose local setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doctor.Run()
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gc-vault %s\n", version.Version)
		},
	}
}
