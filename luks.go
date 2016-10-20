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
