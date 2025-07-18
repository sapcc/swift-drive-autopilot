// SPDX-FileCopyrightText: 2016-2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/sapcc/go-bits/logg"
)

// Run is a shortcut for Command.Run() that just takes a command line.
func Run(cmd ...string) (string, bool) {
	return Command{}.Run(cmd...)
}

// Command contains optional parameters for Command.Run().
type Command struct {
	Stdin       string
	NoChroot    bool
	SkipLog     bool
	NoNsenter   bool
	ExitOnError bool
}

// Run executes the given command, possibly within the chroot (if
// configured in Config.ChrootPath, and if the first argument is true).
func (c Command) Run(cmd ...string) (stdout string, success bool) {
	cmdName := cmd[0]

	// if we are executing mount, we need to make sure that we are in the
	// correct mount namespace; for cryptsetup, we even need to be in the
	// correct IPC namespace (device-mapper wants to talk to udev)
	if !c.NoNsenter {
		switch cmd[0] {
		case "mount", "umount":
			cmd = append([]string{"nsenter", "--mount=/proc/1/ns/mnt", "--"}, cmd...)
		case "cryptsetup":
			cmd = append([]string{"nsenter", "--mount=/proc/1/ns/mnt", "--ipc=/proc/1/ns/ipc", "--"}, cmd...)
		}
	}

	// prepend chroot if requested (note that if there is a ChrootPath, it's our
	// cwd; and if there is none, our cwd is /, so this is a no-op)
	if !c.NoChroot {
		cmd = append([]string{"chroot", "."}, cmd...)
	}

	// become root if necessary (useful for development mode)
	if os.Geteuid() != 0 {
		cmd = append([]string{"sudo"}, cmd...)
	}

	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	logg.Debug("executing command: %v", cmd)
	execCmd := exec.Command(cmd[0], cmd[1:]...) //nolint:gosec // inputs are not user supplied
	execCmd.Stdout = stdoutBuf
	execCmd.Stderr = stderrBuf
	if c.Stdin != "" {
		execCmd.Stdin = bytes.NewReader([]byte(c.Stdin))
	}
	err := execCmd.Run()

	cmdForLog := strings.Join(cmd, " ")
	if !c.SkipLog {
		for line := range strings.SplitSeq(stderrBuf.String(), "\n") {
			if line != "" {
				log.Printf("Output from %s: %s\n", cmdName, line)
			}
		}
		if err != nil {
			logLevel := logg.Error
			if c.ExitOnError {
				logLevel = logg.Fatal
			}
			logLevel("exec(%s) failed: %s", cmdForLog, err.Error())
		}
	}

	stdout = stdoutBuf.String()
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			logg.Debug("exec(%s) produced stdout: %s", cmdForLog, line)
		}
	}
	return stdout, err == nil
}
