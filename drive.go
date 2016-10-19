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
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"strings"
)

//Drive contains all the information about a single drive: its device path,
//where it is mounted, etc. All fields ending in "Path" are absolute and refer
//to a location in the chroot (if any).
type Drive struct {
	//DevicePath is where the device file is located (with all symlinks
	//resolved, e.g. "/dev/sdc" instead of "/dev/disk/by-path/...").
	DevicePath string
	//MountID is the name of the directory below /run/swift-storage where this
	//drive shall be mounted.
	MountID string
	//Mounted is true if this drive is mounted below /run/swift-storage.
	Mounted bool
	//SwiftID is the name of the directory below /srv/node where this
	//drive shall be mounted.
	SwiftID string
	//Mapped is true if this drive is mounted below /srv/node.
	Mapped bool
}

//Drives is a list of Drive structs with some extra methods.
type Drives []*Drive

//ListDrives returns the list of all Swift storage drives, by expanding the
//shell globs in Config.DriveGlobs and resolving any symlinks.
func ListDrives() Drives {
	var result Drives

	for _, pattern := range Config.DriveGlobs {
		//make pattern relative to current directory (== chroot directory)
		pattern = strings.TrimPrefix(pattern, "/")

		matches, err := filepath.Glob(pattern)
		if err != nil {
			Log(LogFatal, "glob(%#v) failed: %s", pattern, err.Error())
		}
		Log(LogDebug, "ListDrives: %s matches %#v", pattern, matches)

		for _, match := range matches {
			//resolve any symlinks to get the actual devicePath
			devicePath, err := filepath.EvalSymlinks(match)
			if err != nil {
				Log(LogFatal, "readlink(%#v) failed: %s", match, err.Error())
			}

			//make path absolute if necessary (the glob was a relative path!)
			if !strings.HasPrefix(devicePath, "/") {
				devicePath = "/" + devicePath
			}
			result = append(result, newDrive(devicePath))

			if devicePath != match {
				Log(LogDebug, "ListDrives: resolved %s to %s", match, devicePath)
			}
		}
	}

	return result
}

func newDrive(devicePath string) *Drive {
	//default value for MountID is md5sum of devicePath
	s := md5.Sum([]byte(devicePath))
	mountID := hex.EncodeToString(s[:])

	return &Drive{
		DevicePath: devicePath,
		MountID:    mountID,
		//Mounted and Mapped will be initialized by ScanDriveMountPoints()
		//SwiftID will be initialized by ScanDriveSwiftIDs()
	}
}
