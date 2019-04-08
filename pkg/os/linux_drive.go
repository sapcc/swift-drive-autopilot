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
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//When a drive has a partition table, there will be a line like "Disklabel
//type: gpt" in the output of `sfdisk -l`. For unpartitioned devices, this line
//is missing.
var driveWithPartitionTableRx = regexp.MustCompile(`(?mi)^Disklabel type`)

//This is used to extract a drive's serial number from `smartctl -i`.
var serialNumberRx = regexp.MustCompile(`(?m)^Serial number:\s*(\S+)\s*$`)

//CollectDrives implements the Interface interface.
func (l *Linux) CollectDrives(devicePathGlobs []string, trigger <-chan struct{}, added chan<- []Drive, removed chan<- []string) {
	knownDrives := make(map[string]string)

	//work loop
	for range trigger {
		//expand globs to find drives
		existingDrives := make(map[string]string)
		for _, pattern := range devicePathGlobs {
			//make pattern relative to current directory (== chroot directory)
			pattern = strings.TrimPrefix(pattern, "/")

			matches, err := filepath.Glob(pattern)
			if err != nil {
				util.LogFatal("glob(%#v) failed: %s", pattern, err.Error())
			}

			for _, globbedRelPath := range matches {
				//resolve any symlinks to get the actual devicePath (this also makes
				//the path absolute again)
				devicePath, err := l.evalSymlinksInChroot(globbedRelPath)
				if err != nil {
					util.LogFatal(err.Error())
				}

				existingDrives["/"+globbedRelPath] = devicePath
			}
		}

		//fail loudly when there are no drives matching our glob
		//(https://github.com/sapcc/swift-drive-autopilot/issues/23)
		if len(existingDrives) == 0 {
			util.LogFatal("no drives found matching the configured patterns: %s",
				strings.Join(devicePathGlobs, ", "),
			)
		}

		//check if any of the reported drives have been removed
		var removedDrives []string
		for globbedPath, devicePath := range knownDrives {
			if _, exists := existingDrives[globbedPath]; !exists {
				removedDrives = append(removedDrives, devicePath)
				delete(knownDrives, globbedPath)
			}
		}

		if len(removedDrives) > 0 {
			sort.Strings(removedDrives) //test needs to be deterministic
			removed <- removedDrives
		}

		//handle new drives
		var addedDrives []Drive
		for globbedPath, devicePath := range existingDrives {
			//ignore drives that were already found in a previous run
			if _, exists := knownDrives[globbedPath]; exists {
				continue
			}
			knownDrives[globbedPath] = devicePath

			//ignore devices with partitions
			stdout, _ := command.Command{ExitOnError: false}.Run("sfdisk", "-l", devicePath)
			switch {
			case driveWithPartitionTableRx.MatchString(stdout):
				util.LogInfo("ignoring drive %s because it contains partitions", devicePath)
			case strings.TrimSpace(stdout) == "":
				//if `sfdisk -l` does not print anything at all, then the device is
				//not readable and should be ignored (e.g. on some servers, we have
				///dev/sdX which is a KVM remote volume that's usually not
				//accessible, i.e. open() fails with ENOMEDIUM; we want to ignore those)
				util.LogInfo("ignoring drive %s because it is not readable", devicePath)
			default:
				//drive is eligible -> find serial number and report it
				drive := Drive{
					DevicePath:  devicePath,
					FoundAtPath: globbedPath,
				}

				//read serial number using smartctl (using the relative path and skipping
				//nsenter and chroot here since the host may not have smartctl in its PATH)
				relDevicePath := strings.TrimPrefix(devicePath, "/")
				stdout, ok := command.Command{SkipLog: true, NoChroot: true, NoNsenter: true}.Run("smartctl", "-d", "scsi", "-i", relDevicePath)
				if ok {
					match := serialNumberRx.FindStringSubmatch(stdout)
					if match != nil {
						drive.SerialNumber = match[1]
					}
				}

				addedDrives = append(addedDrives, drive)
			}
		}

		if len(addedDrives) > 0 {
			sort.Slice(addedDrives, func(i, j int) bool { //test needs to be deterministic
				return addedDrives[i].DevicePath < addedDrives[j].DevicePath
			})
			added <- addedDrives
		}
	}
}
