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
	"fmt"
	"os"
	"path/filepath"
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

		dir, err := os.Open("run/swift-storage/broken")
		if err != nil {
			Log(LogError, err.Error())
			continue
		}
		fis, err := dir.Readdir(-1)
		if err != nil {
			Log(LogError, err.Error())
			continue
		}

		failed := false
		for _, fi := range fis {
			if (fi.Mode() & os.ModeType) != os.ModeSymlink {
				continue
			}
			devicePath, err := os.Readlink("run/swift-storage/broken/" + fi.Name())
			if err != nil {
				Log(LogError, err.Error())
				failed = true
				break
			}
			newBrokenDevices[devicePath] = true
		}
		if failed {
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
// TODO: kernel log collector
