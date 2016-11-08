/*******************************************************************************
*
* Copyright 2016 SAP SE
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

package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"strings"
)

type ExecMode int

const (
	ExecNormal          ExecMode = 0
	ExecChroot          ExecMode = 1
	ExecChrootNoNsenter ExecMode = 2
)

type LogLevel int

const (
	LogFatal LogLevel = iota
	LogError
	LogInfo
	LogDebug
)

var logLevelNames = []string{"FATAL", "ERROR", "INFO", "DEBUG"}

var isDebug = os.Getenv("DEBUG") != ""

//Log writes a log message. LogDebug messages are only written if
//the environment variable `DEBUG` is set.
func Log(level LogLevel, msg string, args ...interface{}) {
	if level == LogDebug && !isDebug {
		return
	}

	if len(args) > 0 {
		log.Printf(logLevelNames[level]+": "+msg+"\n", args...)
	} else {
		log.Println(logLevelNames[level] + ": " + msg)
	}

	if level == LogFatal {
		os.Exit(1)
	}
}

//Command contains optional parameters for Command.Run().
type Command struct {
	Stdin       string
	NoChroot    bool
	SkipLog     bool
	NoNsenter   bool
	ExitOnError bool
}

//Run executes the given command, possibly within the chroot (if
//configured in Config.ChrootPath, and if the first argument is true).
func (c Command) Run(cmd ...string) (stdout string, success bool) {
	//if we are executing mount, we need to make sure that we are in the
	//correct mount namespace; for cryptsetup, we even need to be in the
	//correct IPC namespace (device-mapper wants to talk to udev)
	if !c.NoNsenter {
		switch cmd[0] {
		case "mount", "umount":
			cmd = append([]string{"nsenter", "--mount=/proc/1/ns/mnt", "--"}, cmd...)
		case "cryptsetup":
			cmd = append([]string{"nsenter", "--mount=/proc/1/ns/mnt", "--ipc=/proc/1/ns/ipc", "--"}, cmd...)
		}
	}

	//prepend `chroot $CHROOT_PATH` if requested
	if !c.NoChroot && Config.ChrootPath != "" {
		cmd = append([]string{"chroot", Config.ChrootPath}, cmd...)
	}

	//become root if necessary (useful for development mode)
	if os.Geteuid() != 0 {
		cmd = append([]string{"sudo"}, cmd...)
	}

	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	execCmd := exec.Command(cmd[0], cmd[1:]...)
	execCmd.Stdout = stdoutBuf
	execCmd.Stderr = stderrBuf
	if c.Stdin != "" {
		execCmd.Stdin = bytes.NewReader([]byte(c.Stdin))
	}
	err := execCmd.Run()

	if !c.SkipLog {
		for _, line := range strings.Split(string(stderrBuf.Bytes()), "\n") {
			if line != "" {
				log.Printf("Output from %s: %s\n", cmd[0], line)
			}
		}
		if err != nil {
			logLevel := LogError
			if c.ExitOnError {
				logLevel = LogFatal
			}
			Log(logLevel, "exec(%s) failed: %s", strings.Join(cmd, " "), err.Error())
		}
	}

	return string(stdoutBuf.Bytes()), err == nil
}

//Run is a shortcut for Command.Run() that just takes a command line.
func Run(cmd ...string) (string, bool) {
	return Command{}.Run(cmd...)
}
