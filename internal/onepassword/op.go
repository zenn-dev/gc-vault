package onepassword

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GetDocument(vault, item string) ([]byte, error) {
	cmd := exec.Command("op", "document", "get", item, "--vault", vault)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("op document get %q --vault %q: %s", item, vault, msg)
	}
	return stdout.Bytes(), nil
}

func WhoAmI() (string, error) {
	cmd := exec.Command("op", "whoami")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("op whoami: %s", msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
