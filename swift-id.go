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
	"path/filepath"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/core"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//ScanSwiftIDs inspects the "swift-id" file of all mounted drives and fills the
//SwiftID field of the drives accordingly, while also detecting ID collisions,
//and drives mounted below /srv/node with the wrong SwiftID.
func ScanSwiftIDs(drives []*core.Drive, osi os.Interface) {

	drivesBySwiftID := make(map[string]*core.Drive)
	for _, drive := range drives {
		//ignore broken drives and keep going
		mountedPath := drive.MountedPath()
		if mountedPath == "" {
			continue
		}

		//read this device's swift-id
		swiftID, err := osi.ReadSwiftID(mountedPath)
		if err != nil {
			util.LogError(err.Error())
			continue
		} else if swiftID == "" {
			//this is not an error if we can choose a swift-id in the next step
			if len(Config.SwiftIDPool) > 0 {
				util.LogInfo("no swift-id file found on new device %s (mounted at %s), will try to assign one", drive.DevicePath, mountedPath)
			} else {
				util.LogError("no swift-id file found on device %s (mounted at %s)", drive.DevicePath, mountedPath)
			}
			continue
		}

		//recognize spare disks
		if swiftID == "spare" {
			drive.SwiftID = &swiftID
			continue //skip collision check
		}

		//does this swift-id conflict with where the device is currently mounted?
		if filepath.Dir(mountedPath) == "/srv/node" && filepath.Base(mountedPath) != swiftID {
			util.LogError(
				"drive %s is currently mounted at %s, but its swift-id says \"%s\" (not going to touch it)",
				drive.DevicePath, mountedPath, swiftID)
		} else {
			//record swift-id for the final mount
			drive.SwiftID = &swiftID
		}

		//is this the first device with this swift-id?
		otherDrive, exists := drivesBySwiftID[swiftID]
		if exists {
			//no - do not mount any of them, just complain
			util.LogError("found multiple drives with swift-id \"%s\" (not mounting any of them)", swiftID)
			//clear swift-id for all drives involved in the collision, to
			//skip them during the final mount
			drive.SwiftID = nil
			otherDrive.SwiftID = nil
		} else {
			//yes - remember it to check for collisions later on
			drivesBySwiftID[swiftID] = drive
		}
	}
}

//AutoAssignSwiftIDs will try to do exactly that.
func (c *Converger) AutoAssignSwiftIDs(osi os.Interface) {
	//can only do something if a swift-id-pool is given in the config
	if len(Config.SwiftIDPool) == 0 {
		return
	}

	//tracks assigned swift-ids
	assigned := make(map[string]bool)
	spareIdx := 0

	for _, drive := range c.Drives {
		//do not do anything if any drive is broken (if a drive is broken, we
		//cannot look at its swift-id and thus cannot ensure that we don't
		//assign it to another drive)
		if drive.Broken {
			//complain about all the drives for which we could not assign a swift-id
			for _, drive := range c.Drives {
				if drive.SwiftID == nil && !drive.Broken {
					util.LogError("tried to assign swift-id to %s, but some drives are broken", drive.DevicePath)
				}
			}
			return
		}

		//mark assigned swift-ids
		if drive.SwiftID != nil {
			if *drive.SwiftID == "spare" {
				//count how many spare disks exist by giving them names like "spare/0", "spare/1", etc.
				//(this is the same format in which spare disks are presented in the Config.SwiftIDPool)
				name := fmt.Sprintf("spare/%d", spareIdx)
				assigned[name] = true
				spareIdx++
			} else {
				assigned[*drive.SwiftID] = true
			}
		}
	}

	//look for drives that are eligible for automatic swift-id assignment
	for _, drive := range c.Drives {
		if drive.SwiftID != nil || drive.Broken {
			continue
		}

		//try to find an unused swift-id
		//
		//WARNING: IDs are GUARANTEED by our interface contract to be assigned
		//in the order in which they appear in the configuration (see docs for
		//`swift-id-pool` in README).
		var poolID string
		for _, id := range Config.SwiftIDPool {
			if !assigned[id] {
				poolID = id
				break
			}
		}

		if poolID == "" {
			util.LogError("tried to assign swift-id to %s, but pool is exhausted", drive.DevicePath)
			continue
		}

		swiftID := poolID
		if strings.HasPrefix(poolID, "spare/") {
			swiftID = "spare"
		}

		util.LogInfo("assigning swift-id '%s' to %s", swiftID, drive.DevicePath)

		//try to write the assignment to disk
		err := osi.WriteSwiftID(drive.MountPath(), swiftID)
		if err != nil {
			util.LogError(err.Error())
			continue
		}

		assigned[poolID] = true
		drive.SwiftID = &swiftID
	}
}
