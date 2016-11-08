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
	"regexp"
	"strings"
)

//OpenLUKS will open a LUKS container on the given drive, and set
//MappedDevicePath accordingly. If the drive is not encrypted with LUKS,
//OpenLUKS returns true without doing anything.
func (d *Drive) OpenLUKS() (success bool) {
	//already done?
	if d.MappedDevicePath != "" {
		return true
	}
	//is the drive encrypted?
	if !d.Classify() {
		return false
	}
	if d.Type != DeviceTypeLUKS {
		return true
	}

	//try each key until one works
	mapperName := d.TemporaryMount.Name
	for idx, key := range Config.Keys {
		Log(LogDebug, "trying to luksOpen %s as %s with key %d...", d.DevicePath, mapperName, idx)
		_, ok := Command{
			Stdin:   key.Secret + "\n",
			SkipLog: true,
		}.Run("cryptsetup", "luksOpen", d.DevicePath, mapperName)
		if ok {
			success = true
			break
		}
	}

	if !success {
		Log(LogError, "exec(cryptsetup luksOpen %s %s) failed: none of the configured keys was accepted")
		return false
	}

	d.MappedDevicePath = "/dev/mapper/" + mapperName
	d.Type = DeviceTypeNotScanned //reset because Classification now refers to what's in the mapped device
	Log(LogDebug, "LUKS container at %s opened as %s", d.DevicePath, d.MappedDevicePath)
	return true
}

//CloseLUKS will close the LUKS container on the given drive, if it exists and
//is currently open.
func (d *Drive) CloseLUKS() (success bool) {
	//anything to do?
	if d.MappedDevicePath == "" {
		return true
	}

	mapperName := filepath.Base(d.MappedDevicePath)
	_, ok := Run("cryptsetup", "close", mapperName)
	if !ok {
		return false
	}

	Log(LogDebug, "LUKS container %s closed", d.MappedDevicePath)
	d.MappedDevicePath = ""
	return true
}

//ScanLUKSMappings checks all mapped devices in /dev/mapper/*, and records them
//as a map of backing device path to mapping name.
func ScanLUKSMappings() (result map[string]string) {
	result = make(map[string]string)
	stdout, _ := Command{ExitOnError: true}.Run("dmsetup", "ls", "--target=crypt")

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
		mapName := fields[0]

		//ask cryptsetup for the device backing this mapping
		backingDevicePath := getBackingDevicePath(mapName)
		result[backingDevicePath] = mapName
	}
	return
}

var backingDeviceRx = regexp.MustCompile(`(?m)^\s*device:\s*(\S+)\s*$`)

//Ask cryptsetup for the device backing an open LUKS container.
func getBackingDevicePath(mapName string) string {
	stdout, _ := Command{ExitOnError: true}.Run("cryptsetup", "status", mapName)

	//look for a line like "  device:  /dev/sdb"
	match := backingDeviceRx.FindStringSubmatch(stdout)
	if match == nil {
		Log(LogFatal, "cannot find backing device for /dev/mapper/%s", mapName)
	}
	return match[1]
}

//CheckLUKS takes the output from ScanLUKSMappings and fills the
//MappedDevicePath of this Drive if it is mapped. False is returned if any
//inconsistencies are found.
func (d *Drive) CheckLUKS(activeMappings map[string]string) (success bool) {
	//TODO: mark drive as broken instead of returning false
	actualMapName := activeMappings[d.DevicePath]

	if actualMapName == "" {
		if d.MappedDevicePath != "" {
			Log(LogError, "LUKS container in %s should be open at %s, but is not",
				d.DevicePath, d.MappedDevicePath,
			)
			return false
		}
		return true
	}

	actualMappedPath := "/dev/mapper/" + actualMapName
	switch d.MappedDevicePath {
	case "":
		//existing mapping is now discovered for the first time -> update Drive struct
		d.MappedDevicePath = actualMappedPath
		return true
	case actualMappedPath:
		//no change
		return true
	default:
		//our internal state tells a different story!
		Log(LogError, "LUKS container in %s should be open at %s, but is actually open at %s",
			d.DevicePath, d.MappedDevicePath, actualMappedPath,
		)
		return false
	}
}

//FormatLUKSIfRequired will create a LUKS container on this device if empty.
func (d *Drive) FormatLUKSIfRequired() (success bool) {
	//we can skip all of this if the LUKS container exists and is mapped already
	if d.MappedDevicePath != "" {
		return true
	}

	//sanity check
	if len(Config.Keys) == 0 {
		Log(LogError, "FormatLUKSIfRequired called on %s, but no keys specified!", d.DevicePath)
		return false
	}

	//is it safe to be formatted? (i.e. don't format when there is already a
	//filesystem or LUKS container)
	if !d.Classify() {
		return false
	}
	if d.Type != DeviceTypeUnknown {
		return true
	}

	//format with the preferred key
	key := Config.Keys[0]
	Log(LogDebug, "running cryptsetup luksFormat %s with key 0...", d.DevicePath)
	_, ok := Command{Stdin: key.Secret + "\n"}.Run("cryptsetup", "luksFormat", d.DevicePath)

	//update drive classification so that OpenLUKS() will now open this device
	if ok {
		d.Type = DeviceTypeLUKS
	}
	return ok
}
