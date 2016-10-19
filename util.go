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
	ExecNormal        ExecMode = 0
	ExecChroot        ExecMode = 1 << 0
	ExecNsenter       ExecMode = 1 << 1
	ExecChrootNsenter ExecMode = ExecChroot | ExecNsenter
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

//Exec executes the given command, possibly within the chroot (if
//configured in Config.ChrootPath, and if the first argument is true).
func Exec(mode ExecMode, command string, args ...string) (stdout, stderr string, e error) {
	//if we are executing mount, we need to make sure that we are in the
	//correct mount namespace
	if (mode&ExecNsenter == ExecNsenter) && command == "mount" {
		args = append([]string{"--mount=/proc/1/ns/mnt", "--", "mount"}, args...)
		command = "nsenter"
	}

	//prepend `chroot $CHROOT_PATH` if requested
	if (mode&ExecChroot == ExecChroot) && Config.ChrootPath != "" {
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

//ExecSimple is like Exec, but error output from the called program is sent to
//stderr directly.
func ExecSimple(mode ExecMode, command string, args ...string) (string, error) {
	stdout, stderr, err := Exec(mode, command, args...)
	for _, line := range strings.Split(stderr, "\n") {
		if line != "" {
			log.Printf("Output from %s: %s\n", command, line)
		}
	}
	return stdout, err
}
