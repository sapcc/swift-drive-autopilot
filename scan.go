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
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tempMountRx = regexp.MustCompile(`^/run/swift-storage/([^/]+)$`)
var finalMountRx = regexp.MustCompile(`^/srv/node/([^/]+)$`)

//ScanMountPoints looks through the active mounts to check which of the given
//drives are already mounted below /run/swift-storage or /srv/node. If so, the
//`MountID/Mounted` and `SwiftID/Mapped` fields of the drives are initialized
//accordingly.
func (drives Drives) ScanMountPoints() {
	stdout, err := ExecSimple(ExecChroot, nil, "mount")
	if err != nil {
		Log(LogFatal, "exec(mount) failed: %s", err.Error())
	}

	for _, line := range strings.Split(stdout, "\n") {
		//line looks like "<device> on <mountpoint> type <type> (<options>)"
		words := strings.Split(line, " ")
		if len(words) < 3 || words[1] != "on" {
			continue
		}
		devicePath, mountPath := words[0], words[2]

		//does `devicePath` refer to a Drive that we know?
		var drive *Drive
		for _, theDrive := range drives {
			if devicePath == theDrive.ActiveDevicePath() {
				drive = theDrive
				break
			}
		}
		if drive == nil {
			continue
		}

		//is this a mount in which we are interested?
		match := tempMountRx.FindStringSubmatch(mountPath)
		if match != nil {
			drive.TemporaryMount.Name = match[1]
			drive.TemporaryMount.Active = true
			continue
		}
		match = finalMountRx.FindStringSubmatch(mountPath)
		if match != nil {
			drive.FinalMount.Name = match[1]
			drive.FinalMount.Active = true
			continue
		}
	}

	for _, drive := range drives {
		devicePath := drive.ActiveDevicePath()
		drive.TemporaryMount.ReportToDebugLog("ScanMountPoints", devicePath)
		drive.FinalMount.ReportToDebugLog("ScanMountPoints", devicePath)
	}
}

//ScanSwiftIDs inspects the "swift-id" file of all mounted drives and fills the
//SwiftID field of the drives accordingly, while also detecting ID collisions,
//and drives mounted below /srv/node with the wrong SwiftID.
func (drives Drives) ScanSwiftIDs() (success bool) {
	success = true //until proven otherwise

	drivesBySwiftID := make(map[string]*Drive)
	for _, drive := range drives {
		//find a path where this drive is mounted
		var mountPath string
		switch {
		case drive.FinalMount.Active:
			mountPath = drive.FinalMount.Path()
		case drive.TemporaryMount.Active:
			mountPath = drive.TemporaryMount.Path()
		}
		//if a drive could not be mounted because of an earlier error, ignore
		//it and keep going
		if mountPath == "" {
			continue
		}

		//read this device's swift-id
		idPath := filepath.Join(mountPath, "swift-id")
		idBytes, err := readFileFromChroot(idPath)
		if err != nil {
			if os.IsNotExist(err) {
				Log(LogError, "no swift-id file found on device %s (mounted at %s)", drive.DevicePath, mountPath)
			} else {
				Log(LogError, "read /%s: %s", idPath, err.Error())
			}
			success = false
			continue
		}
		swiftID := strings.TrimSpace(string(idBytes))

		//does this swift-id conflict with where the device is currently mounted?
		if drive.FinalMount.Active && drive.FinalMount.Name != swiftID {
			Log(LogError,
				"drive %s is currently mounted at /srv/node/%s, but its swift-id says \"%s\" (not going to touch it)",
				drive.DevicePath, drive.FinalMount.Name, swiftID)
			drive.FinalMount.Name = "" //to skip it during the final mount
		} else {
			//record swift-id for the final mount
			drive.FinalMount.Name = swiftID
		}

		//is this the first device with this swift-id?
		otherDrive, exists := drivesBySwiftID[swiftID]
		if exists {
			//no - do not mount any of them, just complain
			Log(LogError, "found multiple drives with swift-id \"%s\" (not mounting any of them)", swiftID)
			//clear swift-id for all drives involved in the collision, to
			//skip them during the final mount
			drive.FinalMount.Name = ""
			otherDrive.FinalMount.Name = ""
		} else {
			//yes - remember it to check for collisions later on
			drivesBySwiftID[swiftID] = drive
		}
	}

	return success
}

func readFileFromChroot(path string) ([]byte, error) {
	if Config.ChrootPath != "" {
		path = filepath.Join(Config.ChrootPath, strings.TrimPrefix(path, "/"))
	}
	return ioutil.ReadFile(path)
}
