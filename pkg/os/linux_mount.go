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
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

func (l *Linux) mountScopesAreSeparate() bool {
	return l.MountPropagationMode == SeparateMountNamespaces
}

func oppositeOf(scope MountScope) MountScope {
	if scope == HostScope {
		return LocalScope
	}
	return HostScope
}

// MountDevice implements the Interface interface.
func (l *Linux) MountDevice(devicePath, mountPath string, scope MountScope) bool {
	//check if already mounted
	for _, m := range l.ActiveMountPoints[scope] {
		if m.DevicePath == devicePath && m.MountPath == mountPath {
			return true
		}
	}

	//prepare target directory
	_, ok := command.Run("mkdir", "-m", "0700", "-p", mountPath)
	if !ok {
		return false
	}
	//execute mount
	_, ok = command.Command{NoNsenter: scope == LocalScope}.Run("mount", devicePath, mountPath)
	if !ok {
		return false
	}
	util.LogInfo("mounted %s to %s in %s mount namespace", devicePath, mountPath, scope)
	if !l.mountScopesAreSeparate() {
		util.LogInfo("mounted %s to %s in %s mount namespace", devicePath, mountPath, oppositeOf(scope))
	}

	//record the new mount
	m := MountPoint{
		DevicePath: devicePath,
		MountPath:  mountPath,
	}
	if l.mountScopesAreSeparate() {
		l.ActiveMountPoints[scope] = append(l.ActiveMountPoints[scope], m)
	} else {
		l.ActiveMountPoints[HostScope] = append(l.ActiveMountPoints[HostScope], m)
		l.ActiveMountPoints[LocalScope] = append(l.ActiveMountPoints[LocalScope], m)
	}

	return true
}

// UnmountDevice implements the Interface interface.
func (l *Linux) UnmountDevice(mountPath string, scope MountScope) bool {
	//check if already unmounted
	mounted := false
	for _, m := range l.ActiveMountPoints[scope] {
		if m.MountPath == mountPath {
			mounted = true
			break
		}
	}
	if !mounted {
		return true
	}

	//perform the unmount
	_, ok := command.Command{NoNsenter: scope == LocalScope}.Run("umount", mountPath)
	if !ok {
		return false
	}
	util.LogInfo("unmounted %s in %s mount namespace", mountPath, scope)
	if !l.mountScopesAreSeparate() {
		util.LogInfo("unmounted %s in %s mount namespace", mountPath, oppositeOf(scope))
	}

	//record that the unmount happened
	if l.mountScopesAreSeparate() {
		l.ActiveMountPoints[scope] = removeMountPoint(l.ActiveMountPoints[scope], mountPath)
	} else {
		l.ActiveMountPoints[HostScope] = removeMountPoint(l.ActiveMountPoints[HostScope], mountPath)
		l.ActiveMountPoints[LocalScope] = removeMountPoint(l.ActiveMountPoints[LocalScope], mountPath)
	}
	return true
}

func removeMountPoint(ms []MountPoint, mountPath string) []MountPoint {
	for idx, m := range ms {
		if m.MountPath == mountPath {
			return append(ms[:idx], ms[idx+1:]...)
		}
	}
	return ms
}

// RefreshMountPoints implements the Interface interface.
func (l *Linux) RefreshMountPoints() {
	l.ActiveMountPoints = map[MountScope][]MountPoint{LocalScope: collectMountPoints(LocalScope)}
	if l.mountScopesAreSeparate() {
		l.ActiveMountPoints[HostScope] = collectMountPoints(HostScope)
	} else {
		//make a deep copy to ensure that editing of one list does not affect the other one inadvertently
		l.ActiveMountPoints[HostScope] = append([]MountPoint(nil), l.ActiveMountPoints[LocalScope]...)
	}

	for scope, mounts := range l.ActiveMountPoints {
		for _, mount := range mounts {
			util.LogDebug("ActiveMountPoints[%s] += %#v", scope, mount)
		}
	}
}

func collectMountPoints(scope MountScope) (result []MountPoint) {
	//`mount` is executed with chroot even for LocalScope to ensure that paths are not prefixed with the ChrootPath
	stdout, _ := command.Command{ExitOnError: true, NoNsenter: scope == LocalScope}.Run("mount")

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

		//ignore mount points that have been duplicated by Docker/etc. for passing into a container
		if strings.HasPrefix(mountPath, "/var/lib/docker/") {
			continue
		}
		if strings.HasPrefix(mountPath, "/var/lib/rkt/") {
			continue
		}
		if strings.HasPrefix(mountPath, "/var/lib/kubelet/") {
			continue
		}

		result = append(result, MountPoint{
			DevicePath: devicePath,
			MountPath:  mountPath,
			Options:    options,
		})
	}
	return
}

// GetMountPointsIn implements the Interface interface.
func (l *Linux) GetMountPointsIn(mountPathPrefix string, scope MountScope) []MountPoint {
	if !strings.HasSuffix(mountPathPrefix, "/") {
		mountPathPrefix += "/"
	}

	var result []MountPoint
	for _, m := range l.ActiveMountPoints[scope] {
		if strings.HasPrefix(m.MountPath, mountPathPrefix) {
			result = append(result, m)
		}
	}
	return result
}

// GetMountPointsOf implements the Interface interface.
func (l *Linux) GetMountPointsOf(devicePath string, scope MountScope) []MountPoint {
	var result []MountPoint
	for _, m := range l.ActiveMountPoints[scope] {
		if m.DevicePath == devicePath {
			result = append(result, m)
		}
	}
	return result
}
