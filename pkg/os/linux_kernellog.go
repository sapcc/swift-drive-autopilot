/*******************************************************************************
*
* Copyright 2016-2018 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package os

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

var klogErrorRx = regexp.MustCompile(`(?i)\b(?:error|metadata corruption detected|unmount and run xfs_repair)\b`)
var klogDeviceRx = regexp.MustCompile(`\b(sd[a-z]{1,2})\b`)

// CollectDriveErrors implements the Interface interface.
func (l *Linux) CollectDriveErrors(errors chan<- []DriveError) {
	//assemble commandline for journalctl (similar to logic in Command.Run()
	//which we cannot use here because we need a pipe on stdout)
	command := []string{"chroot", ".", "nsenter", "--ipc=/proc/1/ns/ipc", "--", "journalctl", "-kf"}
	if os.Geteuid() != 0 {
		command = append([]string{"sudo"}, command...)
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		util.LogFatal(err.Error())
	}
	err = cmd.Start()
	if err != nil {
		util.LogFatal(err.Error())
	}

	//wait for a few seconds before starting to read stuff, so that all the
	//DriveAddedEvents have already been sent
	time.Sleep(3 * time.Second)

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			util.LogError(err.Error())
		}
		//NOTE: no special handling of io.EOF here; we will encounter it very
		//frequently anyway while we're waiting for new log lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		//we're looking for log lines with "error" and a disk device name like "sda"
		util.LogDebug("received kernel log line: '%s'", line)
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
