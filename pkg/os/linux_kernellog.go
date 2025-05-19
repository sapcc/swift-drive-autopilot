// SPDX-FileCopyrightText: 2016-2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package os

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
)

var klogErrorRx = regexp.MustCompile(`(?i)\b(?:error|metadata corruption detected|unmount and run xfs_repair)\b`)
var klogDeviceRx = regexp.MustCompile(`\b(sd[a-z]{1,2})\b`)

// CollectDriveErrors implements the Interface interface.
func (l *Linux) CollectDriveErrors(errors chan<- []DriveError) {
	// assemble commandline for journalctl (similar to logic in Command.Run()
	// which we cannot use here because we need a pipe on stdout)
	command := []string{"chroot", ".", "nsenter", "--ipc=/proc/1/ns/ipc", "--", "journalctl", "-kf"}
	if os.Geteuid() != 0 {
		command = append([]string{"sudo"}, command...)
	}

	cmd := exec.Command(command[0], command[1:]...) //nolint:gosec // inputs are not user supplied
	cmd.Stderr = os.Stderr
	stdout := must.Return(cmd.StdoutPipe())
	must.Succeed(cmd.Start())

	// wait for a few seconds before starting to read stuff, so that all the
	// DriveAddedEvents have already been sent
	time.Sleep(3 * time.Second)

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			logg.Error(err.Error())
		}
		//NOTE: no special handling of io.EOF here; we will encounter it very
		// frequently anyway while we're waiting for new log lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// we're looking for log lines with "error" and a disk device name like "sda"
		logg.Debug("received kernel log line: '%s'", line)
		if !klogErrorRx.MatchString(line) {
			continue
		}
		match := klogDeviceRx.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		errors <- []DriveError{{
			DevicePath: "/dev/" + match[1],
			Message:    line,
		}}
	}

	//NOTE: the loop above will never return, so I don't bother with cmd.Wait()
}
