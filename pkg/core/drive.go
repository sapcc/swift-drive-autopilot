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
	"crypto/md5" //nolint:gosec // usage is not security related
	"encoding/hex"
	std_os "os"
	"strings"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
)

// NewDrive initializes a Drive instance.
func NewDrive(devicePath, serialNumber string, keys []string, osi os.Interface) *Drive {
	d := &Drive{
		DevicePath: devicePath,
		Device:     newDevice(devicePath, osi, len(keys) > 0),
		DriveID:    serialNumber,
		Keys:       keys,
	}

	//fallback value for DriveID is md5sum of devicePath
	if d.DriveID == "" {
		s := md5.Sum([]byte(devicePath)) //nolint:gosec // usage is for identification purposes and not security related
		d.DriveID = hex.EncodeToString(s[:])
		logg.Error(
			"cannot determine serial number for %s, will use device ID %s instead",
			devicePath, d.DriveID)
	}

	//detect unreadable device
	if d.Device == nil {
		d.Broken = true
	}

	//check if the broken-flag is still present
	for _, brokenFlagPath := range []string{d.TransientBrokenFlagPath(), d.DurableBrokenFlagPath()} {
		_, err := std_os.Readlink(strings.TrimPrefix(brokenFlagPath, "/"))
		switch {
		case err == nil:
			//link still exists, so device is broken
			logg.Info("%s was flagged as broken by a previous run of swift-drive-autopilot", d.DevicePath)
			d.MarkAsBroken(osi) //this will re-print the log message explaining how to reinstate the drive into the cluster
		case std_os.IsNotExist(err):
			//ignore this error (no broken-flag means everything's okay)
		default:
			logg.Error(err.Error())
		}
	}

	return d
}

// MountedPath returns the path where this drive is mounted right now.
func (d *Drive) MountedPath() string {
	if d.Device == nil {
		return ""
	}
	return d.Device.MountedPath()
}

// MountPath returns the path where this drive is supposed to be mounted.
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

// Converge moves the drive into its locally desired state.
//
// If the drive is not broken, its LUKS container (if any) will be created
// and/or opened, and its filesystem will be mounted. The only thing missing
// will be the final mount (since this step needs knowledge of all drives to
// check for swift-id collisions) and the swift-id auto-assignment.
//
// If the drive is broken (or discovered to be broken during this operation),
// any existing mappings or mounts will be teared down.
func (d *Drive) Converge(osi os.Interface) {
	if d.Broken {
		d.Device.Teardown(d, osi)
		return
	}

	ok := d.Device.Setup(d, osi)
	if !ok {
		d.MarkAsBroken(osi)
		d.Device.Teardown(d, osi)
		return
	}
}

// Teardown tears down all active mounts and mappings relating to this device.
func (d *Drive) Teardown(osi os.Interface) {
	if d.Device != nil {
		d.Device.Teardown(d, osi)
	}
}

// TransientBrokenFlagPath is the absolute path to a file that marks this drive
// as broken when it exists. The transient flag is created by
// swift-drive-autopilot upon encountering a disk error, and is not persisted
// across reboots.
func (d *Drive) TransientBrokenFlagPath() string {
	return "/run/swift-storage/broken/" + d.DriveID
}

// DurableBrokenFlagPath is the absolute path to a file that marks this drive
// as broken when it exists. The durable flag is created by an operator (by
// copying the transient flag file), in order to persist the brokenness across
// reboots.
func (d *Drive) DurableBrokenFlagPath() string {
	return "/var/lib/swift-storage/broken/" + d.DriveID
}

// MarkAsBroken sets the d.Broken flag.
func (d *Drive) MarkAsBroken(osi os.Interface) {
	d.Broken = true
	logg.Info("flagging %s as broken because of previous error", d.DevicePath)

	flagPath := d.TransientBrokenFlagPath()
	_, ok := command.Run("ln", "-sfT", d.DevicePath, flagPath)
	if ok {
		logg.Info("To reinstate this drive into the cluster, delete the symlink at " + flagPath)
	}

	//reset assignment (and thus require a re-reading of the swift-id file after the
	//drive is reinstated)
	d.Assignment = nil
}

// EligibleForAutoAssignment returns true if the drive does not have a swift-id
// yet, but is eligible for having one auto-assigned.
func (d *Drive) EligibleForAutoAssignment() bool {
	return !d.Broken && d.Assignment != nil && d.Assignment.Error == AssignmentPending
}
