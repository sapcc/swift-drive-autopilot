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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//Chown changes the ownership of the path to the given user and
//group. Both arguments may either be a name or a numeric ID (but still given
//as a string in decimal).
func Chown(path, user, group string) {
	var (
		cmd string
		arg string
	)

	if path == "" {
		util.LogFatal("Cannot chown empty path")
	}

	//set only those things which were given
	if user == "" {
		if group == "" {
			return // nothing to do
		}
		cmd, arg = "chgrp", group
	} else {
		cmd, arg = "chown", user
		if group != "" {
			arg += ":" + group
		}
	}

	util.LogDebug("%s %s to %s", cmd, path, arg)
	command.Run(cmd, arg, path)
}

//ForeachSymlinkIn finds all symlinks in the given directory, and calls the
//handler once for each symlink (with its file name and link target). Any
//errors because of filesystem operations will be logged to LogError and false
//will be returned if any such error occurred.
func ForeachSymlinkIn(path string, handler func(name, target string)) (success bool) {
	dir, err := os.Open(path)
	if err != nil {
		util.LogError(err.Error())
		return false
	}
	fis, err := dir.Readdir(-1)
	if err != nil {
		util.LogError(err.Error())
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
			util.LogError(err.Error())
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
