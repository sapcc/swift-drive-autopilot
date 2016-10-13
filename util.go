package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"strings"
)

//ExecChroot executes the given command, possibly within the chroot (if
//configured in Config.ChrootPath).
func ExecChroot(command string, args ...string) (stdout, stderr string, e error) {
	//if we are executing mount, we need to make sure that we are in the
	//correct mount namespace
	if command == "mount" {
		args = append([]string{"--mount=/proc/1/ns/mnt", "--", "mount"}, args...)
		command = "nsenter"
	}

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

	log.Printf("DEBUG: execute %s %s\n", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	return string(stdoutBuf.Bytes()), string(stderrBuf.Bytes()), err
}

//ExecChrootSimple is like ExecChroot, but error output from the called program
//is sent to stderr directly.
func ExecChrootSimple(command string, args ...string) (string, error) {
	stdout, stderr, err := ExecChroot(command, args...)
	for _, line := range strings.Split(stderr, "\n") {
		if line != "" {
			log.Printf("Output from %s: %s\n", command, line)
		}
	}
	return stdout, err
}
