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
	"os"
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
		c.ActiveMounts = ScanMountPoints()
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

	//map mountpoints from /run/swift-storage to /srv/node
	Drives(c.Drives).ScanSwiftIDs()

	for _, drive := range c.Drives {
		if !drive.Broken {
			if drive.FinalMount.Activate(drive.ActiveDevicePath()) {
				drive.FinalMount.Chown(Config.Owner.User, Config.Owner.Group)
			}
			drive.CleanupDuplicateMounts()
		}
	}

	//mark storage as ready for consumption by Swift
	Command{ExitOnError: true}.Run("mkdir", "-p", "/run/swift-storage/state")
	Command{ExitOnError: true}.Run("touch", "/run/swift-storage/state/flag-ready")
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
	drive.FinalMount.Deactivate()
	drive.TemporaryMount.Deactivate()
	drive.CloseLUKS()

	//remove drive from list
	c.Drives = otherDrives
}

//Handle implements the Event interface.
func (e DriveReinstatedEvent) Handle(c *Converger) {
	//do we know this drive?
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			d.Broken = false
			d.Converge(c)
			return
		}
	}
}
