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
	Log(LogDebug, "mounting %s to %s", devicePath, mountPath)

	//prepare new target directory
	_, err := ExecSimple(ExecChroot, nil, "mkdir", "-m", "0700", "-p", mountPath)
	if err != nil {
		Log(LogError, "exec(mkdir -p %s): %s", mountPath, err.Error())
		return false
	}

	//for the mount to appear both in the container and the host, it has to be
	//performed twice, once for each mount namespace involved
	_, err = ExecSimple(ExecChroot, nil, "mount", devicePath, mountPath)
	if err != nil {
		Log(LogError, "exec(mount %s) on host: %s", devicePath, err.Error())
		return false
	}
	if Config.ChrootPath != "" {
		_, err = ExecSimple(ExecChrootNoNsenter, nil, "mount", devicePath, mountPath)
		if err != nil {
			Log(LogError, "exec(mount %s) in container: %s", devicePath, err.Error())
			return false
		}
	}

	m.Active = true
	return true
}

//ReportToDebugLog reports the state of the MountPoint to Log(LogDebug).
func (m MountPoint) ReportToDebugLog(callerDesc, devicePath string) {
	if m.Active {
		Log(LogDebug, "%s: %s is mounted at %s", callerDesc, devicePath, m.Path())
	} else {
		Log(LogDebug, "%s: %s is not mounted below %s", callerDesc, devicePath, m.Location)
	}
}

//Chown changes the ownership of the mount point path to the given user and
//group. Both arguments may either be a name or a numeric ID (but still given
//as a string in decimal).
func (m MountPoint) Chown(user, group string) (success bool) {
	var (
		command string
		arg     string
	)

	//set only those things which were given
	if user == "" {
		if group == "" {
			return true // nothing to do
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
	_, err := ExecSimple(ExecChroot, nil, command, arg, mountPath)
	if err != nil {
		Log(LogError, "exec(%s %s %s): %s", command, arg, mountPath, err.Error())
		return false
	}
	return true
}
