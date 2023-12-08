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

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/parsers"
)

// When a drive has a partition table, there will be a line like "Disklabel
// type: gpt" in the output of `sfdisk -l`. For unpartitioned devices, this line
// is missing.
var driveWithPartitionTableRx = regexp.MustCompile(`(?mi)^Disklabel type`)

// This is used to extract a drive's serial number from `smartctl -i`.
var serialNumberRx = regexp.MustCompile(`(?m)^Serial number:\s*(\S+)\s*$`)

// This is used to extract a drive's vendor from `smartctl -i`.
var vendorRx = regexp.MustCompile(`(?m)^Vendor:\s*(\S+)\s*$`)

// This is used to extract a drive's Rotation Rate from `smartctl -i`.
var rotationRateRx = regexp.MustCompile(`(?m)^Rotation Rate:\s*([a-zA-Z0-9- ]*)\s*$`)

// CollectDrives implements the Interface interface.
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
				logg.Fatal("glob(%#v) failed: %s", pattern, err.Error())
			}

			for _, globbedRelPath := range matches {
				//resolve any symlinks to get the actual devicePath (this also makes
				//the path absolute again)
				devicePath := must.Return(l.evalSymlinksInChroot(globbedRelPath))
				existingDrives["/"+globbedRelPath] = devicePath
			}
		}

		//fail loudly when there are no drives matching our glob
		//(https://github.com/sapcc/swift-drive-autopilot/issues/23)
		if len(existingDrives) == 0 {
			logg.Fatal("no drives found matching the configured patterns: %s",
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
				logg.Info("ignoring drive %s because it contains partitions", devicePath)
			case strings.TrimSpace(stdout) == "":
				//if `sfdisk -l` does not print anything at all, then the device is
				//not readable and should be ignored (e.g. on some servers, we have
				///dev/sdX which is a KVM remote volume that's usually not
				//accessible, i.e. open() fails with ENOMEDIUM; we want to ignore those)
				//
				//HOWEVER If the problem is an IO error and the drive has a LUKS
				//container already opened from before the IO error, we can see that in
				//`lsblk` and we can infer the serial number from the mapping name.
				//In this case we want to report the device so that the IO error gets
				//propagated upwards correctly.
				serialNumber := tryFindSerialNumberForBrokenDevice(devicePath)
				if serialNumber != nil {
					drive := Drive{
						DevicePath:   devicePath,
						FoundAtPath:  globbedPath,
						SerialNumber: *serialNumber,
					}
					addedDrives = append(addedDrives, drive)
				}
				logg.Info("ignoring drive %s because it is not readable", devicePath)
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

				ok = true
				stdout = `smartctl 7.3 2022-02-28 r5338 [x86_64-linux-6.1.62-flatcar] (local build)
Copyright (C) 2002-22, Bruce Allen, Christian Franke, www.smartmontools.org
=== START OF INFORMATION SECTION ===
Vendor:        NVMe
Product:       Micron_7450_MTFD
Revision:       U200
Compliance:      SPC-5
User Capacity:    15,360,950,534,144 bytes [15.3 TB]
Logical block size:  512 bytes
LU is resource provisioned, LBPRZ=1
Rotation Rate:    Solid State Device
Logical Unit id:   0x000000000000000200a0752342dede3a0x3adede420275a000
Serial number:    232942DEDE3A
Device type:     disk
Transport protocol:  PCIe
Local Time is:    Wed Dec 6 11:30:28 2023 UTC
SMART support is:   Available - device has SMART capability.
SMART support is:   Enabled
Temperature Warning: Enabled`

				if ok {
					match := serialNumberRx.FindStringSubmatch(stdout)
					vendor := vendorRx.FindStringSubmatch(stdout)
					rotationRate := rotationRateRx.FindStringSubmatch(stdout)

					logg.Info("Vendor: %s", vendor[1])
					logg.Info("Rotation Rate: %s", rotationRate[1])

					if match != nil {
						drive.SerialNumber = sanitizeSerialNumber(match[1])
					}

					if vendor != nil {
						drive.Vendor = strings.ToLower(sanitizeSerialNumber(vendor[1]))
					}

					if rotationRate != nil {
						drive.RotationRate = strings.ToLower(sanitizeSerialNumber(rotationRate[1]))
					}

					if drive.Vendor != "" && drive.RotationRate != "" {
						drive.Type = determineDriveType(drive)
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

var specialCharInSerialNumberRx = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// In some pathological cases, disk serial numbers may contain non-alphanumeric
// characters (e.g. we use iSCSI volumes instead of real disks in some of our QA
// environments and those have + or ] in their serial numbers). These chars
// could confuse the autopilot e.g. because `cryptsetup luksOpen` apparently
// does some escaping when creating mapped devices, so get rid of them early on.
func sanitizeSerialNumber(input string) string {
	return specialCharInSerialNumberRx.ReplaceAllString(input, "_")
}

func determineDriveType(drive Drive) string {
	if drive.Vendor == "nvme" {
		return "nvme"
	} else if strings.Contains(drive.RotationRate, "rpm") {
		return "hdd"
	} else {
		return "ssd"
	}
}

func tryFindSerialNumberForBrokenDevice(devicePath string) *string {
	stdout, _ := command.Command{ExitOnError: true}.Run("lsblk", "-J")
	lsblkOutput, err := parsers.ParseLsblkOutput(stdout)
	if err != nil {
		logg.Fatal("cannot parse `lsblk -J` output: " + err.Error())
	}

	return lsblkOutput.FindSerialNumberForDevice(devicePath)
}
