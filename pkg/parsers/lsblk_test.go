// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package parsers

import (
	"os"
	"testing"
)

func TestFindBackingDeviceForLUKS(t *testing.T) {
	testCasesPerFile := map[string]map[string]string{
		"fixtures/lsblk-mpath.json": {
			"BAINGOO2":     "/dev/mapper/mpatha",
			"MI2IA7EL":     "/dev/mapper/mpathak",
			"usr":          "/dev/sda3",
			"DOESNOTEXIST": "",
		},
		"fixtures/lsblk-plain.json": {
			"EJIOQU5P":     "/dev/sdc",
			"XOHSOHW9":     "/dev/sdg",
			"usr":          "/dev/sda3",
			"DOESNOTEXIST": "",
		},
	}
	for fileName, testCases := range testCasesPerFile {
		buf, err := os.ReadFile(fileName)
		if err != nil {
			t.Fatal(err.Error())
		}
		output, err := ParseLsblkOutput(string(buf))
		if err != nil {
			t.Fatal(err.Error())
		}
		for mappingName, expectedDevicePath := range testCases {
			actualDevicePath := emptyIfNil(output.FindBackingDeviceForLUKS(mappingName))
			if actualDevicePath != expectedDevicePath {
				t.Errorf("%s: expected %q to have backing device %q, but has backing device %q",
					fileName, mappingName, expectedDevicePath, actualDevicePath)
			}
		}
	}
}

func TestFindSerialNumberForDevice(t *testing.T) {
	testCasesPerFile := map[string]map[string]string{
		"fixtures/lsblk-mpath.json": {
			"/dev/mapper/mpatha":  "BAINGOO2",
			"/dev/mapper/mpathak": "MI2IA7EL",
			"/dev/sda":            "",
			"/dev/sda3":           "usr",
			"/dev/null":           "",
		},
		"fixtures/lsblk-plain.json": {
			"/dev/sdc":  "EJIOQU5P",
			"/dev/sdg":  "XOHSOHW9",
			"/dev/sda":  "",
			"/dev/sda3": "usr",
			"/dev/null": "",
		},
	}
	for fileName, testCases := range testCasesPerFile {
		buf, err := os.ReadFile(fileName)
		if err != nil {
			t.Fatal(err.Error())
		}
		output, err := ParseLsblkOutput(string(buf))
		if err != nil {
			t.Fatal(err.Error())
		}
		for devicePath, expectedSerialNumber := range testCases {
			actualSerialNumber := emptyIfNil(output.FindSerialNumberForDevice(devicePath))
			if actualSerialNumber != expectedSerialNumber {
				t.Errorf("%s: expected %q to have serial number %q, but has serial number %q",
					fileName, devicePath, expectedSerialNumber, actualSerialNumber)
			}
		}
	}
}

func emptyIfNil(val *string) string {
	if val == nil {
		return ""
	}
	return *val
}
