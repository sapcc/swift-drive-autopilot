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
	"strings"
	"time"
)

//Event is the base interface for messages sent from the collector threads to
//the converger thread.
type Event interface {
	LogMessage() string
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

//DriveRemovedEvent is an Event that fires when a drive's device file disappears.
type DriveRemovedEvent struct {
	DevicePath string
}

//LogMessage implements the Event interface.
func (e DriveRemovedEvent) LogMessage() string {
	return "device removed: " + e.DevicePath
}

//CollectDriveEvents is a collector thread that emits DriveAddedEvent and
//DriveRemovedEvent.
func CollectDriveEvents(queue chan []Event) {
	reportedEmptyGlob := false
	reportedPartitionedDisk := make(map[string]bool)
	//this map will track which drives we know about (i.e. for which drives we
	//have sent DriveAddedEvent)
	devicePaths := make(map[string]string)

	//work loop
	for {
		var events []Event

		//check if any of the known drives have been removed
		for globbedPath, devicePath := range devicePaths {
			_, err := os.Stat(globbedPath)
			switch {
			case os.IsNotExist(err):
				events = append(events, DriveRemovedEvent{DevicePath: devicePath})
				delete(devicePaths, globbedPath)
			case err != nil:
				Log(LogFatal, "stat(%s) failed: %s", devicePath, err.Error())
			}
		}

		//expand globs to find new drives
		for _, pattern := range Config.DriveGlobs {
			//make pattern relative to current directory (== chroot directory)
			pattern = strings.TrimPrefix(pattern, "/")

			matches, err := filepath.Glob(pattern)
			if err != nil {
				Log(LogFatal, "glob(%#v) failed: %s", pattern, err.Error())
			}
			if len(matches) == 0 {
				//this could hint at a misconfiguration, but could also just
				//mean that all drives are disconnected at the moment
				if !reportedEmptyGlob {
					Log(LogError, "glob(%#v) does not match anything", pattern)
					reportedEmptyGlob = true
				}
			}

			for _, match := range matches {
				//resolve any symlinks to get the actual devicePath
				devicePath, err := filepath.EvalSymlinks(match)
				if err != nil {
					Log(LogFatal, "readlink(%#v) failed: %s", match, err.Error())
				}

				//ignore devices with partitions
				pattern := devicePath + "#"
				submatches, err := filepath.Glob(pattern)
				if err != nil {
					Log(LogFatal, "glob(%#v) failed: %s", pattern, err.Error())
				}
				if len(submatches) != 0 {
					if !strings.HasPrefix(devicePath, "/") {
						devicePath = "/" + devicePath
					}
					if !reportedPartitionedDisk[devicePath] {
						Log(LogInfo, "ignoring drive %s because it contains partitions", devicePath)
						reportedPartitionedDisk[devicePath] = true
					}
					continue
				}

				//make path absolute if necessary (the glob was a relative path!)
				if !strings.HasPrefix(devicePath, "/") {
					devicePath = "/" + devicePath
				}

				//report drive unless it was already found in a previous run
				if devicePaths[match] == "" {
					events = append(events, DriveAddedEvent{
						DevicePath:  devicePath,
						FoundAtPath: "/" + match,
					})
					devicePaths[match] = devicePath
				}
			}
		}

		//wake up the converger thread
		if len(events) > 0 {
			queue <- events
		}

		//sleep for 5 seconds before running globs again
		time.Sleep(5 * time.Second)
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

//CollectReinstatements watches /run/swift-storage/broken and issues a
//DriveReinstatedEvent whenever a broken-flag in there is deleted by an
//administrator.
func CollectReinstatements(queue chan []Event) {
	//tracks broken devices between loop iterations; we only send an event when
	//a device is removed from this set
	brokenDevices := make(map[string]bool)

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
		time.Sleep(5 * time.Second)
	}
}

////////////////////////////////////////////////////////////////////////////////
// wakeup scheduler

//ScheduleWakeups is a collector job that pushes a no-op event every 30 seconds
//to invoke the consistency checks that the converger executes during each of
//its event loop iterations.
func ScheduleWakeups(queue chan []Event) {
	for {
		time.Sleep(30 * time.Second)
		queue <- []Event{WakeupEvent{}}
	}
}

//WakeupEvent is sent by the ScheduleWakeups collector.
type WakeupEvent struct{}

//LogMessage implements the Event interface.
func (e WakeupEvent) LogMessage() string {
	return "scheduled consistency check"
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

var klogErrorRx = regexp.MustCompile(`(?i)\berror\b`)
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
		Log(LogFatal, err.Error())
	}
	err = cmd.Start()
	if err != nil {
		Log(LogFatal, err.Error())
	}

	//wait for a few seconds before starting to read stuff, so that all the
	//DriveAddedEvents have already been sent
	time.Sleep(3 * time.Second)

	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			Log(LogError, err.Error())
		}
		//NOTE: no special handling of io.EOF here; we will encounter it very
		//frequently anyway while we're waiting for new log lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		//we're looking for log lines with "error" and a disk device name like "sda"
		Log(LogDebug, "received kernel log line: '%s'", line)
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
