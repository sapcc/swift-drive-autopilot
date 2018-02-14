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
	"regexp"
	"strings"

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
func (m *MountPoint) Check(devicePath string, activeMounts SystemMountPoints, allowDifferentBaseName bool) (success bool) {
	//check if there exists a mountpoint of the same device in the same location
	var actualMount *SystemMountPoint
	for _, am := range activeMounts {
		if am.DevicePath == devicePath && am.Location == m.Location {
			actualMount = am
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
		if actualMount.Name != m.Name {
			logLevel := util.LogError
			if allowDifferentBaseName {
				logLevel = util.LogInfo
			}
			logLevel(
				"expected %s to be mounted at %s, but is actually mounted at %s",
				devicePath, m.Path(), actualMount.Path(),
			)
			m.Name = actualMount.Name //to ensure that subsequent unmounting works correctly
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
		m.Name = actualMount.Name
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

//SystemMountPoint is an extension of MountPoint that reflects the state of an actual mount point as reported by mount().
type SystemMountPoint struct {
	MountPoint
	DevicePath string
	Options    map[string]bool
}

//SystemMountPoints is a list of SystemMountPoint with additional methods.
type SystemMountPoints []*SystemMountPoint

var mountPointRx = regexp.MustCompile(`^(/run/swift-storage|/srv/node)/([^/]+)$`)

//ScanMountPoints looks through the active mounts to check which drives are
//already mounted below /run/swift-storage or /srv/node.
func ScanMountPoints() SystemMountPoints {
	var result []*SystemMountPoint
	stdout, _ := command.Command{ExitOnError: true}.Run("mount")

	for _, line := range strings.Split(stdout, "\n") {
		//line looks like "<device> on <mountpoint> type <type> (<options>)"
		words := strings.Split(line, " ")
		if len(words) < 3 || words[1] != "on" {
			continue
		}
		devicePath, mountPath := words[0], words[2]

		//are we interested in this mountpoint?
		match := mountPointRx.FindStringSubmatch(mountPath)
		if match == nil {
			continue
		}

		//parse options into a set
		optionsStr := words[5]
		optionsStr = strings.TrimPrefix(optionsStr, "(")
		optionsStr = strings.TrimSuffix(optionsStr, ")")
		options := make(map[string]bool)
		for _, option := range strings.Split(optionsStr, ",") {
			options[option] = true
		}

		result = append(result, &SystemMountPoint{
			DevicePath: devicePath,
			MountPoint: MountPoint{
				Location: match[1],
				Name:     match[2],
			},
			Options: options,
		})
	}

	return SystemMountPoints(result)
}

//MarkAsDeactivated can be used to update a SystemMountPoints list when a
//mountpoint has been deactivated.
func (mounts SystemMountPoints) MarkAsDeactivated(mountPath string) {
	for _, m := range mounts {
		if m.Path() == mountPath {
			m.Active = false
		}
	}
}
