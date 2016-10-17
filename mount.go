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
	"io/ioutil"
	"log"
	"os"
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

		//make mountPath relative to chroot dir (which is "/" because of ExecChroot)
		mountPath = strings.TrimPrefix(mountPath, "/")
		result[devicePath] = append(result[devicePath], mountPath)
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
	return strings.TrimPrefix(mountPath, "/"), doMount(devicePath, mountPath)
}

func doMount(fromPath, toPath string) error {
	//prepare new target directory
	_, err := ExecChrootSimple("mkdir", "-m", "0700", "-p", toPath)
	if err != nil {
		return fmt.Errorf("mkdir -p %s: %s", toPath, err.Error())
	}

	//perform mount
	_, err = ExecChrootSimple("mount", fromPath, toPath)
	if err != nil {
		return fmt.Errorf("mount %s: %s", fromPath, err.Error())
	}
	return nil
}

func md5sum(str string) string {
	s := md5.Sum([]byte(str))
	return hex.EncodeToString(s[:])
}

//ScanSwiftID looks at all drives mounted below /run/swift-storage and
//returns a mapping of the contents of their swift-id file to the device path
//(relative to the chroot dir). The argument is the output of
//ScanMountPoints().
func ScanSwiftID(allMounts map[string][]string) (result map[string]string, failed bool) {
	chrootPath := Config.ChrootPath
	if chrootPath == "" {
		chrootPath = "/"
	}

	//find devices mounted below /run/swift-storage
	drivesByID := make(map[string]string)
	for device, deviceMounts := range allMounts {
		for _, deviceMount := range deviceMounts {
			if !strings.HasPrefix(deviceMount, "run/swift-storage/") {
				continue
			}

			//read this device's swift-id
			idPath := filepath.Join(deviceMount, "swift-id")
			idBytes, err := ioutil.ReadFile(filepath.Join(chrootPath, idPath))
			if err != nil {
				if os.IsNotExist(err) {
					log.Printf("ERROR: no swift-id file found on device %s (mounted at /%s)", device, deviceMount)
				} else {
					log.Printf("ERROR: read /%s: %s", idPath, err.Error())
				}
				continue
			}
			idStr := strings.TrimSpace(string(idBytes))

			//is this the first device with this swift-id?
			_, exists := drivesByID[idStr]
			if exists {
				//no - do not mount any of them, just complain
				log.Printf("ERROR: multiple drives with swift-id \"%s\" (not mounting any of them)", idStr)
				//leave entry in drivesByID hash to detect further duplicates,
				//but set value to empty string to skip during mounting
				drivesByID[idStr] = ""
			} else {
				//yes - mount it after this loop
				drivesByID[idStr] = device
			}
		}
	}

	//remove all the entries that were marked as duplicates
	result = make(map[string]string)
	for idStr, device := range drivesByID {
		if device != "" {
			result[idStr] = device
		}
	}

	log.Printf("DEBUG: result = %#v, failed = %#v\n", result, failed)
	return
}

//ExecuteFinalMount mounts the given device into `/srv/node`. The last argument
//is the output of ScanMountPoints().
func ExecuteFinalMount(devicePath, swiftID string, allMounts map[string][]string) error {
	log.Printf("DEBUG: called ExecuteFinalMount(%#v, %#v)\n", devicePath, swiftID)

	//check if this device is already mounted at the desired location
	mountPath := "/srv/node/" + swiftID
	for _, otherMountPath := range allMounts[devicePath] {
		if "/"+otherMountPath == mountPath {
			return nil
		}
	}

	return doMount(devicePath, mountPath)
}
