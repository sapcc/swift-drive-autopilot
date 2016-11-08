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
func (m *MountPoint) Check(devicePath, actualMountName string) (success bool) {
	if actualMountName == "" {
		if !m.Active {
			return true
		}
		Log(LogError,
			"expected %s to be mounted at %s, but is not mounted anymore",
			devicePath, m.Path(),
		)
		return false
	}

	if m.Active {
		if actualMountName != m.Name {
			Log(LogError,
				"expected %s to be mounted at %s, but is actually mounted at /run/swift-storage/%s",
				devicePath, m.Path(), actualMountName,
			)
			return false
		}
	} else {
		//this case is okay - the MountPoint struct may have just been created
		//and now we know that it is already active (and under which name)
		m.Name = actualMountName
		m.Active = true
		Log(LogDebug, "discovered %s to be mounted at %s/%s already", m.Location, m.Active)
	}
	return true
}

//Activate will mount the given device to this MountPoint if the MountPoint is
//not yet Active.
func (m *MountPoint) Activate(devicePath string) bool {
	//ready to be mounted?
	if m.Location == "" || m.Name == "" {
		return false
	}
	//already mounted?
	if m.Active {
		return true
	}

	mountPath := m.Path()

	//prepare new target directory
	_, ok := Run("mkdir", "-m", "0700", "-p", mountPath)
	if !ok {
		return false
	}

	//for the mount to appear both in the container and the host, it has to be
	//performed twice, once for each mount namespace involved
	_, ok = Run("mount", devicePath, mountPath)
	if !ok {
		return false
	}
	if Config.ChrootPath != "" {
		_, ok = Command{NoNsenter: true}.Run("mount", devicePath, mountPath)
		if !ok {
			return false
		}
	}

	m.Active = true
	Log(LogInfo, "mounted %s to %s", devicePath, mountPath)
	return true
}

//Deactivate will unmount the given MountPoint if it is Active.
func (m *MountPoint) Deactivate() {
	//already unmounted?
	if !m.Active {
		return
	}

	mountPath := m.Path()

	//unmount both in the container and the host (same pattern as for Activate)
	Run("umount", mountPath)
	if Config.ChrootPath != "" {
		Command{NoNsenter: true}.Run("umount", mountPath)
	}

	m.Active = false
	Log(LogInfo, "unmounted %s", mountPath)
}

//Chown changes the ownership of the mount point path to the given user and
//group. Both arguments may either be a name or a numeric ID (but still given
//as a string in decimal).
func (m MountPoint) Chown(user, group string) {
	var (
		command string
		arg     string
	)

	//set only those things which were given
	if user == "" {
		if group == "" {
			return // nothing to do
		}
		command, arg = "chgrp", group
	} else {
		command, arg = "chown", user
		if group != "" {
			arg += ":" + group
		}
	}

	mountPath := m.Path()
	Log(LogDebug, "%s %s to %s", command, mountPath, arg)
	Run(command, arg, mountPath)
}

var tempMountRx = regexp.MustCompile(`^/run/swift-storage/([^/]+)$`)
var finalMountRx = regexp.MustCompile(`^/srv/node/([^/]+)$`)

//ScanMountPoints looks through the active mounts to check which drives are
//already mounted below /run/swift-storage or /srv/node.
func ScanMountPoints() (temporaryMounts, finalMounts map[string]string) {
	temporaryMounts = make(map[string]string)
	finalMounts = make(map[string]string)

	stdout, _ := Command{ExitOnError: true}.Run("mount")

	for _, line := range strings.Split(stdout, "\n") {
		//line looks like "<device> on <mountpoint> type <type> (<options>)"
		words := strings.Split(line, " ")
		if len(words) < 3 || words[1] != "on" {
			continue
		}
		devicePath, mountPath := words[0], words[2]

		match := tempMountRx.FindStringSubmatch(mountPath)
		if match != nil {
			temporaryMounts[devicePath] = match[1]
			continue
		}
		match = finalMountRx.FindStringSubmatch(mountPath)
		if match != nil {
			finalMounts[devicePath] = match[1]
			continue
		}
	}

	return
}
