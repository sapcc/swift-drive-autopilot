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
	"path/filepath"
	"strings"
)

//Drive contains all the information about a single drive: its device path,
//where it is mounted, etc. All fields ending in "Path" are relative to the
//Config.ChrootPath (or to "/" if no chroot is configured).
type Drive struct {
	//DevicePath is where the device file is located (with all symlinks
	//resolved, e.g. "/dev/sdc" instead of "/dev/disk/by-path/...").
	DevicePath string
}

//ListDrives returns the list of all Swift storage drives, by expanding the
//shell globs in Config.DriveGlobs and resolving any symlinks.
func ListDrives() []*Drive {
	var result []*Drive

	for _, pattern := range Config.DriveGlobs {
		//make pattern relative to current directory (== chroot directory)
		pattern = strings.TrimPrefix(pattern, "/")

		matches, err := filepath.Glob(pattern)
		if err != nil {
			Log(LogFatal, "glob(%#v) failed: %s", pattern, err.Error())
		}
		Log(LogDebug, "ListDrives: %s matches %#v", pattern, matches)

		for _, match := range matches {
			devicePath, err := filepath.EvalSymlinks(match)
			if err != nil {
				Log(LogFatal, "readlink(%#v) failed: %s", match, err.Error())
			}

			//path might have become absolute because of this, so make it
			//relative to the current directory (== chroot directory) again
			devicePath = strings.TrimPrefix(devicePath, "/")
			result = append(result, &Drive{DevicePath: devicePath})

			if devicePath != match {
				Log(LogDebug, "ListDrives: resolved %s to %s", match, devicePath)
			}
		}
	}

	return result
}
