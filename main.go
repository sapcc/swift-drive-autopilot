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

import "os"

func main() {
	//set working directory to the chroot directory; this simplifies file
	//system operations because we can just use relative paths to refer to
	//stuff inside the chroot
	workingDir := "/"
	if Config.ChrootPath != "" {
		workingDir = Config.ChrootPath
	}
	err := os.Chdir(workingDir)
	if err != nil {
		Log(LogFatal, "chdir to %s: %s", workingDir, err.Error())
	}

	//list drives
	drives := ListDrives()
	drives.ScanOpenLUKSContainers()

	//open LUKS containers if required
	failed := false
	for _, drive := range drives {
		if !drive.OpenLUKS() {
			failed = true //but keep going for the drives that work
			continue
		}
	}

	//try to mount all drives to /run/swift-storage (if not yet mounted)
	drives.ScanMountPoints()
	for _, drive := range drives {
		if !drive.EnsureFilesystem() {
			failed = true //but keep going for the drives that work
			continue
		}
		if !drive.MountSomewhere() {
			failed = true //but keep going for the drives that work
			continue
		}
	}

	//map mountpoints from /run/swift-storage to /srv/node
	if !drives.ScanSwiftIDs() {
		failed = true //but keep going for the drives that work
	}

	for _, drive := range drives {
		if drive.FinalMount.Activate(drive.ActiveDevicePath()) {
			Log(LogInfo, "%s is mounted on %s", drive.DevicePath, drive.FinalMount.Path())
		} else {
			failed = true //but keep going for the drives that work
			continue
		}

		owner := Config.Owner
		if !drive.FinalMount.Chown(owner.User, owner.Group) {
			failed = true
		}
	}

	//mark /srv/node as ready
	_, err = ExecSimple(ExecChroot, nil, "touch", "/srv/node/ready")
	if err != nil {
		Log(LogError, "touch /srv/node/ready: %s", err.Error())
		failed = true
	}

	//signal intermittent failures to the caller
	if failed {
		Log(LogInfo, "completed with errors, see above")
		os.Exit(1)
	}
}
