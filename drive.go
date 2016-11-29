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

import "strings"

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
	//DeviceID is hex.EncodeToString(md5.Sum(DevicePath)). This string is used
	//to identify this drive in derived filenames.
	DeviceID string
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
	//Broken is set when some operation regarding the drive fails, or if the
	//device or its mountpoints act in a way that is inconsistent with our
	//expectations. Broken drives will be unmounted from /srv/node so that
	//Swift will stop consuming them.
	Broken bool
	//Converged is set when Drive.Converge() runs, to ensure that it does not
	//run multiple times in a single event loop iteration.
	Converged bool
	//StartedOutEmpty is set during Drive.Converge() if the device did not
	//contain a filesystem, LUKS container, or active mount when
	//Drive.Converge() started. This flag triggers swift-id auto-assignment.
	StartedOutEmpty bool
}

//Drives is a list of Drive structs with some extra methods.
type Drives []*Drive

//ActiveDevicePath is usually DevicePath, but if the drive is LUKS-encrypted
//and the LUKS container has already been opened, MappedDevicePath is returned.
func (d Drive) ActiveDevicePath() string {
	if d.MappedDevicePath == "" {
		return d.DevicePath
	}
	return d.MappedDevicePath
}

//BrokenFlagPath returns the path to the symlink that is written to the
//filesystem to flag this drive as broken.
func (d Drive) BrokenFlagPath() string {
	return "/run/swift-storage/broken/" + d.DeviceID
}

//MarkAsBroken sets the Broken flag on the drive.
func (d *Drive) MarkAsBroken() {
	if d.Broken {
		return
	}

	d.Broken = true
	Log(LogInfo, "flagging %s as broken because of previous error", d.DevicePath)

	brokenFlagPath := d.BrokenFlagPath()
	_, ok := Run("ln", "-sfT", d.DevicePath, brokenFlagPath)
	if ok {
		Log(LogInfo, "To reinstate this drive into the cluster, delete the symlink at "+brokenFlagPath)
	}

	d.FinalMount.Deactivate(d.DevicePath)
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
		d.MarkAsBroken()
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
func (d *Drive) EnsureFilesystem() {
	//do not touch broken stuff
	if d.Broken {
		return
	}
	//is it safe to be formatted? (i.e. don't format when there is already a
	//filesystem or LUKS container)
	if !d.Classify() {
		return
	}
	if d.Type != DeviceTypeUnknown {
		return
	}

	//format device with XFS
	devicePath := d.ActiveDevicePath()
	_, ok := Run("mkfs.xfs", devicePath)
	if !ok {
		d.MarkAsBroken()
		return
	}
	Log(LogDebug, "XFS filesystem created on %s", devicePath)

	//do not attempt to format again during the next Converge
	d.Type = DeviceTypeFilesystem
}

//MountSomewhere will mount the given device below `/run/swift-storage` if it
//has not been mounted yet.
func (d *Drive) MountSomewhere() {
	//do not touch broken stuff
	if d.Broken {
		return
	}
	//already mounted somewhere?
	if d.FinalMount.Active {
		return
	}
	ok := d.TemporaryMount.Activate(d.ActiveDevicePath())
	if !ok {
		d.MarkAsBroken()
	}
}

//CheckMounts takes the return values of ScanMountPoints() and checks where the
//given drive is mounted. False is returned if the state of the Drive is
//inconsistent with the mounts lists.
func (d *Drive) CheckMounts(activeMounts SystemMountPoints) {
	//if a LUKS container is open, then the base device should not be mounted
	if d.MappedDevicePath != "" {
		for _, m := range activeMounts {
			if m.DevicePath == d.DevicePath {
				Log(LogError, "%s contains an open LUKS container, but is also mounted directly", d.DevicePath)
				d.MarkAsBroken()
				return
			}
		}
	}

	//check that the mountpoints recorded in this Drive are consistent with the
	//actual system state
	devicePath := d.ActiveDevicePath()
	tempMountOk := d.TemporaryMount.Check(devicePath, activeMounts)
	finalMountOk := d.FinalMount.Check(devicePath, activeMounts)

	success := tempMountOk && finalMountOk
	if !success {
		d.MarkAsBroken()
	}

	//drive cannot be empty if it is mounted
	if d.TemporaryMount.Active || d.FinalMount.Active {
		d.StartedOutEmpty = false
	}
}

//CleanupDuplicateMounts will deactivate the temporary mount if the final mount
//is active.
func (d *Drive) CleanupDuplicateMounts() {
	//do not touch broken stuff
	if d.Broken {
		return
	}

	if d.TemporaryMount.Active && d.FinalMount.Active {
		d.TemporaryMount.Deactivate(d.DevicePath)
	}
}

//Converge moves the drive into its locally desired state.
//
//If the drive is not broken, its LUKS container (if any) will be created
//and/or opened, and its filesystem will be mounted. The only thing missing
//will be the final mount (since this step needs knowledge of all drives to
//check for swift-id collisions) and the swift-id auto-assignment.
//
//If the drive is broken (or discovered to be broken during this operation),
//no new mappings and mounts will be performed.
func (d *Drive) Converge(c *Converger) {
	if d.Converged || d.Broken {
		return
	}

	//before converging, check if device is empty and initialize the
	//StartedOutEmpty flag accordingly (note that StartedOutEmpty might be
	//reset by CheckLUKS or CheckMounts if an active mapping or mount is found
	//for this drive)
	ok := d.Classify()
	if !ok {
		return
	}
	d.StartedOutEmpty = d.Type == DeviceTypeUnknown

	d.CheckLUKS(c.ActiveLUKSMappings)
	if len(Config.Keys) > 0 {
		d.FormatLUKSIfRequired()
		d.OpenLUKS()
	}
	//try to mount the drive to /run/swift-storage (if not yet mounted)
	d.CheckMounts(c.ActiveMounts)
	d.EnsureFilesystem()
	d.MountSomewhere()

	d.Converged = true

	//if the device was marked as broken during this run, we need to update
	//c.ActiveMounts to avoid a false-negative in c.CheckForUnexpectedMounts()
	if d.Broken {
		c.ActiveMounts.MarkAsDeactivated(d.FinalMount.Path())
		//NOTE: This might suppress an actual unexpected mount for one event
		//loop iteration, but the error will show up at most 30 seconds later
		//during the scheduled healthcheck.
	}
}
