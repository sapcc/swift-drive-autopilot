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
	"path/filepath"
	"strings"
)

//DeviceType describes the contents of a device, to the granularity required by
//this program.
type DeviceType int

const (
	//DeviceTypeNotScanned says that Drive.Classify() has not been run for this
	//device yet.
	DeviceTypeNotScanned DeviceType = iota
	//DeviceTypeUnknown describes a device that contains neither a LUKS
	//container nor a filesystem.
	DeviceTypeUnknown
	//DeviceTypeLUKS describes a device that contains a LUKS container.
	DeviceTypeLUKS
	//DeviceTypeFilesystem describes a device that contains a filesystem.
	DeviceTypeFilesystem
)

//Drive contains all the information about a single drive.
type Drive struct {
	//DevicePath is where the device file is located (with all symlinks
	//resolved, e.g. "/dev/sdc" instead of "/dev/disk/by-path/..."). The path
	//is absolute and refers to a location in the chroot (if any).
	DevicePath string
	//MappedDevicePath is only set when the device at DevicePath is encrypted
	//with LUKS. After the LUKS container is opened, MappedDevicePath is the
	//device file below /dev/mapper with the decrypted block device
	//(e.g. "/dev/mapper/swift-a45e9353-7836-4de6-ba1a-2c69ab577e91").
	MappedDevicePath string
	//Type describes the contents of this drive's device file, as
	//returned by file(1). This field refers to the device at DevicePath,
	//unless MappedDevicePath is set, in which case it refers to that device.
	//A value of nil means that Classify() has not been run yet.
	Type DeviceType
	//TemporaryMount is this device's mount point below /run/swift-storage.
	TemporaryMount MountPoint
	//FinalMount is this device's mount point below /srv/node.
	FinalMount MountPoint
}

//Drives is a list of Drive structs with some extra methods.
type Drives []*Drive

//ListDrives returns the list of all Swift storage drives, by expanding the
//shell globs in Config.DriveGlobs and resolving any symlinks.
func ListDrives() Drives {
	var result Drives

	for _, pattern := range Config.DriveGlobs {
		//make pattern relative to current directory (== chroot directory)
		pattern = strings.TrimPrefix(pattern, "/")

		matches, err := filepath.Glob(pattern)
		if err != nil {
			Log(LogFatal, "glob(%#v) failed: %s", pattern, err.Error())
		}
		if len(matches) == 0 {
			//this could hint at a misconfiguration
			Log(LogError, "ListDrives: %s does not match anything", pattern)
		} else {
			//when logging, prepend slashes to all matches because they are relative paths!
			Log(LogDebug, "ListDrives: %s matches /%s", pattern, strings.Join(matches, ", /"))
		}

		for _, match := range matches {
			//resolve any symlinks to get the actual devicePath
			devicePath, err := filepath.EvalSymlinks(match)
			if err != nil {
				Log(LogFatal, "readlink(%#v) failed: %s", match, err.Error())
			}

			//make path absolute if necessary (the glob was a relative path!)
			if !strings.HasPrefix(devicePath, "/") {
				devicePath = "/" + devicePath
			}
			result = append(result, newDrive(devicePath))

			if devicePath != "/"+match {
				Log(LogDebug, "ListDrives: resolved %s to %s", match, devicePath)
			}
		}
	}

	return result
}

func newDrive(devicePath string) *Drive {
	//default value for MountID is md5sum of devicePath
	s := md5.Sum([]byte(devicePath))
	mountID := hex.EncodeToString(s[:])

	//- MappedDevicePath will be initialized by TryDecrypt()
	//- MountPoint.Active will be initialized by ScanDriveMountPoints()
	//- FinalMount.Name will be initialized by ScanDriveSwiftIDs()
	return &Drive{
		DevicePath:       devicePath,
		MappedDevicePath: "",
		TemporaryMount: MountPoint{
			Location: "/run/swift-storage",
			Name:     mountID,
			Active:   false,
		},
		FinalMount: MountPoint{
			Location: "/srv/node",
			Name:     "",
			Active:   false,
		},
	}
}

//ActiveDevicePath is usually DevicePath, but if the drive is LUKS-encrypted
//and the LUKS container has already been opened, MappedDevicePath is returned.
func (d Drive) ActiveDevicePath() string {
	if d.MappedDevicePath == "" {
		return d.DevicePath
	}
	return d.MappedDevicePath
}

//Classify will call file(1) on the drive's device file (or the mapped device
//file, if any), and save the result in the Classification field.
func (d *Drive) Classify() (success bool) {
	//run only once
	if d.Type != DeviceTypeNotScanned {
		return true
	}

	//ask file(1) to identify the contents of this device
	//BUT: do not run file(1) in the chroot (e.g. CoreOS does not have file(1))
	devicePath := d.ActiveDevicePath()
	desc, ok := Command{
		NoChroot: true,
	}.Run("file", "-bLs", Config.ChrootPath+devicePath)
	if !ok {
		return false
	}

	//convert into DeviceType
	switch {
	case strings.HasPrefix(desc, "LUKS encrypted file"):
		d.Type = DeviceTypeLUKS
	case strings.Contains(desc, "filesystem data"):
		d.Type = DeviceTypeFilesystem
	default:
		d.Type = DeviceTypeUnknown
	}

	return true
}

//EnsureFilesystem will check if the device contains a filesystem, and if not,
//create an XFS. (Swift requires a filesystem that supports extended
//attributes, and XFS is the most popular choice.)
func (d *Drive) EnsureFilesystem() (success bool) {
	//is it safe to be formatted? (i.e. don't format when there is already a
	//filesystem or LUKS container)
	if !d.Classify() {
		return false
	}
	if d.Type != DeviceTypeUnknown {
		return true
	}

	//format device with XFS
	devicePath := d.ActiveDevicePath()
	_, ok := Run("mkfs.xfs", devicePath)
	if !ok {
		return false
	}
	Log(LogDebug, "XFS filesystem created on %s", devicePath)

	return true
}

//MountSomewhere will mount the given device below `/run/swift-storage` if it
//has not been mounted yet.
func (d *Drive) MountSomewhere() (success bool) {
	//already mounted somewhere?
	if d.FinalMount.Active {
		return true
	}
	return d.TemporaryMount.Activate(d.ActiveDevicePath())
}
