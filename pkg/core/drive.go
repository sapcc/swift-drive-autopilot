/*******************************************************************************
*
* Copyright 2018 SAP SE
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

package core

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/sapcc/swift-drive-autopilot/pkg/cluster"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//NewDrive initializes a Drive instance.
func NewDrive(devicePath, serialNumber string, keys []string, osi os.Interface, cli cluster.Interface) *Drive {
	d := &Drive{
		DevicePath: devicePath,
		Device:     newDevice(devicePath, osi, len(keys) > 0),
		Status:     cluster.DriveReady, //but see below
		DriveID:    serialNumber,
		Keys:       keys,
	}

	//fallback value for DriveID is md5sum of devicePath
	if d.DriveID == "" {
		s := md5.Sum([]byte(devicePath))
		d.DriveID = hex.EncodeToString(s[:])
		util.LogError(
			"cannot determine serial number for %s, will use device ID %s instead",
			devicePath, d.DriveID)
	}

	//detect unreadable device
	if d.Device == nil {
		d.MarkAsBroken(cli)
	} else {
		//detect if this drive was declared broken earlier
		d.Status = cli.GetDriveStatus(d.DriveID)
	}

	return d
}

//MountedPath returns the path where this drive is mounted right now.
func (d *Drive) MountedPath() string {
	if d.Device == nil {
		return ""
	}
	return d.Device.MountedPath()
}

//MountPath returns the path where this drive is supposed to be mounted.
func (d *Drive) MountPath() string {
	path := d.Assignment.MountPath()
	if path == "" {
		//not assigned yet -> prefer path where drive is already mounted from an
		//earlier run of swift-drive-autopilot
		mountedPath := d.MountedPath()
		if mountedPath != "" {
			return mountedPath
		}
		return "/run/swift-storage/" + d.DriveID
	}
	return path
}

//Converge moves the drive into its locally desired state.
//
//If the drive is not broken, its LUKS container (if any) will be created
//and/or opened, and its filesystem will be mounted. The only thing missing
//will be the final mount (since this step needs knowledge of all drives to
//check for swift-id collisions) and the swift-id auto-assignment.
//
//If the drive is broken (or discovered to be broken during this operation),
//any existing mappings or mounts will be teared down.
func (d *Drive) Converge(osi os.Interface, cli cluster.Interface) {
	if d.Status != cluster.DriveReady {
		d.Device.Teardown(d, osi)
		return
	}

	ok := d.Device.Setup(d, osi)
	if !ok {
		d.MarkAsBroken(cli)
		d.Device.Teardown(d, osi)
		return
	}
}

//Teardown tears down all active mounts and mappings relating to this device.
func (d *Drive) Teardown(osi os.Interface) {
	if d.Device != nil {
		d.Device.Teardown(d, osi)
	}
}

//MarkAsBroken sets the drive status to cluster.DriveBroken.
func (d *Drive) MarkAsBroken(cli cluster.Interface) {
	d.Status = cluster.DriveBroken
	cli.SetDriveStatus(d.DriveID, d.Status, d.DevicePath)

	//reset assignment (and thus require a re-reading of the swift-id file after the
	//drive is reinstated)
	d.Assignment = nil
}

//EligibleForAutoAssignment returns true if the drive does not have a swift-id
//yet, but is eligible for having one auto-assigned.
func (d *Drive) EligibleForAutoAssignment() bool {
	return d.Status == cluster.DriveReady && d.Assignment != nil && d.Assignment.Error == AssignmentPending
}
