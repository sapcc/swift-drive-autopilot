/*******************************************************************************
*
* Copyright 2017 SAP SE
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

import "github.com/prometheus/client_golang/prometheus"

var eventCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "swift_drive_autopilot_events",
		Help: "Counts drive events handled and consistency checks performed.",
	},
	[]string{"type"},
)

func init() {
	prometheus.MustRegister(eventCounter)

	//make sure that the count for every event type is reported, even as 0, so
	//that users know which (possibly rare) events can occur
	events := []Event{
		DriveAddedEvent{},
		DriveRemovedEvent{},
		DriveReinstatedEvent{},
		DriveErrorEvent{},
		WakeupEvent{},
	}
	for _, event := range events {
		eventCounter.With(prometheus.Labels{"type": event.EventType()}).Add(0)
	}
}
