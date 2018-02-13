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
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//Event is the base interface for messages sent from the collector threads to
//the converger thread.
type Event interface {
	LogMessage() string
	EventType() string //for counter metric that counts events
	Handle(c *Converger)
}

////////////////////////////////////////////////////////////////////////////////
// drive collector

//DriveAddedEvent is an Event that fires when a new drive is found.
type DriveAddedEvent struct {
	DevicePath  string
	FoundAtPath string //the DevicePath before symlinks were expanded
}

//LogMessage implements the Event interface.
func (e DriveAddedEvent) LogMessage() string {
	if e.FoundAtPath == "" || e.FoundAtPath == e.DevicePath {
		return "new device found: " + e.DevicePath
	}
	return fmt.Sprintf("new device found: %s -> %s", e.FoundAtPath, e.DevicePath)
}

//EventType implements the Event interface.
func (e DriveAddedEvent) EventType() string {
	return "drive-added"
}

//DriveRemovedEvent is an Event that fires when a drive's device file disappears.
type DriveRemovedEvent struct {
	DevicePath string
}

//LogMessage implements the Event interface.
func (e DriveRemovedEvent) LogMessage() string {
	return "device removed: " + e.DevicePath
}

//EventType implements the Event interface.
func (e DriveRemovedEvent) EventType() string {
	return "drive-removed"
}

//When a drive has a partition table, there will be a line like "Disklabel
//type: gpt" in the output of `sfdisk -l /dev/XXX`. For unpartitioned devices,
//this line is missing.
var driveWithPartitionTableRx = regexp.MustCompile(`(?mi)^Disklabel type`)

//CollectDriveEvents is a collector thread that emits DriveAddedEvent and
//DriveRemovedEvent.
func CollectDriveEvents(queue chan []Event) {
	knownDrives := make(map[string]string)

	//work loop
	interval := util.GetJobInterval(5*time.Second, 1*time.Second)
	for {
		var events []Event

		//expand globs to find drives
		existingDrives := make(map[string]string)
		for _, pattern := range Config.DriveGlobs {
			//make pattern relative to current directory (== chroot directory)
			pattern = strings.TrimPrefix(pattern, "/")

			matches, err := filepath.Glob(pattern)
			if err != nil {
				util.LogFatal("glob(%#v) failed: %s", pattern, err.Error())
			}

			for _, globbedRelPath := range matches {
				//resolve any symlinks to get the actual devicePath (this also makes
				//the path absolute again)
				devicePath, err := EvalSymlinksInChroot(globbedRelPath)
				if err != nil {
					util.LogFatal(err.Error())
				}

				existingDrives["/"+globbedRelPath] = devicePath
			}
		}

		//check if any of the reported drives have been removed
		for globbedPath, devicePath := range knownDrives {
			if _, exists := existingDrives[globbedPath]; !exists {
				events = append(events, DriveRemovedEvent{DevicePath: devicePath})
				delete(knownDrives, globbedPath)
			}
		}

		//handle new drives
		for globbedPath, devicePath := range existingDrives {
			//ignore drives that were already found in a previous run
			if _, exists := knownDrives[globbedPath]; exists {
				continue
			}
			knownDrives[globbedPath] = devicePath

			//ignore devices with partitions
			stdout, _ := command.Command{ExitOnError: false}.Run("sfdisk", "-l", devicePath)
			switch {
			case driveWithPartitionTableRx.MatchString(stdout):
				util.LogInfo("ignoring drive %s because it contains partitions", devicePath)
			case strings.TrimSpace(stdout) == "":
				//if `sfdisk -l` does not print anything at all, then the device is
				//not readable and should be ignored (e.g. on some servers, we have
				///dev/sdX which is a KVM remote volume that's usually not
				//accessible, i.e. open() fails with ENOMEDIUM; we want to ignore those)
				util.LogInfo("ignoring drive %s because it is not readable", devicePath)
			default:
				//drive is eligible -> report it
				events = append(events, DriveAddedEvent{
					DevicePath:  devicePath,
					FoundAtPath: globbedPath,
				})
			}
		}

		//sort events for deterministic behavior in tests
		sort.Slice(events, func(i, j int) bool {
			return events[i].LogMessage() < events[j].LogMessage()
		})

		//wake up the converger thread
		if len(events) > 0 {
			queue <- events
		}

		//sleep for 5 seconds before running globs again
		time.Sleep(interval)
	}
}

////////////////////////////////////////////////////////////////////////////////
// reinstatement collector

//DriveReinstatedEvent is an Event that is emitted by CollectReinstatements.
type DriveReinstatedEvent struct {
	DevicePath string
}

//LogMessage implements the Event interface.
func (e DriveReinstatedEvent) LogMessage() string {
	return "device reinstated: " + e.DevicePath
}

//EventType implements the Event interface.
func (e DriveReinstatedEvent) EventType() string {
	return "drive-reinstated"
}

//CollectReinstatements watches /run/swift-storage/broken and issues a
//DriveReinstatedEvent whenever a broken-flag in there is deleted by an
//administrator.
func CollectReinstatements(queue chan []Event) {
	//tracks broken devices between loop iterations; we only send an event when
	//a device is removed from this set
	brokenDevices := make(map[string]bool)

	interval := util.GetJobInterval(5*time.Second, 1*time.Second)
	for {
		var events []Event

		//enumerate broken devices linked in /run/swift-storage/broken
		newBrokenDevices := make(map[string]bool)

		success := ForeachSymlinkIn("run/swift-storage/broken",
			func(name, devicePath string) {
				newBrokenDevices[devicePath] = true
			},
		)
		if !success {
			continue
		}

		//generate DriveReinstatedEvent for all devices that are not broken anymore
		for devicePath := range brokenDevices {
			if !newBrokenDevices[devicePath] {
				events = append(events, DriveReinstatedEvent{DevicePath: devicePath})
			}
		}
		brokenDevices = newBrokenDevices

		//wake up the converger thread
		if len(events) > 0 {
			queue <- events
		}

		//sleep for 5 seconds before re-running
		time.Sleep(interval)
	}
}

////////////////////////////////////////////////////////////////////////////////
// wakeup scheduler

//ScheduleWakeups is a collector job that pushes a no-op event every 30 seconds
//to invoke the consistency checks that the converger executes during each of
//its event loop iterations.
func ScheduleWakeups(queue chan []Event) {
	interval := util.GetJobInterval(30*time.Second, 10*time.Second)
	for {
		time.Sleep(interval)
		queue <- []Event{WakeupEvent{}}
	}
}

//WakeupEvent is sent by the ScheduleWakeups collector.
type WakeupEvent struct{}

//LogMessage implements the Event interface.
//
//The WakeupEvent does not produce an "event received" log message because it
//would spam the log continuously. Instead, the continued execution of
//consistency checks is tracked by the Prometheus metric that counts events.
func (e WakeupEvent) LogMessage() string {
	if util.InTestMode() {
		return "scheduled consistency check"
	}
	return ""
}

//EventType implements the Event interface.
func (e WakeupEvent) EventType() string {
	return "consistency-check"
}

//Handle implements the Event interface.
func (e WakeupEvent) Handle(c *Converger) {
	//do nothing
}

////////////////////////////////////////////////////////////////////////////////
// kernel log collector

//DriveErrorEvent is emitted by the WatchKernelLog collector.
type DriveErrorEvent struct {
	DevicePath string
	LogLine    string
}

//LogMessage implements the Event interface.
func (e DriveErrorEvent) LogMessage() string {
	return "potential device error for " + e.DevicePath + " seen in kernel log: " + e.LogLine
}

//EventType implements the Event interface.
func (e DriveErrorEvent) EventType() string {
	return "drive-error"
}

var klogErrorRx = regexp.MustCompile(`(?i)\b(?:error|metadata corruption detected|unmount and run xfs_repair)\b`)
var klogDeviceRx = regexp.MustCompile(`\b(sd[a-z]{1,2})\b`)

//WatchKernelLog is a collector job that sends DriveErrorEvent when the kernel
//log contains an error regarding a SCSI disk.
func WatchKernelLog(queue chan []Event) {
	//assemble commandline for journalctl (similar to logic in Command.Run()
	//which we cannot use here because we need a pipe on stdout)
	command := []string{"journalctl", "-kf"}
	if Config.ChrootPath != "" {
		prefix := []string{
			"chroot", Config.ChrootPath,
			"nsenter", "--ipc=/proc/1/ns/ipc", "--",
		}
		command = append(prefix, command...)
	}
	if os.Geteuid() != 0 {
		command = append([]string{"sudo"}, command...)
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		util.LogFatal(err.Error())
	}
	err = cmd.Start()
	if err != nil {
		util.LogFatal(err.Error())
	}

	//wait for a few seconds before starting to read stuff, so that all the
	//DriveAddedEvents have already been sent
	time.Sleep(3 * time.Second)

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			util.LogError(err.Error())
		}
		//NOTE: no special handling of io.EOF here; we will encounter it very
		//frequently anyway while we're waiting for new log lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		//we're looking for log lines with "error" and a disk device name like "sda"
		util.LogDebug("received kernel log line: '%s'", line)
		if !klogErrorRx.MatchString(line) {
			continue
		}
		match := klogDeviceRx.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		event := DriveErrorEvent{
			DevicePath: "/dev/" + match[1],
			LogLine:    line,
		}
		queue <- []Event{event}
	}

	//NOTE: the loop above will never return, so I don't bother with cmd.Wait()
}
