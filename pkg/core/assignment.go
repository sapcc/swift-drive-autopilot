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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/os"
)

// AssignmentError is a reason why a drive could not be assigned.
type AssignmentError string

const (
	//AssignmentMissing indicates a drive with missing SwiftID that is *not*
	//eligible for auto-assignment.
	AssignmentMissing AssignmentError = "no swift-id file found on device"
	//AssignmentPending indicates a drive with missing SwiftID that is eligible
	//for auto-assignment.
	AssignmentPending = "no swift-id file found on device, will try to assign one"
	//AssignmentBlocked indicates a drive with missing SwiftID that would be
	//eligible for auto-assignment, but auto-assignment is blocked by broken drives.
	AssignmentBlocked = "no swift-id file found on device, cannot auto-assign because of broken drives"
	//AssignmentDuplicate indicates a drive which a SwiftID that is also assigned
	//to another drive.
	AssignmentDuplicate = "found multiple drives with swift-id \"%s\" (not mounting any of them)"
	//AssignmentMismatch indicates a drive whose SwiftID differs from its
	//mountpoint below /srv/node.
	AssignmentMismatch = "mountpoint mismatches swift-id \"%s\""
)

// Assignment describes whether a drive is assigned an identity within Swift,
// and where it shall hence be mounted.
type Assignment struct {
	//SwiftID identifies the drive within the Swift ring.
	SwiftID string
	//If Error is not empty, the device shall not be mounted in /srv/node.
	Error AssignmentError
}

// Apply changes the assignment of this drive. If the assignment changes and
// the new assignment is invalid, the corresponding error message will be
// logged.
func (a Assignment) Apply(d *Drive) {
	prevErrorMsg := d.Assignment.ErrorMessage(d)
	currErrorMsg := a.ErrorMessage(d)
	if prevErrorMsg != currErrorMsg && currErrorMsg != "" {
		//AssignmentPending will be fixed shortly, so it's not really an error
		if a.Error == AssignmentPending {
			logg.Info(currErrorMsg)
		} else {
			logg.Error(currErrorMsg)
		}
	}

	d.Assignment = &a
}

// ErrorMessage returns an empty string if the assignment is valid, or an error
// message if the assignment is invalid.
func (a *Assignment) ErrorMessage(d *Drive) string {
	if a == nil || a.Error == "" {
		return ""
	}

	msg := string(a.Error)
	if strings.Contains(msg, "%s") {
		msg = fmt.Sprintf(msg, a.SwiftID)
	}
	mountedPath := d.MountedPath()
	if mountedPath == "" {
		return fmt.Sprintf("invalid assignment for %s: %s", d.DevicePath, msg)
	}
	return fmt.Sprintf("invalid assignment for %s (mounted at %s): %s", d.DevicePath, mountedPath, msg)
}

// MountPath returns the path where a disk with this assignment shall be mounted,
// or an empty string if this assignment does not allow mounting below /srv/node.
func (a *Assignment) MountPath() string {
	if a == nil || a.SwiftID == "spare" || a.Error != "" {
		return ""
	}
	return filepath.Join("/srv/node", a.SwiftID)
}

////////////////////////////////////////////////////////////////////////////////

type SwiftIDPools struct {
	Type          string
	Prefix        string
	Postfix       string
	Start         int
	End           int
	SpareInterval int
	SwiftIDPool   []string
}

// UpdateDriveAssignments scans all drives for their swift-id assignments, and
// auto-assigns swift-ids from the given pool if required and possible.
func UpdateDriveAssignments(drives []*Drive, swiftIDPool []string, osi os.Interface, swiftIDPools []SwiftIDPools) {
	//are there any broken drives?
	hasBrokenDrives := false
	for _, drive := range drives {
		if drive.Broken {
			hasBrokenDrives = true
			break
		}
	}

	//read existing swift-id assignments
	drivesBySwiftID := make(map[string]*Drive)
	hasMismountedDrives := false
	isAssignedSwiftID := make(map[string]bool)
	spareIdx := 0
	for _, drive := range drives {
		//ignore broken drives and keep going
		mountedPath := drive.MountedPath()
		if mountedPath == "" {
			continue
		}

		//read this device's swift-id
		swiftID, err := osi.ReadSwiftID(mountedPath)
		if err != nil {
			logg.Error(err.Error())
			continue
		} else if swiftID == "" {
			if len(swiftIDPool) > 0 {
				//mark this drive as eligible for automatic assignment during AutoAssignSwiftIDs()
				//BUT auto-assignment is only possible when no drives are broken (if a
				//drive is broken, we cannot look at its swift-id and thus cannot
				//ensure that we don't assign it to another drive)
				if hasBrokenDrives {
					Assignment{Error: AssignmentBlocked}.Apply(drive)
				} else {
					Assignment{Error: AssignmentPending}.Apply(drive)
				}
			} else {
				//auto-assignment not configured - operator has to enter a swift-id manually
				Assignment{Error: AssignmentMissing}.Apply(drive)
			}
			continue
		}

		//recognize spare disks
		if swiftID == "spare" {
			Assignment{SwiftID: "spare"}.Apply(drive)

			//count how many spare disks exist by giving them names like "spare/0", "spare/1", etc.
			//(this is the same format in which spare disks are presented in the Config.SwiftIDPool)
			name := fmt.Sprintf("spare/%d", spareIdx)
			isAssignedSwiftID[name] = true
			spareIdx++

			//skip collision check
			continue
		} else {
			isAssignedSwiftID[swiftID] = true
		}

		//does this swift-id conflict with where the device is currently mounted?
		if filepath.Dir(mountedPath) == "/srv/node" && filepath.Base(mountedPath) != swiftID {
			Assignment{SwiftID: swiftID, Error: AssignmentMismatch}.Apply(drive)
			hasMismountedDrives = true //something is seriously wrong - inhibit automatic assignment
		} else {
			Assignment{SwiftID: swiftID}.Apply(drive)
		}

		//is this the first device with this swift-id?
		otherDrive, exists := drivesBySwiftID[swiftID]
		if exists {
			//no - skip these drives during the final mount
			Assignment{SwiftID: swiftID, Error: AssignmentDuplicate}.Apply(drive)
			Assignment{SwiftID: swiftID, Error: AssignmentDuplicate}.Apply(otherDrive)
		} else {
			//yes - remember it to check for collisions later on
			drivesBySwiftID[swiftID] = drive
		}
	}

	//can we perform auto-assignment?
	if hasBrokenDrives || hasMismountedDrives || len(swiftIDPool) == 0 {
		return
	}

	//perform auto-assignment
	for _, drive := range drives {
		if drive.EligibleForAutoAssignment() {
			//try to find an unused swift-id
			//
			//WARNING: IDs are GUARANTEED by our interface contract to be assigned
			//in the order in which they appear in the configuration (see docs for
			//`swift-id-pool` in README).
			var poolID string
			for _, id := range swiftIDPool {
				if !isAssignedSwiftID[id] {
					poolID = id
					break
				}
			}

			if poolID == "" {
				//TODO: This may get spammy since it is printed during each converger pass.
				logg.Error("tried to assign swift-id to %s, but pool is exhausted", drive.DevicePath)
				continue
			}

			swiftID := poolID
			if strings.HasPrefix(poolID, "spare/") {
				swiftID = "spare"
			}

			logg.Info("assigning swift-id '%s' to %s", swiftID, drive.DevicePath)
			err := osi.WriteSwiftID(drive.MountPath(), swiftID)
			if err != nil {
				logg.Error(err.Error())
				continue
			}

			isAssignedSwiftID[poolID] = true
			Assignment{SwiftID: swiftID}.Apply(drive)
		}
	}
}
