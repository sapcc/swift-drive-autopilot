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
	"time"

	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

// Event is the base interface for messages sent from the collector threads to
// the converger thread.
type Event interface {
	LogMessage() string
	EventType() string //for counter metric that counts events
	Handle(c *Converger)
}

////////////////////////////////////////////////////////////////////////////////
// drive collector

// DriveAddedEvent is an Event that fires when a new drive is found.
type DriveAddedEvent struct {
	DevicePath   string
	FoundAtPath  string //the DevicePath before symlinks were expanded
	SerialNumber string //may be empty if it cannot be determined
}

// LogMessage implements the Event interface.
func (e DriveAddedEvent) LogMessage() string {
	if e.FoundAtPath == "" || e.FoundAtPath == e.DevicePath {
		return "new device found: " + e.DevicePath
	}
	return fmt.Sprintf("new device found: %s -> %s", e.FoundAtPath, e.DevicePath)
}

// EventType implements the Event interface.
func (e DriveAddedEvent) EventType() string {
	return "drive-added"
}

// DriveRemovedEvent is an Event that fires when a drive's device file disappears.
type DriveRemovedEvent struct {
	DevicePath string
}

// LogMessage implements the Event interface.
func (e DriveRemovedEvent) LogMessage() string {
	return "device removed: " + e.DevicePath
}

// EventType implements the Event interface.
func (e DriveRemovedEvent) EventType() string {
	return "drive-removed"
}

// CollectDriveEvents is a collector thread that emits DriveAddedEvent and
// DriveRemovedEvent.
func CollectDriveEvents(osi os.Interface, queue chan []Event) {
	added := make(chan []os.Drive)
	removed := make(chan []string)
	trigger := util.StandardTrigger(5*time.Second, "run/swift-storage/check-drives", true)
	go osi.CollectDrives(Config.DriveGlobs, trigger, added, removed)

	for {
		select {
		case drives := <-added:
			events := make([]Event, len(drives))
			for idx, drive := range drives {
				events[idx] = DriveAddedEvent{
					DevicePath:   drive.DevicePath,
					FoundAtPath:  drive.FoundAtPath,
					SerialNumber: drive.SerialNumber,
				}
			}
			queue <- events
		case devicePaths := <-removed:
			events := make([]Event, len(devicePaths))
			for idx, devicePath := range devicePaths {
				events[idx] = DriveRemovedEvent{DevicePath: devicePath}
			}
			queue <- events
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// reinstatement collector

// DriveReinstatedEvent is an Event that is emitted by CollectReinstatements.
type DriveReinstatedEvent struct {
	DevicePath string
}

// LogMessage implements the Event interface.
func (e DriveReinstatedEvent) LogMessage() string {
	return "device reinstated: " + e.DevicePath
}

// EventType implements the Event interface.
func (e DriveReinstatedEvent) EventType() string {
	return "drive-reinstated"
}

// CollectReinstatements watches /run/swift-storage/broken and
// /var/lib/swift-storage/broken and issues a DriveReinstatedEvent whenever a
// broken-flag in there is deleted by an administrator.
func CollectReinstatements(queue chan []Event) {
	//tracks broken devices between loop iterations; we only send an event when
	//a device is removed from this set
	brokenDevices := make(map[string]bool)

	interval := util.GetJobInterval(5*time.Second, 1*time.Second)
OUTER:
	for {
		var events []Event

		//enumerate broken devices linked in /run/swift-storage/broken
		newBrokenDevices := make(map[string]bool)

		for _, brokenFlagDir := range []string{"run/swift-storage/broken", "var/lib/swift-storage/broken"} {
			success := util.ForeachSymlinkIn(brokenFlagDir,
				func(name, devicePath string) {
					newBrokenDevices[devicePath] = true
				},
			)
			if !success {
				continue OUTER
			}
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

// ScheduleWakeups is a collector job that pushes a no-op event every 30 seconds
// to invoke the consistency checks that the converger executes during each of
// its event loop iterations.
func ScheduleWakeups(queue chan []Event) {
	trigger := util.StandardTrigger(30*time.Second, "run/swift-storage/wakeup", false)
	for range trigger {
		queue <- []Event{WakeupEvent{}}
	}
}

// WakeupEvent is sent by the ScheduleWakeups collector.
type WakeupEvent struct{}

// LogMessage implements the Event interface.
//
// The WakeupEvent does not produce an "event received" log message because it
// would spam the log continuously. Instead, the continued execution of
// consistency checks is tracked by the Prometheus metric that counts events.
func (e WakeupEvent) LogMessage() string {
	if util.InTestMode() {
		return "scheduled consistency check"
	}
	return ""
}

// EventType implements the Event interface.
func (e WakeupEvent) EventType() string {
	return "consistency-check"
}

// Handle implements the Event interface.
func (e WakeupEvent) Handle(c *Converger) {
	//do nothing
}

////////////////////////////////////////////////////////////////////////////////
// kernel log collector

// DriveErrorEvent is emitted by the WatchKernelLog collector.
type DriveErrorEvent struct {
	DevicePath string
	LogLine    string
}

// LogMessage implements the Event interface.
func (e DriveErrorEvent) LogMessage() string {
	return "potential device error for " + e.DevicePath + " seen in kernel log: " + e.LogLine
}

// EventType implements the Event interface.
func (e DriveErrorEvent) EventType() string {
	return "drive-error"
}

// WatchKernelLog is a collector job that sends DriveErrorEvent when the kernel
// log contains an error regarding a SCSI disk.
func WatchKernelLog(osi os.Interface, queue chan []Event) {
	errors := make(chan []os.DriveError)
	go osi.CollectDriveErrors(errors)

	for errs := range errors {
		for _, err := range errs {
			queue <- []Event{DriveErrorEvent{
				DevicePath: err.DevicePath,
				LogLine:    err.Message,
			}}
		}
	}
}
