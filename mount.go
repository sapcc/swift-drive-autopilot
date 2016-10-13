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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

//ScanMountPoints returns a mapping of device names to their mount points.
func ScanMountPoints() (map[string][]string, error) {
	chrootPath := Config.ChrootPath
	if chrootPath == "" {
		chrootPath = "/"
	}

	stdout, err := ExecChrootSimple("mount")
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	for _, line := range strings.Split(stdout, "\n") {
		//line looks like "<device> on <mountpoint> type <type> (<options>)"
		words := strings.Split(line, " ")
		if len(words) < 3 || words[1] != "on" {
			continue
		}
		devicePath, mountPath := words[0], words[2]

		//only consider mounts to actual block devices
		if !strings.HasPrefix(devicePath, "/") {
			continue
		}

		//only consider mount points below the chrootPath
		if strings.HasPrefix(mountPath, chrootPath) {
			rel, _ := filepath.Rel(chrootPath, mountPath)
			mountPath = filepath.Join("/", rel)

			//insert into map
			result[devicePath] = append(result[devicePath], mountPath)
		}
	}

	return result, nil
}

//MountDevice will mount the given device below /run/swift-storage if it has
//not been mounted yet. The second argument is the output of ScanMountPoints().
func MountDevice(devicePath string, allMounts map[string][]string) (mountPath string, e error) {
	//check if this device is already mounted somewhere
	for _, mountPath = range allMounts[devicePath] {
		return mountPath, nil
	}

	//prepare new target directory
	mountPath = filepath.Join("/run/swift-storage", md5sum(devicePath))
	_, err := ExecChrootSimple("mkdir", "-m", "0700", "-p", mountPath)
	if err != nil {
		return mountPath, fmt.Errorf("Cannot mkdir -p %s: %s", mountPath, err.Error())
	}

	//perform mount
	_, err = ExecChrootSimple("mount", devicePath, mountPath)
	if err != nil {
		return mountPath, fmt.Errorf("Cannot mount %s: %s", devicePath, err.Error())
	}

	return mountPath, nil
}

func md5sum(str string) string {
	s := md5.Sum([]byte(str))
	return hex.EncodeToString(s[:])
}
