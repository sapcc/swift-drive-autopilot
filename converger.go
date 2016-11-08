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
)

//Converger contains the internal state of the converger thread.
type Converger struct {
	//long-lived state
	Drives []*Drive

	//short-lived state that is gathered before the event handlers run
	ActiveLUKSMappings    map[string]string
	ActiveTemporaryMounts map[string]string
	ActiveFinalMounts     map[string]string

	//When Failed becomes true, the application will shutdown after the next
	//Converge() run.  This is intended to signal failures of critical
	//operations (like cryptsetup/mount/umount) to the caller, without
	//interrupting the setup of other drives.
	Failed bool
}

//RunConverger runs the converger thread. This function does not return.
func RunConverger(queue chan []Event) {
	c := &Converger{}

	for {
		//wait for processable events
		events := <-queue

		//initialize short-lived state for this event loop iteration
		c.ActiveLUKSMappings = ScanLUKSMappings()
		c.ActiveTemporaryMounts, c.ActiveFinalMounts = ScanMountPoints()

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
		if !drive.CheckLUKS(c.ActiveLUKSMappings) {
			c.Failed = true
			continue //but keep going for the drives that work
		}
		if len(Config.Keys) > 0 {
			//create LUKS containers on unformatted drives
			if !drive.FormatLUKSIfRequired() {
				c.Failed = true //but keep going for the drives that work
				continue
			}
			//open LUKS containers if required
			if !drive.OpenLUKS() {
				c.Failed = true //but keep going for the drives that work
				continue
			}
		}
		//try to mount all drives to /run/swift-storage (if not yet mounted)
		if !drive.CheckMounts(c.ActiveTemporaryMounts, c.ActiveFinalMounts) {
			c.Failed = true //but keep going for the drives that work
			continue
		}
		if !drive.EnsureFilesystem() {
			c.Failed = true //but keep going for the drives that work
			continue
		}
		if !drive.MountSomewhere() {
			c.Failed = true //but keep going for the drives that work
			continue
		}
	}

	//map mountpoints from /run/swift-storage to /srv/node
	if !Drives(c.Drives).ScanSwiftIDs() {
		c.Failed = true //but keep going for the drives that work
	}

	for _, drive := range c.Drives {
		if drive.FinalMount.Activate(drive.ActiveDevicePath()) {
			Log(LogInfo, "%s is mounted on %s", drive.DevicePath, drive.FinalMount.Path())
		} else {
			c.Failed = true //but keep going for the drives that work
			continue
		}

		owner := Config.Owner
		if !drive.FinalMount.Chown(owner.User, owner.Group) {
			c.Failed = true
		}
	}

	//mark storage as ready for consumption by Swift
	_, ok := Run("mkdir", "-p", "/run/swift-storage/state")
	if !ok {
		c.Failed = true
	}
	_, ok = Run("touch", "/run/swift-storage/state/flag-ready")
	if !ok {
		c.Failed = true
	}

	//signal failure to the caller by terminating the process
	if c.Failed {
		Log(LogInfo, "encountered some errors, see above")
		os.Exit(1)
	}
}

//Handle implements the Event interface.
func (e DriveAddedEvent) Handle(c *Converger) {
	//default value for MountID is md5sum of devicePath
	s := md5.Sum([]byte(e.DevicePath))
	mountID := hex.EncodeToString(s[:])

	//- MappedDevicePath will be initialized by ScanOpenLUKSContainers() or OpenLUKS()
	//- MountPoint.Active will be initialized by ScanDriveMountPoints()
	//- FinalMount.Name will be initialized by ScanDriveSwiftIDs()
	drive := &Drive{
		DevicePath:       e.DevicePath,
		MappedDevicePath: "",
		TemporaryMount: MountPoint{
			Location: "/run/swift-storage",
			Name:     mountID,
			Active:   false,
		},
		FinalMount: MountPoint{
			Location: "/srv/node",
			Name:     "",
			Active:   false,
		},
	}

	c.Drives = append(c.Drives, drive)
}

//Handle implements the Event interface.
func (e DriveRemovedEvent) Handle(c *Converger) {
	//do we know this drive?
	var drive *Drive
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			drive = d
			break
		}
	}
	if drive == nil {
		return
	}

	//shutdown all active mounts
	//TODO: flag unmount to other containers
	if !drive.FinalMount.Deactivate() {
		c.Failed = true
		return
	}
	if !drive.TemporaryMount.Deactivate() {
		c.Failed = true
		return
	}
	if !drive.CloseLUKS() {
		c.Failed = true
		return
	}
}
