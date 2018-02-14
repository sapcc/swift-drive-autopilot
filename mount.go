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
	"path/filepath"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//MountPoint describes a location where a Drive can be mounted.
type MountPoint struct {
	//Location is the dirname(1) of the mountpoint. It is an absolute path
	//referring to a location in the chroot (if any).
	Location string
	//Name is the basename(1) of the mountpoint.
	Name string
	//Active is true when the device is mounted at this path.
	Active bool
}

//Path returns the full absolute path (inside the chroot) of the mountpoint.
func (m MountPoint) Path() string {
	return m.Location + "/" + m.Name
}

//Check takes the actual mount name of this device below the mountpoint's
//Location (or an empty string if the mount is not active), and checks whether
//this is consistent with the internal state of the MountPoint struct.
func (m *MountPoint) Check(devicePath string, osi os.Interface, allowDifferentBaseName bool) (success bool) {
	//check if there exists a mountpoint of the same device in the same location
	var actualMount *os.MountPoint
	for _, am := range osi.GetMountPointsOf(devicePath) {
		if filepath.Dir(am.MountPath) == m.Location {
			actualMount = &am
			break
		}
	}

	//no such mount - we're fine if this MountPoint is supposed to be inactive
	if actualMount == nil {
		if !m.Active {
			return true
		}
		util.LogError(
			"expected %s to be mounted at %s, but is not mounted anymore",
			devicePath, m.Path(),
		)
		m.Active = false
		return false
	}

	if m.Active {
		actualMountName := filepath.Base(actualMount.MountPath)
		if actualMountName != m.Name {
			logLevel := util.LogError
			if allowDifferentBaseName {
				logLevel = util.LogInfo
			}
			logLevel(
				"expected %s to be mounted at %s, but is actually mounted at %s",
				devicePath, m.Path(), actualMount.MountPath,
			)
			m.Name = actualMountName //to ensure that subsequent unmounting works correctly
			if !allowDifferentBaseName {
				return false
			}
		}
		if actualMount.Options["ro"] {
			util.LogError("mount of %s at %s is read-only (could be due to a disk error)", devicePath, m.Path())
			return false
		}
	} else {
		//this case is okay - the MountPoint struct may have just been created
		//and now we know that it is already active (and under which name)
		m.Name = filepath.Base(actualMount.MountPath)
		m.Active = true
		util.LogInfo("discovered %s to be mounted at %s already", devicePath, m.Path())
	}
	return true
}

//Activate will mount the given device to this MountPoint if the MountPoint is
//not yet Active.
func (m *MountPoint) Activate(devicePath string, osi os.Interface) bool {
	//ready to be mounted?
	if m.Location == "" || m.Name == "" {
		return false
	}
	//already mounted?
	if m.Active {
		return true
	}

	mountPath := m.Path()
	ok := osi.MountDevice(devicePath, mountPath, Config.ChrootPath != "")
	if !ok {
		return false
	}

	m.Active = true
	util.LogInfo("mounted %s to %s", devicePath, mountPath)
	return true
}

//Deactivate will unmount the given MountPoint if it is Active.
func (m *MountPoint) Deactivate(devicePath string, osi os.Interface) {
	//already unmounted?
	if !m.Active {
		return
	}

	mountPath := m.Path()
	osi.UnmountDevice(mountPath, Config.ChrootPath != "")

	m.Active = false
	util.LogInfo("unmounted %s", mountPath)

	if m.Location == "/srv/node" {
		command.Run("ln", "-sTf", devicePath, "/run/swift-storage/state/unmount-propagation/"+m.Name)
	}
}

//Chown changes the ownership of the mount point path to the given user and
//group. Both arguments may either be a name or a numeric ID (but still given
//as a string in decimal).
func (m MountPoint) Chown(user, group string) {
	mountPath := m.Path()
	Chown(mountPath, user, group)
}
