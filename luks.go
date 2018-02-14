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
	"path/filepath"

	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//OpenLUKS will open a LUKS container on the given drive, and set
//MappedDevicePath accordingly. If the drive is not encrypted with LUKS,
//OpenLUKS returns true without doing anything.
func (d *Drive) OpenLUKS(osi os.Interface) {
	//do not touch broken stuff
	if d.Broken {
		return
	}
	//already done?
	if d.MappedDevicePath != "" {
		return
	}
	//is the drive encrypted?
	if !d.Classify(osi) {
		return
	}
	if *d.Type != os.DeviceTypeLUKS {
		return
	}

	//decrypt drive
	keyList := make([]string, len(Config.Keys))
	for idx, key := range Config.Keys {
		keyList[idx] = key.Secret
	}
	mappingName := d.TemporaryMount.Name
	mappedDevicePath, ok := osi.OpenLUKSContainer(d.DevicePath, mappingName, keyList)
	if !ok {
		util.LogError(
			"exec(cryptsetup luksOpen %s %s) failed: none of the configured keys was accepted",
			d.DevicePath, mappingName,
		)
		d.MarkAsBroken(osi)
		return
	}

	d.MappedDevicePath = mappedDevicePath
	d.Type = nil //reset because Classification now refers to what's in the mapped device
	util.LogInfo("LUKS container at %s opened as %s", d.DevicePath, d.MappedDevicePath)
}

//CloseLUKS will close the LUKS container on the given drive, if it exists and
//is currently open.
func (d *Drive) CloseLUKS(osi os.Interface) {
	//anything to do?
	if d.MappedDevicePath == "" {
		return
	}

	ok := osi.CloseLUKSContainer(filepath.Base(d.MappedDevicePath))
	if ok {
		util.LogInfo("LUKS container %s closed", d.MappedDevicePath)
		d.MappedDevicePath = ""
	}
}

//CheckLUKS fills the MappedDevicePath of this Drive if it is mapped. False is
//returned if any inconsistencies are found.
func (d *Drive) CheckLUKS(osi os.Interface) {
	actualMappedPath := osi.GetLUKSMappingOf(d.DevicePath)

	if actualMappedPath == "" {
		if d.MappedDevicePath != "" {
			util.LogError("LUKS container in %s should be open at %s, but is not",
				d.DevicePath, d.MappedDevicePath,
			)
			d.MarkAsBroken(osi)
		}
		return
	}

	switch d.MappedDevicePath {
	case "":
		//existing mapping is now discovered for the first time -> update Drive struct
		d.MappedDevicePath = actualMappedPath
		util.LogInfo("discovered %s to be mapped to %s already", d.DevicePath, d.MappedDevicePath)
		//device cannot be empty if a LUKS mapping is active
		d.StartedOutEmpty = false
	case actualMappedPath:
		//no change
	default:
		//our internal state tells a different story!
		util.LogError("LUKS container in %s should be open at %s, but is actually open at %s",
			d.DevicePath, d.MappedDevicePath, actualMappedPath,
		)
		d.MarkAsBroken(osi)
	}
}

//FormatLUKSIfRequired will create a LUKS container on this device if empty.
func (d *Drive) FormatLUKSIfRequired(osi os.Interface) {
	//do not touch broken stuff
	if d.Broken {
		return
	}
	//we can skip all of this if the LUKS container exists and is mapped already
	if d.MappedDevicePath != "" {
		return
	}

	//sanity check
	if len(Config.Keys) == 0 {
		util.LogFatal("FormatLUKSIfRequired called on %s, but no keys specified!", d.DevicePath)
	}

	//is it safe to be formatted? (i.e. don't format when there is already a
	//filesystem or LUKS container)
	if !d.Classify(osi) {
		return
	}
	if *d.Type != os.DeviceTypeUnknown {
		return
	}

	//format with the preferred key
	ok := osi.CreateLUKSContainer(d.DevicePath, Config.Keys[0].Secret)
	//update drive classification so that OpenLUKS() will now open this device
	if ok {
		*d.Type = os.DeviceTypeLUKS
	} else {
		d.MarkAsBroken(osi)
	}
}
