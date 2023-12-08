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
	"encoding/json"
	sys_os "os"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/core"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
)

// Converger contains the internal state of the converger thread.
type Converger struct {
	//long-lived state
	Drives []*core.Drive
	OS     os.Interface
}

// RunConverger runs the converger thread. This function does not return.
func RunConverger(queue chan []Event, osi os.Interface) {
	c := &Converger{OS: osi}

	for {
		//wait for processable events
		events := <-queue

		//initialize short-lived state for this event loop iteration
		osi.RefreshMountPoints()
		osi.RefreshLUKSMappings()

		//handle events
		for _, event := range events {
			if msg := event.LogMessage(); msg != "" {
				logg.Info("event received: " + msg)
			}
			eventCounter.With(prometheus.Labels{"type": event.EventType()}).Add(1)
			event.Handle(c)
		}

		c.Converge()
	}
}

// Converge moves towards the desired state of all drives after a set of events
// has been received and handled by the converger.
func (c *Converger) Converge() {
	for _, drive := range c.Drives {
		drive.Converge(c.OS)
	}

	core.UpdateDriveAssignments(c.Drives, Config.SwiftIDPool, c.OS)

	for _, drive := range c.Drives {
		if !drive.Broken {
			drive.Converge(c.OS) //to reflect updated drive assignments
			mountPath := drive.MountPath()
			if filepath.Dir(mountPath) == "/srv/node" {
				c.OS.Chown(mountPath, Config.Owner.User, Config.Owner.Group)
			}
		}
	}

	c.CheckForUnexpectedMounts()
	c.WriteDriveAudit()

	//mark storage as ready for consumption by Swift
	command.Command{ExitOnError: true}.Run("touch", "/run/swift-storage/state/flag-ready")
}

// CheckForUnexpectedMounts prints error messages for every unexpected mount
// below /srv/node.
func (c *Converger) CheckForUnexpectedMounts() {
MOUNT:
	for _, mount := range c.OS.GetMountPointsIn("/srv/node", os.HostScope) {
		for _, drive := range c.Drives {
			if drive.MountPath() == mount.MountPath {
				continue MOUNT
			}
		}

		logg.Error("unexpected mount at %s", mount.MountPath)
	}
}

// WriteDriveAudit writes /var/cache/swift/drive.recon in the same format as
// emitted by swift-drive-audit.
func (c *Converger) WriteDriveAudit() {
	data := make(map[string]int)
	total := 0

	for _, drive := range c.Drives {
		mountPath := drive.MountPath()
		if drive.Broken {
			data[mountPath] = 1
			total++
		} else {
			data[mountPath] = 0
		}
	}
	data["drive_audit_errors"] = total
	jsonStr, err := json.Marshal(data)
	if err != nil {
		logg.Error(err.Error())
	}

	path := "/var/cache/swift/drive.recon"
	if Config.ChrootPath != "" {
		path = filepath.Join(Config.ChrootPath, strings.TrimPrefix(path, "/"))
	}
	err = sys_os.WriteFile(path, jsonStr, 0644)
	if err != nil {
		logg.Error(err.Error())
	}
}

// Handle implements the Event interface.
func (e DriveAddedEvent) Handle(c *Converger) {
	keys := make([]string, len(Config.Keys))
	for idx, key := range Config.Keys {
		keys[idx] = string(key.Secret)
	}

	drive := core.NewDrive(e.DevicePath, e.SerialNumber, e.Vendor, e.RotationRate, e.Type, keys, c.OS)
	c.Drives = append(c.Drives, drive)
	drive.Converge(c.OS)
}

// Handle implements the Event interface.
func (e DriveRemovedEvent) Handle(c *Converger) {
	//do we know this drive?
	var drive *core.Drive
	var otherDrives []*core.Drive
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

	//remove drive
	drive.Teardown(c.OS)
	c.Drives = otherDrives
}

// Handle implements the Event interface.
func (e DriveErrorEvent) Handle(c *Converger) {
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			d.MarkAsBroken(c.OS)
			return
		}
	}
}

// Handle implements the Event interface.
func (e DriveReinstatedEvent) Handle(c *Converger) {
	for idx, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			//reset the drive to pristine condition
			d = core.NewDrive(d.DevicePath, d.DriveID, d.Vendor, d.RotationRate, d.DriveType, d.Keys, c.OS)
			c.Drives[idx] = d
			d.Converge(c.OS)
			break
		}
	}
}
