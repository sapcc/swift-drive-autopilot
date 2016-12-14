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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

//Converger contains the internal state of the converger thread.
type Converger struct {
	//long-lived state
	Drives []*Drive

	//short-lived state that is gathered before the event handlers run
	ActiveLUKSMappings map[string]string
	ActiveMounts       SystemMountPoints
}

//RunConverger runs the converger thread. This function does not return.
func RunConverger(queue chan []Event) {
	c := &Converger{}

	for {
		//wait for processable events
		events := <-queue

		//initialize short-lived state for this event loop iteration
		c.ActiveLUKSMappings = ScanLUKSMappings()
		Log(LogDebug, "ActiveLUKSMappings = %#v", c.ActiveLUKSMappings)
		c.ActiveMounts = ScanMountPoints()
		for _, mount := range c.ActiveMounts {
			Log(LogDebug, "ActiveMounts += %#v", mount)
		}

		for _, drive := range c.Drives {
			drive.Converged = false
		}

		//handle events
		for _, event := range events {
			Log(LogInfo, "event received: "+event.LogMessage())
			event.Handle(c)
		}

		c.Converge()
	}
}

//Converge moves towards the desired state of all drives after a set of events
//has been received and handled by the converger.
func (c *Converger) Converge() {
	for _, drive := range c.Drives {
		drive.Converge(c)
	}

	//discover and auto-assign swift-ids of drives
	Drives(c.Drives).ScanSwiftIDs()
	c.AutoAssignSwiftIDs()

	for _, drive := range c.Drives {
		if !drive.Broken {
			if drive.FinalMount.Activate(drive.ActiveDevicePath()) {
				drive.FinalMount.Chown(Config.Owner.User, Config.Owner.Group)
			}
			drive.CleanupDuplicateMounts()
		}
	}

	c.CheckForUnexpectedMounts()
	c.WriteDriveAudit()

	//mark storage as ready for consumption by Swift
	Command{ExitOnError: true}.Run("touch", "/run/swift-storage/state/flag-ready")
}

//AutoAssignSwiftIDs will try to do exactly that.
func (c *Converger) AutoAssignSwiftIDs() {
	//tracks assigned swift-ids
	assigned := make(map[string]bool)

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
		assigned[drive.FinalMount.Name] = true
	}

	//look for drives that are eligible for automatic swift-id assignment
	for _, drive := range c.Drives {
		if !drive.StartedOutEmpty || !drive.TemporaryMount.Active {
			continue
		}

		//try to find an unused swift-id
		var swiftID string
		for _, id := range Config.SwiftIDPool {
			if !assigned[id] {
				swiftID = id
				break
			}
		}

		if swiftID == "" {
			Log(LogError, "tried to assign swift-id to %s, but pool is exhausted", drive.DevicePath)
			continue
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

		assigned[swiftID] = true
		drive.FinalMount.Name = swiftID
	}
}

//CheckForUnexpectedMounts prints error messages for every unexpected mount
//below /srv/node.
func (c *Converger) CheckForUnexpectedMounts() {
	for _, mount := range c.ActiveMounts {
		if mount.Location != "/srv/node" || !mount.Active {
			continue
		}

		found := false
		for _, drive := range c.Drives {
			if drive.FinalMount.Active && drive.FinalMount.Name == mount.Name {
				found = true
				break
			}
		}

		if !found {
			Log(LogError, "unexpected mount at %s", mount.Path())
		}
	}
}

//WriteDriveAudit writes /var/cache/swift/drive.recon in the same format as
//emitted by swift-drive-audit.
func (c *Converger) WriteDriveAudit() {
	data := make(map[string]int)
	total := 0

	for _, drive := range c.Drives {
		var mountPath string
		if drive.FinalMount.Name == "" {
			mountPath = drive.TemporaryMount.Path()
		} else {
			mountPath = drive.FinalMount.Path()
		}

		if drive.Broken {
			data[mountPath] = 1
			total++
		} else {
			data[mountPath] = 0
		}
	}
	data["drive_audit_errors"] = total
	jsonStr, _ := json.Marshal(data)

	path := "/var/cache/swift/drive.recon"
	if Config.ChrootPath != "" {
		path = filepath.Join(Config.ChrootPath, strings.TrimPrefix(path, "/"))
	}
	err := ioutil.WriteFile(path, jsonStr, 0644)
	if err != nil {
		Log(LogError, err.Error())
	}
}

//Handle implements the Event interface.
func (e DriveAddedEvent) Handle(c *Converger) {
	//default value for TemporaryMount.Name is md5sum of devicePath
	s := md5.Sum([]byte(e.DevicePath))
	deviceID := hex.EncodeToString(s[:])

	//- MappedDevicePath will be initialized by ScanOpenLUKSContainers() or OpenLUKS()
	//- MountPoint.Active will be initialized by ScanDriveMountPoints()
	//- FinalMount.Name will be initialized by ScanDriveSwiftIDs()
	drive := &Drive{
		DevicePath:       e.DevicePath,
		DeviceID:         deviceID,
		MappedDevicePath: "",
		TemporaryMount: MountPoint{
			Location: "/run/swift-storage",
			Name:     deviceID,
			Active:   false,
		},
		FinalMount: MountPoint{
			Location: "/srv/node",
			Name:     "",
			Active:   false,
		},
	}

	//check if the broken-flag is still present
	brokenFlagPath := drive.BrokenFlagPath()
	//make path relative to chroot dir (= cwd)
	brokenFlagPath = strings.TrimPrefix(brokenFlagPath, "/")
	_, err := os.Readlink(brokenFlagPath)
	switch {
	case err == nil:
		//link still exists, so device is broken
		Log(LogInfo, "%s was flagged as broken by a previous run of swift-drive-autopilot", drive.DevicePath)
		drive.MarkAsBroken() //this will re-print the log message explaining how to reinstate the drive into the cluster
	case os.IsNotExist(err):
		//ignore this error (no broken-flag means everything's okay)
	default:
		Log(LogError, err.Error())
	}

	c.Drives = append(c.Drives, drive)
	drive.Converge(c)
}

//Handle implements the Event interface.
func (e DriveRemovedEvent) Handle(c *Converger) {
	//do we know this drive?
	var drive *Drive
	var otherDrives []*Drive
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			drive = d
		} else {
			otherDrives = append(otherDrives, d)
		}
	}
	if drive == nil {
		return
	}

	//shutdown all active mounts
	//TODO: flag unmount to other containers
	if drive.FinalMount.Active {
		drive.FinalMount.Deactivate(drive.DevicePath)
		c.ActiveMounts.MarkAsDeactivated(drive.FinalMount.Path())
	}
	drive.TemporaryMount.Deactivate(drive.DevicePath)
	drive.CloseLUKS()

	//remove drive from list
	c.Drives = otherDrives
}

//Handle implements the Event interface.
func (e DriveErrorEvent) Handle(c *Converger) {
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			d.MarkAsBroken()
			return
		}
	}
}

//Handle implements the Event interface.
func (e DriveReinstatedEvent) Handle(c *Converger) {
	//do we know this drive?
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			d.Broken = false
			//reset the classification - who knows what was done to fix the drive
			d.Type = DeviceTypeNotScanned
			d.Converge(c)
			break
		}
	}

	//remove the unmount-propagation entry for this drive, if it still exists
	path := "run/swift-storage/state/unmount-propagation"
	_ = ForeachSymlinkIn(path, func(name, devicePath string) {
		if devicePath == e.DevicePath {
			err := os.Remove(path + "/" + name)
			if err != nil {
				Log(LogError, err.Error())
			}
		}
	})
}
