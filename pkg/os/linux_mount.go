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

package os

import (
	sys_os "os"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

func repeatInOwnNamespace() bool {
	cwd, _ := sys_os.Getwd()
	return cwd != "/"
}

//MountDevice implements the Interface interface.
func (l *Linux) MountDevice(devicePath, mountPath string) bool {
	//prepare target directory
	_, ok := command.Run("mkdir", "-m", "0700", "-p", mountPath)
	if !ok {
		return false
	}

	//for the mount to appear both in the container and the host, it has to be
	//performed twice, once for each mount namespace involved
	_, ok = command.Run("mount", devicePath, mountPath)
	if ok && repeatInOwnNamespace() {
		_, ok = command.Command{NoNsenter: true}.Run("mount", devicePath, mountPath)
	}

	//record the new mount
	if ok {
		l.ActiveMountPoints = append(l.ActiveMountPoints, MountPoint{
			DevicePath: devicePath,
			MountPath:  mountPath,
		})
	}

	return ok
}

//UnmountDevice implements the Interface interface.
func (l *Linux) UnmountDevice(mountPath string) bool {
	//unmount both in the container and the host (same pattern as for Activate)
	_, ok := command.Run("umount", mountPath)
	if ok && repeatInOwnNamespace() {
		_, ok = command.Command{NoNsenter: true}.Run("umount", mountPath)
	}

	//record that the unmount happened
	if ok {
		idxToRemove := -1
		for idx, m := range l.ActiveMountPoints {
			if m.MountPath == mountPath {
				idxToRemove = idx
				break
			}
		}
		if idxToRemove >= 0 {
			l.ActiveMountPoints = append(
				l.ActiveMountPoints[:idxToRemove],
				l.ActiveMountPoints[idxToRemove+1:]...,
			)
		}
	}

	return ok
}

//RefreshMountPoints implements the Interface interface.
func (l *Linux) RefreshMountPoints() {
	l.ActiveMountPoints = nil
	stdout, _ := command.Command{ExitOnError: true}.Run("mount")

	for _, line := range strings.Split(stdout, "\n") {
		//line looks like "<device> on <mountpoint> type <type> (<options>)"
		words := strings.Split(line, " ")
		if len(words) < 3 || words[1] != "on" {
			continue
		}
		devicePath, mountPath := words[0], words[2]

		//parse options into a set
		optionsStr := words[5]
		optionsStr = strings.TrimPrefix(optionsStr, "(")
		optionsStr = strings.TrimSuffix(optionsStr, ")")
		options := make(map[string]bool)
		for _, option := range strings.Split(optionsStr, ",") {
			options[option] = true
		}

		l.ActiveMountPoints = append(l.ActiveMountPoints, MountPoint{
			DevicePath: devicePath,
			MountPath:  mountPath,
			Options:    options,
		})
	}

	for _, mount := range l.ActiveMountPoints {
		util.LogDebug("ActiveMountPoints += %#v", mount)
	}
}

//GetMountPointsIn implements the Interface interface.
func (l *Linux) GetMountPointsIn(mountPathPrefix string) []MountPoint {
	if !strings.HasSuffix(mountPathPrefix, "/") {
		mountPathPrefix += "/"
	}

	var result []MountPoint
	for _, m := range l.ActiveMountPoints {
		if strings.HasPrefix(m.MountPath, mountPathPrefix) {
			result = append(result, m)
		}
	}
	return result
}

//GetMountPointsOf implements the Interface interface.
func (l *Linux) GetMountPointsOf(devicePath string) []MountPoint {
	var result []MountPoint
	for _, m := range l.ActiveMountPoints {
		if m.DevicePath == devicePath {
			result = append(result, m)
		}
	}
	return result
}
