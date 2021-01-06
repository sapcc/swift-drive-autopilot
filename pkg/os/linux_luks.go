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
	"regexp"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/parsers"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//CreateLUKSContainer implements the Interface interface.
func (l *Linux) CreateLUKSContainer(devicePath, key string) bool {
	_, ok := command.Command{Stdin: key + "\n"}.Run("cryptsetup", "luksFormat", devicePath)
	return ok
}

//OpenLUKSContainer implements the Interface interface.
func (l *Linux) OpenLUKSContainer(devicePath, mappingName string, keys []string) (string, bool) {
	//try each key until one works
	for idx, key := range keys {
		util.LogDebug("trying to luksOpen %s as %s with key %d...", devicePath, mappingName, idx)
		_, ok := command.Command{
			Stdin:   key + "\n",
			SkipLog: true,
		}.Run("cryptsetup", "luksOpen", devicePath, mappingName)
		if ok {
			mappedDevicePath := "/dev/mapper/" + mappingName
			//remember this mapping
			if l.ActiveLUKSMappings == nil {
				l.ActiveLUKSMappings = make(map[string]string)
			}
			l.ActiveLUKSMappings[devicePath] = mappedDevicePath
			return mappedDevicePath, true
		}
	}

	//no key worked
	return "", false
}

//CloseLUKSContainer implements the Interface interface.
func (l *Linux) CloseLUKSContainer(mappingName string) bool {
	_, ok := command.Run("cryptsetup", "close", mappingName)
	return ok
}

//RefreshLUKSMappings implements the Interface interface.
func (l *Linux) RefreshLUKSMappings() {
	stdout, _ := command.Command{ExitOnError: true}.Run("lsblk", "-J")
	lsblkOutput, err := parsers.ParseLsblkOutput(stdout)
	if err != nil {
		util.LogFatal("cannot parse `lsblk -J` output: " + err.Error())
	}

	l.ActiveLUKSMappings = make(map[string]string)
	stdout, _ = command.Command{ExitOnError: true}.Run("dmsetup", "ls", "--target=crypt")

	if strings.TrimSpace(stdout) == "No devices found" {
		return
	}

	for _, line := range strings.Split(stdout, "\n") {
		//each output line describes a mapping and looks like
		//"mapname\t(devmajor, devminor)"; extract the mapping names
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		mappingName := fields[0]

		//check lsblk output for the device backing this mapping, otherwise ask cryptsetup for it
		backingDevicePath := lsblkOutput.FindBackingDeviceForLUKS(mappingName)
		if backingDevicePath == nil {
			backingDevicePath = l.getBackingDevicePath(mappingName)
		}
		if backingDevicePath != nil {
			l.ActiveLUKSMappings[*backingDevicePath] = "/dev/mapper/" + mappingName
		}
	}
	return
}

var backingDeviceRx = regexp.MustCompile(`(?m)^\s*device:\s*(\S+)\s*$`)

//Ask cryptsetup for the device backing an open LUKS container.
func (l *Linux) getBackingDevicePath(mapName string) *string {
	stdout, _ := command.Command{ExitOnError: true}.Run("cryptsetup", "status", mapName)

	//look for a line like "  device:  /dev/sdb"
	match := backingDeviceRx.FindStringSubmatch(stdout)
	if match == nil {
		util.LogFatal("cannot find backing device for /dev/mapper/%s", mapName)
	} else if match[1] == "(null)" {
		util.LogError("skipping /dev/mapper/%s: `cryptsetup status` reports backing device as `(null)` (the kernel log probably has an IO error for the underlying device)", mapName)
		return nil
	} else {
		//resolve any symlinks to get the actual devicePath
		//when the luks container is created on top of multipathing, cryptsetup status might report the /dev/mapper/mpath device
		//also the luksFormat was called on actual device
		devicePath, err := l.evalSymlinksInChroot(match[1])
		if err != nil {
			util.LogFatal(err.Error())
		}
		if devicePath != match[1] {
			util.LogDebug("backing device path for %s is %s -> %s", mapName, match[1], devicePath)
			return &devicePath
		}
		util.LogDebug("backing device path for %s is %s", mapName, match[1])
	}
	return &match[1]
}

//GetLUKSMappingOf implements the Interface interface.
func (l *Linux) GetLUKSMappingOf(devicePath string) string {
	return l.ActiveLUKSMappings[devicePath]
}
