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

//Interface describes the set of OS-level operations that can be executed by
//the autopilot. The default implementation for production is struct Linux in
//this package.
//
//There is an important distinction between "drive" and "device" in the
//autopilot's jargon. A "drive" is the physical thing, a "device" is a device
//file. For encrypted drives, there are two devices for each drive: the
//original SCSI device file (e.g. /dev/sda) and the device file representing
//the contents of the LUKS container (e.g. /dev/mapper/ABCDEFGH).
type Interface interface {
	//CollectDrives is run in a separate goroutine and reports drives as they are
	//added or removed. (When first started, all existing drives shall be
	//reported as "added".) It shall not return.
	CollectDrives(devicePathGlobs []string, added chan<- []Drive, removed chan<- []string)
	//CollectDriveErrors is run in a separate goroutine and reports drive errors
	//that are observed in the kernel log. It shall not return.
	CollectDriveErrors(errors chan<- []DriveError)

	//ClassifyDevice examines the contents of the given device to detect existing
	//LUKS containers or filesystems.
	ClassifyDevice(devicePath string) DeviceType
	//FormatDevice creates an XFS filesystem on this device. Existing containers
	//or filesystems will be overwritten.
	FormatDevice(devicePath string) (ok bool)

	//MountDevice mounts this device at the given location. If
	//repeatInOwnNamespace is true, the mount is also performed in this process's
	//own mount namespace.
	MountDevice(devicePath, mountPath string, repeatInOwnNamespace bool) (ok bool)
	//UnmountDevice unmounts the device that is mounted at the given location.
	UnmountDevice(mountPath string, repeatInOwnNamespace bool) (ok bool)
	//RefreshMountPoints examines the system to find any mounts that have changed
	//since we last looked.
	RefreshMountPoints()
	//GetMountPointsIn returns all active mount points below the given path.
	GetMountPointsIn(mountPathPrefix string) []MountPoint
	//GetMountPointsOf returns all active mount points for this device.
	GetMountPointsOf(devicePath string) []MountPoint

	//CreateLUKSContainer creates a LUKS container on the given device, using the
	//given encryption key. Existing data on the device will be overwritten.
	CreateLUKSContainer(devicePath, key string) (ok bool)
	//OpenLUKSContainer opens the LUKS container on the given device. The given
	//keys are tried in order until one works.
	OpenLUKSContainer(devicePath, mappingName string, keys []string) (mappedDevicePath string, ok bool)
	//CloseLUKSContainer closes the LUKS container with the given mapping name.
	CloseLUKSContainer(mappingName string) (ok bool)
}

//Drive contains information about a drive as detected by the OS.
type Drive struct {
	DevicePath   string
	FoundAtPath  string //only used in log messages
	SerialNumber string
}

//DriveError represents a drive error that was found e.g. in a kernel log.
type DriveError struct {
	DevicePath string
	Message    string
}

//DeviceType describes the contents of a device, to the granularity required by
//this program.
type DeviceType int

const (
	//DeviceTypeUnknown describes a device that is readable, but contains neither
	//a LUKS container nor a filesystem.
	DeviceTypeUnknown DeviceType = iota
	//DeviceTypeUnreadable is returned by ClassifyDevice() when the device is
	//unreadable.
	DeviceTypeUnreadable
	//DeviceTypeLUKS describes a device that contains a LUKS container.
	DeviceTypeLUKS
	//DeviceTypeFilesystem describes a device that contains an admissible
	//filesystem.
	DeviceTypeFilesystem
)

//MountPoint describes an active mount point that is present on the system.
type MountPoint struct {
	DevicePath string
	MountPath  string
	Options    map[string]bool
}
