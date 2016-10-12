package main

import (
	"bytes"
	"os"
	"os/exec"
)

//ExecChroot executes the given command, possibly within the chroot (if
//configured in Config.ChrootPath).
func ExecChroot(command string, args ...string) (stdout, stderr string, e error) {
	//prepend `chroot $CHROOT_PATH` if requested
	if Config.ChrootPath != "" {
		args = append([]string{Config.ChrootPath, command}, args...)
		command = "chroot"
	}

	//become root if necessary (useful for development mode)
	if os.Geteuid() != 0 {
		args = append([]string{command}, args...)
		command = "sudo"
	}

	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	cmd := exec.Command(command, args...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	return string(stdoutBuf.Bytes()), string(stderrBuf.Bytes()), err
}
