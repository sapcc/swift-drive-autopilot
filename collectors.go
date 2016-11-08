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
	devicePaths := make(map[string]bool)

	//work loop
	for {
		var events []Event

		//check if any of the known drives have been removed
		for devicePath := range devicePaths {
			_, err := os.Stat(devicePath)
			switch {
			case os.IsNotExist(err):
				events = append(events, DriveRemovedEvent{DevicePath: devicePath})
				delete(devicePaths, devicePath)
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

				//make paths absolute if necessary (the glob was a relative path!)
				if !strings.HasPrefix(devicePath, "/") {
					devicePath = "/" + devicePath
				}
				if !strings.HasPrefix(match, "/") {
					match = "/" + match
				}

				//report drive unless it was already found in a previous run
				if !devicePaths[devicePath] {
					events = append(events, DriveAddedEvent{
						DevicePath:  devicePath,
						FoundAtPath: match,
					})
					devicePaths[devicePath] = true
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
// TODO: kernel log collector
