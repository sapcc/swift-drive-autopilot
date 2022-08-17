/*******************************************************************************
*
* Copyright 2021 SAP SE
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

package parsers

import (
	"encoding/json"
	"fmt"
)

// LsblkOutput contains the parsed output from `lsblk -J`.
type LsblkOutput struct {
	BlockDevices []LsblkDevice `json:"blockdevices"`
}

// LsblkDevice appears in type LsblkOutput.
type LsblkDevice struct {
	Name       string        `json:"name"`
	MajorMinor string        `json:"maj:min"`
	Removable  bool          `json:"rm"`
	Size       string        `json:"size"`
	ReadOnly   bool          `json:"ro"`
	Type       string        `json:"type"`
	MountPoint *string       `json:"mountpoint"`
	Children   []LsblkDevice `json:"children"`
}

// ParseLsblkOutput parses output from `lsblk -J`.
func ParseLsblkOutput(buf string) (out LsblkOutput, err error) {
	err = json.Unmarshal([]byte(buf), &out)
	return
}

// FindBackingDeviceForLUKS returns the device path for the given LUKS mapping,
// or an empty string if the backing device cannot be determined from the lsblk
// output.
func (o LsblkOutput) FindBackingDeviceForLUKS(mappingName string) *string {
	for _, dev := range o.BlockDevices {
		devPath := dev.findBackingDeviceForLUKSRecursively(mappingName)
		if devPath != nil {
			return devPath
		}
	}
	return nil
}

func (d LsblkDevice) findBackingDeviceForLUKSRecursively(mappingName string) *string {
	for _, child := range d.Children {
		if child.Type == "crypt" && child.Name == mappingName {
			devPath := d.devicePath()
			return &devPath
		}
	}
	for _, child := range d.Children {
		devPath := child.findBackingDeviceForLUKSRecursively(mappingName)
		if devPath != nil {
			return devPath
		}
	}
	return nil
}

// FindSerialNumberForDevice returns the serial number for the device with the
// given path, by looking for a LUKS mapping directly below that device, which
// (by convention) has the serial number as its mapping name. This is a
// best-effort operation: If the drive is not encrypted or the respective LUKS
// container is not opened, this returns nil.
func (o LsblkOutput) FindSerialNumberForDevice(devicePath string) *string {
	dev := findDeviceByPath(o.BlockDevices, devicePath)
	if dev == nil {
		return nil
	}
	if len(dev.Children) != 1 {
		return nil
	}
	if dev.Children[0].Type != "crypt" {
		return nil
	}
	return &dev.Children[0].Name
}

func findDeviceByPath(devices []LsblkDevice, devicePath string) *LsblkDevice {
	for _, d := range devices {
		if d.devicePath() == devicePath {
			return &d
		}
		childResult := findDeviceByPath(d.Children, devicePath)
		if childResult != nil {
			return childResult
		}
	}
	return nil
}

func (d LsblkDevice) devicePath() string {
	switch d.Type {
	case "crypt", "mpath":
		return "/dev/mapper/" + d.Name
	case "disk", "part", "rom", "loop":
		return "/dev/" + d.Name
	default:
		panic(fmt.Sprintf("do not know how to compute devicePath for %#v", d))
	}
}
