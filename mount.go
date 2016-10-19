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

//MountSomewhere will mount the given device below `/run/swift-storage` if it
//has not been mounted yet.
func (d *Drive) MountSomewhere() (success bool) {
	//already mounted somewhere?
	if d.Mounted || d.Mapped {
		return true
	}

	//mount in /run/swift-storage
	mountPath := "/run/swift-storage/" + d.MountID
	d.Mounted = doMount(d.DevicePath, mountPath)
	return d.Mounted
}

func doMount(fromPath, toPath string) (success bool) {
	//prepare new target directory
	_, err := ExecSimple(ExecChroot, "mkdir", "-m", "0700", "-p", toPath)
	if err != nil {
		Log(LogError, "exec(mkdir -p %s): %s", toPath, err.Error())
		return false
	}

	//for the mount to appear both in the container and the host, it has to be
	//performed twice, once for each mount namespace involved
	_, err = ExecSimple(ExecChrootNsenter, "mount", fromPath, toPath)
	if err != nil {
		Log(LogError, "exec(mount %s) on host: %s", fromPath, err.Error())
		return false
	}
	_, err = ExecSimple(ExecChroot, "mount", fromPath, toPath) //without nsenter!
	if err != nil {
		Log(LogError, "exec(mount %s) in container: %s", fromPath, err.Error())
		return false
	}

	return true
}

//MountForSwift mounts the given drive below `/srv/node`.
func (d *Drive) MountForSwift() (success bool) {
	//ready to be mounted?
	if d.SwiftID == "" {
		return false
	}
	//already mounted?
	if d.Mapped {
		return true
	}

	//mount in /srv/node
	mountPath := "/srv/node/" + d.SwiftID
	d.Mapped = doMount(d.DevicePath, mountPath)
	return d.Mapped
}
