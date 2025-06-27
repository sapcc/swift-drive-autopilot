// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package os

import (
	"regexp"
	"strings"

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/parsers"
)

// CreateLUKSContainer implements the Interface interface.
func (l *Linux) CreateLUKSContainer(devicePath, key string) bool {
	_, ok := command.Command{Stdin: key + "\n"}.Run("cryptsetup", "luksFormat", devicePath)
	return ok
}

// OpenLUKSContainer implements the Interface interface.
func (l *Linux) OpenLUKSContainer(devicePath, mappingName string, keys []string) (string, bool) {
	// try each key until one works
	for idx, key := range keys {
		logg.Debug("trying to luksOpen %s as %s with key %d...", devicePath, mappingName, idx)
		_, ok := command.Command{
			Stdin:   key + "\n",
			SkipLog: true,
		}.Run("cryptsetup", "luksOpen", devicePath, mappingName)
		if ok {
			mappedDevicePath := "/dev/mapper/" + mappingName
			// remember this mapping
			if l.ActiveLUKSMappings == nil {
				l.ActiveLUKSMappings = make(map[string]string)
			}
			l.ActiveLUKSMappings[devicePath] = mappedDevicePath
			return mappedDevicePath, true
		}
	}

	// no key worked
	return "", false
}

// CloseLUKSContainer implements the Interface interface.
func (l *Linux) CloseLUKSContainer(mappingName string) bool {
	_, ok := command.Run("cryptsetup", "close", mappingName)
	return ok
}

// RefreshLUKSMappings implements the Interface interface.
func (l *Linux) RefreshLUKSMappings() {
	stdout, _ := command.Command{ExitOnError: true}.Run("lsblk", "-J")
	lsblkOutput, err := parsers.ParseLsblkOutput(stdout)
	if err != nil {
		logg.Fatal("cannot parse `lsblk -J` output: " + err.Error())
	}

	l.ActiveLUKSMappings = make(map[string]string)
	stdout, _ = command.Command{ExitOnError: true}.Run("dmsetup", "ls", "--target=crypt")

	if strings.TrimSpace(stdout) == "No devices found" {
		return
	}

	for line := range strings.SplitSeq(stdout, "\n") {
		// each output line describes a mapping and looks like
		// "mapname\t(devmajor, devminor)"; extract the mapping names
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		mappingName := fields[0]

		// check lsblk output for the device backing this mapping, otherwise ask cryptsetup for it
		backingDevicePath := lsblkOutput.FindBackingDeviceForLUKS(mappingName)
		if backingDevicePath == nil {
			backingDevicePath = l.getBackingDevicePath(mappingName)
		}
		if backingDevicePath != nil {
			l.ActiveLUKSMappings[*backingDevicePath] = "/dev/mapper/" + mappingName

			// if `backingDevicePath` is a symlink (e.g. `/dev/mapper/mpathXXX` for
			// multipath devices), callers may also ask us for the underlying device
			// file (e.g. `/dev/dm-NNN`) instead of the symlink, so track that file as well
			backingDeviceCanonicalPath, err := l.evalSymlinksInChroot(*backingDevicePath)
			if err != nil {
				logg.Fatal("while resolving symlinks in %s: %s", *backingDevicePath, err.Error())
			}
			l.ActiveLUKSMappings[backingDeviceCanonicalPath] = "/dev/mapper/" + mappingName
		}
	}
}

var backingDeviceRx = regexp.MustCompile(`(?m)^\s*device:\s*(\S+)\s*$`)

// Ask cryptsetup for the device backing an open LUKS container.
func (l *Linux) getBackingDevicePath(mapName string) *string {
	stdout, _ := command.Command{ExitOnError: true}.Run("cryptsetup", "status", mapName)

	// look for a line like "  device:  /dev/sdb"
	match := backingDeviceRx.FindStringSubmatch(stdout)
	switch {
	case match == nil:
		logg.Fatal("cannot find backing device for /dev/mapper/%s", mapName)
	case match[1] == "(null)":
		logg.Error("skipping /dev/mapper/%s: `cryptsetup status` reports backing device as `(null)` (the kernel log probably has an IO error for the underlying device)", mapName)
		return nil
	default:
		// resolve any symlinks to get the actual devicePath
		// when the luks container is created on top of multipathing, cryptsetup status might report the /dev/mapper/mpath device
		// also the luksFormat was called on actual device
		devicePath := must.Return(l.evalSymlinksInChroot(match[1]))
		if devicePath != match[1] {
			logg.Debug("backing device path for %s is %s -> %s", mapName, match[1], devicePath)
			return &devicePath
		}
		logg.Debug("backing device path for %s is %s", mapName, match[1])
	}
	return &match[1]
}

// GetLUKSMappingOf implements the Interface interface.
func (l *Linux) GetLUKSMappingOf(devicePath string) string {
	logg.Debug("discovered LUKS device path for %s is %q", devicePath, l.ActiveLUKSMappings[devicePath])
	return l.ActiveLUKSMappings[devicePath]
}
