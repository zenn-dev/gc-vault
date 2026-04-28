package runner

import (
	"fmt"
	"os"
	"os/exec"
)

func Exec(profileName string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("no command specified")
	}

	s, err := newSession(profileName)
	if err != nil {
		return err
	}

	cmd := exec.Command(command[0], command[1:]...)
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
		return fmt.Errorf("running %s: %w", command[0], runErr)
	}
	return nil
}
