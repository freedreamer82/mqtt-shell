package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const ShellToUse = "bash"

func Shellout(command string, timeout time.Duration) (error, string) {

	if timeout > 0 {
		// Create a new context and add a timeout to it
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel() // The cancel should be deferred so resources are cleaned up

		// Create the command with our context
		cmd := exec.CommandContext(ctx, ShellToUse, "-c", command)
		cmd.Env = append(os.Environ())
		// This time we can simply use Output() to get the result.
		out, err := cmd.CombinedOutput()

		// We want to check the context error to see if the timeout was executed.
		// The error returned by cmd.Output() will be OS specific based on what
		// happens when a process is killed.
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Println("Command timed out")
			return errors.New("Timeout"), ""
		}

		// If there's no context error, we know the command completed (or errored).
		return err, string(out)
	} else {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd := exec.Command(ShellToUse, "-c", command)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return err, stdout.String() + stderr.String()
	}

}
