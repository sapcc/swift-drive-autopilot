// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	sys_os "os"
	"path/filepath"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
)

// XFSDevice is a device containing an XFS filesystem.
type XFSDevice struct {
	path      string
	formatted bool

	// internal state
	mountPath string
}

// DevicePath implements the Device interface.
func (d *XFSDevice) DevicePath() string {
	return d.path
}

// MountedPath implements the Device interface.
func (d *XFSDevice) MountedPath() string {
	return d.mountPath
}

// Setup implements the Device interface.
func (d *XFSDevice) Setup(drive *Drive, osi os.Interface) bool {
	// sanity check (and recognize pre-existing mount before attempting our own)
	err := d.Validate(drive, osi)
	if err != nil {
		logg.Error(err.Error())
		return false
	}

	// format on first use
	if !d.formatted {
		// double-check that disk is empty
		if osi.ClassifyDevice(d.path) != os.DeviceTypeUnknown {
			logg.Error("XFSDevice.Setup called on %s, but is not empty!", d.path)
			return false
		}

		ok := osi.FormatDevice(d.path)
		if ok {
			d.formatted = true
			logg.Debug("XFS filesystem created on %s", d.path)
		} else {
			return false
		}
	}

	// determine desired mount path
	mountPath := drive.MountPath()

	// tear down all mounts not matching the desired mount path (esp. the
	// temporary mount in /run when moving to the final mount in /srv/node)
	ok := os.ForeachMountScope(func(scope os.MountScope) bool {
		for _, m := range osi.GetMountPointsOf(d.path, scope) {
			if m.MountPath != mountPath {
				if !osi.UnmountDevice(m.MountPath, scope) {
					return false
				}
			}
		}
		return true
	})
	if !ok {
		return false
	}
	if d.mountPath != mountPath {
		d.mountPath = ""
	}

	// perform the mount
	ok = os.ForeachMountScope(func(scope os.MountScope) bool {
		return osi.MountDevice(d.path, mountPath, scope)
	})
	if ok {
		d.mountPath = mountPath
	} else {
		return false
	}

	// clear unmount-propagation flag if necessary (TODO swift.Interface)
	if filepath.Dir(mountPath) == "/srv/node" {
		err := sys_os.Remove(filepath.Join(
			"/run/swift-storage/state/unmount-propagation",
			filepath.Base(mountPath),
		))
		if err != nil && !sys_os.IsNotExist(err) {
			logg.Error(err.Error())
		}
	}

	return true
}

// Teardown implements the Device interface.
func (d *XFSDevice) Teardown(drive *Drive, osi os.Interface) bool {
	// remove all mounts of this device
	ok := os.ForeachMountScope(func(scope os.MountScope) bool {
		for _, m := range osi.GetMountPointsOf(d.path, scope) {
			if filepath.Dir(m.MountPath) == "/srv/node" {
				command.Run("ln", "-sTf", drive.DevicePath, "/run/swift-storage/state/unmount-propagation/"+filepath.Base(m.MountPath))
			}
			if !osi.UnmountDevice(m.MountPath, scope) {
				return false
			}
		}
		return true
	})

	if ok {
		d.mountPath = ""
	}
	return ok
}

// Validate implements the Device interface.
func (d *XFSDevice) Validate(drive *Drive, osi os.Interface) error {
	return os.ForeachMountScopeOrError(func(scope os.MountScope) error {
		mounts := osi.GetMountPointsOf(d.path, scope)

		if len(mounts) == 0 {
			if d.mountPath == "" {
				return nil
			}

			mountPath := d.mountPath
			d.mountPath = ""
			return fmt.Errorf(
				"expected %s to be mounted at %s, but is not mounted anymore in %s mount namespace",
				d.path, mountPath, scope,
			)
		}

		for _, m := range mounts {
			if m.Options["ro"] {
				return fmt.Errorf("mount of %s at %s is read-only in %s mount namespace (could be due to a disk error)", d.path, m.MountPath, scope)
			}

			// this case is okay - the XFSDevice struct may have just been created and
			// now we know that it is already active (and under which name)
			if d.mountPath == "" {
				logg.Info("discovered %s to be mounted at %s already in %s mount namespace", d.path, m.MountPath, scope)
				d.mountPath = m.MountPath
				continue
			}

			if m.MountPath != d.mountPath {
				return fmt.Errorf(
					"expected %s to be mounted at %s, but is actually mounted at %s in %s mount namespace",
					d.path, d.mountPath, m.MountPath, scope,
				)
			}
		}

		return nil
	})
}
