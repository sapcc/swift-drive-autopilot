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

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/os"
)

// LUKSDevice is a device containing a LUKS container.
type LUKSDevice struct {
	path      string
	formatted bool

	// internal state
	mapped      Device
	mappingName string
}

// DevicePath implements the Device interface.
func (d *LUKSDevice) DevicePath() string {
	return d.path
}

// MountedPath implements the Device interface.
func (d *LUKSDevice) MountedPath() string {
	if d.mapped == nil {
		return ""
	}
	return d.mapped.MountedPath()
}

// Setup implements the Device interface.
func (d *LUKSDevice) Setup(drive *Drive, osi os.Interface) bool {
	// sanity check (and recognize pre-existing mapping before attempting our own)
	err := d.Validate(drive, osi)
	if err != nil {
		logg.Error(err.Error())
		return false
	}
	if len(drive.Keys) == 0 {
		logg.Error("LUKSDevice.Setup called on %s, but no keys specified!", d.path)
		return false
	}

	// format on first use
	if !d.formatted {
		// double-check that disk is empty
		if osi.ClassifyDevice(d.path) != os.DeviceTypeUnknown {
			logg.Error("LUKSDevice.Setup called on %s, but is not empty!", d.path)
			return false
		}

		// format with the preferred key
		ok := osi.CreateLUKSContainer(d.path, drive.Keys[0])
		if ok {
			d.formatted = true
		} else {
			return false
		}
	}

	// decrypt if necessary
	if d.mapped == nil {
		mappedDevicePath, ok := osi.OpenLUKSContainer(d.path, drive.DriveID, drive.Keys)
		if ok {
			logg.Info("LUKS container at %s opened as %s", d.path, mappedDevicePath)
			d.mapped = newDevice(mappedDevicePath, osi, false)
			d.mappingName = drive.DriveID
		} else {
			logg.Error(
				"exec(cryptsetup luksOpen %s %s) failed: none of the configured keys was accepted",
				d.path, drive.DriveID,
			)
			return false
		}
	}

	// did that work?
	if d.mapped == nil {
		return false
	}

	// descend into decrypted drive
	return d.mapped.Setup(drive, osi)
}

// Teardown implements the Device interface.
func (d *LUKSDevice) Teardown(drive *Drive, osi os.Interface) bool {
	// need to teardown contents of mapped device first
	if d.mapped != nil {
		ok := d.mapped.Teardown(drive, osi)
		if ok {
			d.mapped = nil
		} else {
			return false
		}
	}

	// unmap container if necessary
	if d.mappingName != "" {
		ok := osi.CloseLUKSContainer(d.mappingName)
		if ok {
			logg.Info("LUKS container /dev/mapper/%s closed", d.mappingName)
			d.mappingName = ""
		} else {
			return false
		}
	}

	return true
}

// Validate implements the Device interface.
func (d *LUKSDevice) Validate(drive *Drive, osi os.Interface) error {
	mappedDevicePath := osi.GetLUKSMappingOf(d.path)

	switch {
	case mappedDevicePath == "":
		if d.mapped != nil {
			return fmt.Errorf("LUKS container in %s should be open at %s, but is not",
				d.path, d.mapped.DevicePath(),
			)
		}
	case d.mapped == nil:
		// existing mapping is now discovered for the first time -> update ourselves
		logg.Info("discovered %s to be mapped to %s already", d.path, mappedDevicePath)
		d.mapped = newDevice(mappedDevicePath, osi, false)
	case mappedDevicePath != d.mapped.DevicePath():
		// our internal state tells a different story!
		return fmt.Errorf("LUKS container in %s should be open at %s, but is actually open at %s",
			d.path, d.mapped.DevicePath(), mappedDevicePath,
		)
	}

	// a device containing a LUKS container should not be mounted itself
	err := os.ForeachMountScopeOrError(func(scope os.MountScope) error {
		if len(osi.GetMountPointsOf(d.path, scope)) > 0 {
			return fmt.Errorf("%s contains an open LUKS container, but is also mounted directly in %s mount namespace", d.path, scope)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// LUKS mapping is looking good -> drill down into the mapped device
	if d.mapped == nil {
		return nil
	}
	return d.mapped.Validate(drive, osi)
}
