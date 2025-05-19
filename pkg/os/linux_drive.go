// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

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

// CollectDrives implements the Interface interface.
func (l *Linux) CollectDrives(devicePathGlobs []string, trigger <-chan struct{}, added chan<- []Drive, removed chan<- []string) {
	knownDrives := make(map[string]string)

	// work loop
	for range trigger {
		// expand globs to find drives
		existingDrives := make(map[string]string)
		for _, pattern := range devicePathGlobs {
			// make pattern relative to current directory (== chroot directory)
			pattern = strings.TrimPrefix(pattern, "/")

			matches, err := filepath.Glob(pattern)
			if err != nil {
				logg.Fatal("glob(%#v) failed: %s", pattern, err.Error())
			}

			for _, globbedRelPath := range matches {
				// resolve any symlinks to get the actual devicePath (this also makes
				// the path absolute again)
				devicePath := must.Return(l.evalSymlinksInChroot(globbedRelPath))
				existingDrives["/"+globbedRelPath] = devicePath
			}
		}

		// fail loudly when there are no drives matching our glob
		// (https://github.com/sapcc/swift-drive-autopilot/issues/23)
		if len(existingDrives) == 0 {
			logg.Fatal("no drives found matching the configured patterns: %s",
				strings.Join(devicePathGlobs, ", "),
			)
		}

		// check if any of the reported drives have been removed
		var removedDrives []string
		for globbedPath, devicePath := range knownDrives {
			if _, exists := existingDrives[globbedPath]; !exists {
				removedDrives = append(removedDrives, devicePath)
				delete(knownDrives, globbedPath)
			}
		}

		if len(removedDrives) > 0 {
			sort.Strings(removedDrives) // test needs to be deterministic
			removed <- removedDrives
		}

		// handle new drives
		var addedDrives []Drive
		for globbedPath, devicePath := range existingDrives {
			// ignore drives that were already found in a previous run
			if _, exists := knownDrives[globbedPath]; exists {
				continue
			}
			knownDrives[globbedPath] = devicePath

			// ignore devices with partitions
			stdout, _ := command.Command{ExitOnError: false}.Run("sfdisk", "-l", devicePath)
			switch {
			case driveWithPartitionTableRx.MatchString(stdout):
				logg.Info("ignoring drive %s because it contains partitions", devicePath)
			case strings.TrimSpace(stdout) == "":
				// if `sfdisk -l` does not print anything at all, then the device is
				// not readable and should be ignored (e.g. on some servers, we have
				///dev/sdX which is a KVM remote volume that's usually not
				// accessible, i.e. open() fails with ENOMEDIUM; we want to ignore those)
				//
				// HOWEVER If the problem is an IO error and the drive has a LUKS
				// container already opened from before the IO error, we can see that in
				// `lsblk` and we can infer the serial number from the mapping name.
				// In this case we want to report the device so that the IO error gets
				// propagated upwards correctly.
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
				// drive is eligible -> find serial number and report it
				drive := Drive{
					DevicePath:  devicePath,
					FoundAtPath: globbedPath,
				}

				// read serial number using smartctl (using the relative path and skipping
				// nsenter and chroot here since the host may not have smartctl in its PATH)
				relDevicePath := strings.TrimPrefix(devicePath, "/")
				stdout, ok := command.Command{SkipLog: true, NoChroot: true, NoNsenter: true}.Run("smartctl", "-d", "scsi", "-i", relDevicePath)
				if ok {
					match := serialNumberRx.FindStringSubmatch(stdout)
					if match != nil {
						drive.SerialNumber = sanitizeSerialNumber(match[1])
					}
				}

				addedDrives = append(addedDrives, drive)
			}
		}

		if len(addedDrives) > 0 {
			sort.Slice(addedDrives, func(i, j int) bool { // test needs to be deterministic
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

func tryFindSerialNumberForBrokenDevice(devicePath string) *string {
	stdout, _ := command.Command{ExitOnError: true}.Run("lsblk", "-J")
	lsblkOutput, err := parsers.ParseLsblkOutput(stdout)
	if err != nil {
		logg.Fatal("cannot parse `lsblk -J` output: " + err.Error())
	}

	return lsblkOutput.FindSerialNumberForDevice(devicePath)
}
