// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package core

import "github.com/sapcc/swift-drive-autopilot/pkg/os"

// Device is implemented by each model class that represents the contents of a
// device. Each method in the interface takes a reference to the drive that
// contains this device, and to the os.Interface with which to execute OS-level
// operations.
type Device interface {
	// DevicePath returns the path to the device file.
	DevicePath() string
	// MountedPath returns the path where this device (or its contents) are mounted.
	MountedPath() string
	// Setup is called by the converger when the drive is not broken. It shall
	// idempotently prepare the drive for consumption by Swift.
	Setup(drive *Drive, osi os.Interface) (ok bool)
	// Teardown is called by the converger when the drive is broken. It shall
	// idempotently shutdown all mounts and mappings for this drive.
	Teardown(drive *Drive, osi os.Interface) (ok bool)
	// Validate is called by the converger when the drive is not broken, to
	// determine whether it has become broken.
	Validate(drive *Drive, osi os.Interface) error
}

// Returns nil to indicate unreadable device.
func newDevice(devicePath string, osi os.Interface, preferLUKS bool) Device {
	switch osi.ClassifyDevice(devicePath) {
	case os.DeviceTypeUnreadable:
		break
	case os.DeviceTypeUnknown:
		if preferLUKS {
			return &LUKSDevice{path: devicePath, formatted: false}
		}
		return &XFSDevice{path: devicePath, formatted: false}
	case os.DeviceTypeLUKS:
		return &LUKSDevice{path: devicePath, formatted: true}
	case os.DeviceTypeFilesystem:
		return &XFSDevice{path: devicePath, formatted: true}
	}
	return nil
}

// Drive enhances os.Drive with a state machine that coordinates the setup and
// teardown of the drive's mount.
type Drive struct {
	DevicePath string
	Device     Device

	// state machine
	Broken bool

	// DriveID identifies this drive in derived filenames.
	DriveID string
	// Assignment identifies this drive's location within the Swift ring.
	Assignment *Assignment
	// Keys contains the LUKS encryption keys that may be used with this drive. When
	// creating a new LUKS container on this drive, Keys[0] must be used. An empty
	// slice indicates that encryption is not configured.
	Keys []string
}
