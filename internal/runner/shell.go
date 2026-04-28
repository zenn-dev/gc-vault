package runner

import (
	"fmt"
	"os"
	"os/exec"
)

func Shell(profileName string) error {
	s, err := newSession(profileName)
	if err != nil {
		return err
	}

	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/bash"
	}

	fmt.Fprintf(os.Stderr, "gc-vault: starting subshell with profile %q (exit to leave)\n", profileName)

	cmd := exec.Command(shellPath)
	cmd.Env = s.env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	runErr := cmd.Run()
	s.cleanup()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("running shell %s: %w", shellPath, runErr)
	}
	return nil
}
