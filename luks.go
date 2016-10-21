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
	"bytes"
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
	if !strings.HasPrefix(d.Classification, "LUKS encrypted file") {
		return true
	}

	//TODO: add a scanning pass to recognize open LUKS devices from a previous run

	//try each key until one works
	mapperName := d.TemporaryMount.Name
	for idx, key := range Config.Keys {
		Log(LogDebug, "trying to luksOpen %s as %s with key %d", d.DevicePath, mapperName, idx)
		_, _, err := Exec(
			ExecChroot, bytes.NewReader([]byte(key.Secret+"\n")),
			"cryptsetup", "luksOpen", d.DevicePath, mapperName,
		)
		if err == nil {
			success = true
			break
		}
	}

	if !success {
		Log(LogError, "exec(cryptsetup luksOpen %s %s) failed: none of the configured keys was accepted")
		return false
	}

	d.MappedDevicePath = "/dev/mapper/" + mapperName
	d.Classification = "" //reset because Classification now refers to what's in the mapped device
	Log(LogDebug, "LUKS container at %s opened as %s", d.DevicePath, d.MappedDevicePath)
	return true
}

//ScanOpenLUKSContainers checks all mapped devices in /dev/mapper/*, and
//records them as MappedDevicePath for their corresponding Drive (if any).
func (drives Drives) ScanOpenLUKSContainers() {
	stdout, err := ExecSimple(ExecChroot, nil, "dmsetup", "ls", "--target=crypt")
	if err != nil {
		Log(LogFatal, "exec(dmsetup ls --target=crypt): %s", err.Error())
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
		for _, drive := range drives {
			//NOTE: do not need to check for duplicates here (i.e. multiple
			//mappings backed by the same device) because device-mapper already
			//forbids that
			if drive.DevicePath == backingDevicePath {
				drive.MappedDevicePath = "/dev/mapper/" + mapName
			}
		}
	}
}

var backingDeviceRx = regexp.MustCompile(`(?m)^\s*device:\s*(\S+)\s*$`)

//Ask cryptsetup for the device backing an open LUKS container.
func getBackingDevicePath(mapName string) string {
	stdout, err := ExecSimple(ExecChroot, nil, "cryptsetup", "status", mapName)
	if err != nil {
		Log(LogFatal, "exec(cryptsetup status %s): %s", mapName, err.Error())
	}

	//look for a line like "  device:  /dev/sdb"
	match := backingDeviceRx.FindStringSubmatch(stdout)
	if match == nil {
		Log(LogFatal, "cannot find backing device for /dev/mapper/%s", mapName)
	}
	return match[1]
}
