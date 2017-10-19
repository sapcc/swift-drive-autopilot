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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

func init() {
	log.SetOutput(os.Stdout)
}

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
	cmdName := cmd[0]

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
		for _, line := range strings.Split(stderrBuf.String(), "\n") {
			if line != "" {
				log.Printf("Output from %s: %s\n", cmdName, line)
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

	return stdoutBuf.String(), err == nil
}

//Run is a shortcut for Command.Run() that just takes a command line.
func Run(cmd ...string) (string, bool) {
	return Command{}.Run(cmd...)
}

//Chown changes the ownership of the path to the given user and
//group. Both arguments may either be a name or a numeric ID (but still given
//as a string in decimal).
func Chown(path, user, group string) {
	var (
		command string
		arg     string
	)

	if path == "" {
		Log(LogFatal, "Cannot chown empty path")
	}

	//set only those things which were given
	if user == "" {
		if group == "" {
			return // nothing to do
		}
		command, arg = "chgrp", group
	} else {
		command, arg = "chown", user
		if group != "" {
			arg += ":" + group
		}
	}

	Log(LogDebug, "%s %s to %s", command, path, arg)
	Run(command, arg, path)
}

//ForeachSymlinkIn finds all symlinks in the given directory, and calls the
//handler once for each symlink (with its file name and link target). Any
//errors because of filesystem operations will be logged to LogError and false
//will be returned if any such error occurred.
func ForeachSymlinkIn(path string, handler func(name, target string)) (success bool) {
	dir, err := os.Open(path)
	if err != nil {
		Log(LogError, err.Error())
		return false
	}
	fis, err := dir.Readdir(-1)
	if err != nil {
		Log(LogError, err.Error())
		return false
	}

	success = true
	for _, fi := range fis {
		if (fi.Mode() & os.ModeType) != os.ModeSymlink {
			continue
		}
		linkTarget, err := os.Readlink(filepath.Join(path, fi.Name()))
		if err == nil {
			handler(fi.Name(), linkTarget)
		} else {
			Log(LogError, err.Error())
			success = false
		}
	}

	return
}

//EvalSymlinksInChroot is like filepath.EvalSymlinks(), but considers that the
//given path is inside the chroot directory.
func EvalSymlinksInChroot(path string) (string, error) {
	//make path relative to current directory (== chroot directory)
	path = strings.TrimPrefix(path, "/")

	result, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("readlink(%#v) failed: %s", filepath.Join("/", path), err.Error())
	}

	//make path absolute again
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	return result, nil
}
