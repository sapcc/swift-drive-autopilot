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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

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
				//this is not an error if we can choose a swift-id in the next step
				if drive.StartedOutEmpty && len(Config.SwiftIDPool) > 0 {
					Log(LogInfo, "no swift-id file found on new device %s (mounted at %s), will try to assign one", drive.DevicePath, mountPath)
				} else {
					Log(LogError, "no swift-id file found on device %s (mounted at %s)", drive.DevicePath, mountPath)
				}
			} else {
				Log(LogError, "read %s: %s", idPath, err.Error())
			}
			success = false
			continue
		}
		swiftID := strings.TrimSpace(string(idBytes))

		//recognize spare disks
		drive.Spare = swiftID == "spare"
		if drive.Spare {
			drive.FinalMount.Name = "" //to skip it during the final mount
			continue                   //skip collision check
		}

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

//AutoAssignSwiftIDs will try to do exactly that.
func (c *Converger) AutoAssignSwiftIDs() {
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
				if drive.StartedOutEmpty && !drive.Broken {
					Log(LogError, "tried to assign swift-id to %s, but some drives are broken", drive.DevicePath)
				}
			}
			return
		}

		//mark assigned swift-ids
		if drive.Spare {
			//count how many spare disks exist by giving them names like "spare/0", "spare/1", etc.
			//(this is the same format in which spare disks are presented in the Config.SwiftIDPool)
			name := fmt.Sprintf("spare/%d", spareIdx)
			assigned[name] = true
			spareIdx++
		} else {
			assigned[drive.FinalMount.Name] = true
		}
	}

	//look for drives that are eligible for automatic swift-id assignment
	for _, drive := range c.Drives {
		if !drive.StartedOutEmpty || !drive.TemporaryMount.Active {
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
			Log(LogError, "tried to assign swift-id to %s, but pool is exhausted", drive.DevicePath)
			continue
		}

		swiftID := poolID
		if strings.HasPrefix(poolID, "spare/") {
			swiftID = "spare"
		}

		Log(LogInfo, "assigning swift-id '%s' to %s", swiftID, drive.DevicePath)

		//try to write the assignment to disk
		path := filepath.Join(drive.TemporaryMount.Path(), "swift-id")
		if Config.ChrootPath != "" {
			path = filepath.Join(Config.ChrootPath, strings.TrimPrefix(path, "/"))
		}
		err := ioutil.WriteFile(path, []byte(swiftID+"\n"), 0644)
		if err != nil {
			Log(LogError, err.Error())
			continue
		}

		assigned[poolID] = true
		drive.Spare = swiftID == "spare"
		if drive.Spare {
			drive.FinalMount.Name = "" //to skip it during the final mount
		} else {
			drive.FinalMount.Name = swiftID
		}
	}
}
