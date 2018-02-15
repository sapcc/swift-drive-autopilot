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
	sys_os "os"
	"path/filepath"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//XFSDevice is a device containing an XFS filesystem.
type XFSDevice struct {
	path      string
	formatted bool

	//internal state
	mountPath string
}

//DevicePath implements the Device interface.
func (d *XFSDevice) DevicePath() string {
	return d.path
}

//MountedPath implements the Device interface.
func (d *XFSDevice) MountedPath() string {
	return d.mountPath
}

//Setup implements the Device interface.
func (d *XFSDevice) Setup(drive *Drive, osi os.Interface) bool {
	//sanity check (and recognize pre-existing mount before attempting our own)
	err := d.Validate(drive, osi)
	if err != nil {
		util.LogError(err.Error())
		return false
	}

	//format on first use
	if !d.formatted {
		//double-check that disk is empty
		if osi.ClassifyDevice(d.path) != os.DeviceTypeUnknown {
			util.LogError("XFSDevice.Setup called on %s, but is not empty!", d.path)
			return false
		}

		ok := osi.FormatDevice(d.path)
		if ok {
			d.formatted = true
			util.LogDebug("XFS filesystem created on %s", d.path)
		} else {
			return false
		}
	}

	//determine desired mount path
	mountPath := drive.MountPath()
	if d.mountPath == mountPath {
		//nothing to do
		return true
	}

	//when moving to the final mount in /srv/node, tear down the temporary mount in /run first
	if d.mountPath != "" {
		ok := osi.UnmountDevice(d.mountPath)
		if ok {
			util.LogInfo("unmounted %s", d.mountPath)
			d.mountPath = ""
		} else {
			return false
		}
	}

	//perform the mount
	ok := osi.MountDevice(d.path, mountPath)
	if ok {
		util.LogInfo("mounted %s to %s", d.path, mountPath)
		d.mountPath = mountPath
	} else {
		return false
	}

	//clear unmount-propagation flag if necessary (TODO swift.Interface)
	if filepath.Dir(d.mountPath) == "/srv/node" {
		err := sys_os.Remove(filepath.Join(
			"/run/swift-storage/state/unmount-propagation",
			filepath.Base(d.mountPath),
		))
		if err != nil && !sys_os.IsNotExist(err) {
			util.LogError(err.Error())
		}
	}

	return true
}

//Teardown implements the Device interface.
func (d *XFSDevice) Teardown(drive *Drive, osi os.Interface) bool {
	if d.mountPath == "" {
		//nothing to do
		return true
	}

	//set unmount-propagation flag if necessary (TODO swift.Interface)
	if filepath.Dir(d.mountPath) == "/srv/node" {
		flagPath := "/run/swift-storage/state/unmount-propagation/" + filepath.Base(d.mountPath)
		command.Run("ln", "-sTf", drive.DevicePath, flagPath)
	}

	//remove the mount that we know about
	ok := osi.UnmountDevice(d.mountPath)
	if ok {
		util.LogInfo("unmounted %s", d.mountPath)
		d.mountPath = ""
	} else {
		return false
	}

	//remove any other mounts that the system knows about
	for _, m := range osi.GetMountPointsOf(d.path) {
		if filepath.Dir(m.MountPath) == "/srv/node" {
			command.Run("ln", "-sTf", d.path, "/run/swift-storage/state/unmount-propagation/"+filepath.Base(m.MountPath))
		}
		ok := osi.UnmountDevice(m.MountPath)
		if ok {
			util.LogInfo("unmounted %s", m.MountPath)
		} else {
			return false
		}
	}

	return true
}

//Validate implements the Device interface.
func (d *XFSDevice) Validate(drive *Drive, osi os.Interface) error {
	mounts := osi.GetMountPointsOf(d.path)

	if len(mounts) == 0 {
		if d.mountPath == "" {
			return nil
		}

		mountPath := d.mountPath
		d.mountPath = ""
		return fmt.Errorf(
			"expected %s to be mounted at %s, but is not mounted anymore",
			d.path, mountPath,
		)
	}

	for _, m := range mounts {
		if m.Options["ro"] {
			return fmt.Errorf("mount of %s at %s is read-only (could be due to a disk error)", d.path, m.MountPath)
		}

		//this case is okay - the XFSDevice struct may have just been created and
		//now we know that it is already active (and under which name)
		if d.mountPath == "" {
			util.LogInfo("discovered %s to be mounted at %s already", d.path, m.MountPath)
			d.mountPath = m.MountPath
			continue
		}

		if m.MountPath != d.mountPath {
			return fmt.Errorf(
				"expected %s to be mounted at %s, but is actually mounted at %s",
				d.path, d.mountPath, m.MountPath,
			)
		}
	}

	return nil
}
