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
	"io/ioutil"
	std_os "os"
	"path/filepath"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//Converger contains the internal state of the converger thread.
type Converger struct {
	//long-lived state
	Drives []*Drive
	OS     os.Interface
}

//RunConverger runs the converger thread. This function does not return.
func RunConverger(queue chan []Event, osi os.Interface) {
	c := &Converger{OS: osi}

	for {
		//wait for processable events
		events := <-queue

		//initialize short-lived state for this event loop iteration
		osi.RefreshMountPoints()
		osi.RefreshLUKSMappings()

		for _, drive := range c.Drives {
			drive.Converged = false
		}

		//handle events
		for _, event := range events {
			if msg := event.LogMessage(); msg != "" {
				util.LogInfo("event received: " + msg)
			}
			eventCounter.With(prometheus.Labels{"type": event.EventType()}).Add(1)
			event.Handle(c)
		}

		c.Converge()
	}
}

//Converge moves towards the desired state of all drives after a set of events
//has been received and handled by the converger.
func (c *Converger) Converge() {
	for _, drive := range c.Drives {
		drive.Converge(c, c.OS)
	}

	//discover and auto-assign swift-ids of drives
	Drives(c.Drives).ScanSwiftIDs()
	c.AutoAssignSwiftIDs()

	for _, drive := range c.Drives {
		if !drive.Broken {
			if drive.FinalMount.Activate(drive.ActiveDevicePath(), c.OS) {
				drive.FinalMount.Chown(Config.Owner.User, Config.Owner.Group)
			}
			drive.CleanupDuplicateMounts(c.OS)
		}
	}

	c.CheckForUnexpectedMounts()
	c.WriteDriveAudit()

	//mark storage as ready for consumption by Swift
	command.Command{ExitOnError: true}.Run("touch", "/run/swift-storage/state/flag-ready")
}

//CheckForUnexpectedMounts prints error messages for every unexpected mount
//below /srv/node.
func (c *Converger) CheckForUnexpectedMounts() {
MOUNT:
	for _, mount := range c.OS.GetMountPointsIn("/srv/node") {
		for _, drive := range c.Drives {
			if drive.FinalMount.Active && drive.FinalMount.Path() == mount.MountPath {
				continue MOUNT
			}
		}

		util.LogError("unexpected mount at %s", mount.MountPath)
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
		util.LogError(err.Error())
	}
}

//Handle implements the Event interface.
func (e DriveAddedEvent) Handle(c *Converger) {
	deviceID := GetDeviceIDFor(e.DevicePath, e.SerialNumber)
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
	_, err := std_os.Readlink(brokenFlagPath)
	switch {
	case err == nil:
		//link still exists, so device is broken
		util.LogInfo("%s was flagged as broken by a previous run of swift-drive-autopilot", drive.DevicePath)
		drive.MarkAsBroken(c.OS) //this will re-print the log message explaining how to reinstate the drive into the cluster
	case std_os.IsNotExist(err):
		//ignore this error (no broken-flag means everything's okay)
	default:
		util.LogError(err.Error())
	}

	c.Drives = append(c.Drives, drive)
	drive.Converge(c, c.OS)
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
	if drive.FinalMount.Active {
		drive.FinalMount.Deactivate(drive.DevicePath, c.OS)
	}
	drive.TemporaryMount.Deactivate(drive.DevicePath, c.OS)
	drive.CloseLUKS(c.OS)

	//remove drive from list
	c.Drives = otherDrives
}

//Handle implements the Event interface.
func (e DriveErrorEvent) Handle(c *Converger) {
	for _, d := range c.Drives {
		if d.DevicePath == e.DevicePath {
			d.MarkAsBroken(c.OS)
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
			d.Type = nil
			d.Converge(c, c.OS)
			break
		}
	}

	//remove the unmount-propagation entry for this drive, if it still exists
	path := "run/swift-storage/state/unmount-propagation"
	_ = ForeachSymlinkIn(path, func(name, devicePath string) {
		if devicePath == e.DevicePath {
			err := std_os.Remove(path + "/" + name)
			if err != nil {
				util.LogError(err.Error())
			}
		}
	})
}
