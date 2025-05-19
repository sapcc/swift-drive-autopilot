// SPDX-FileCopyrightText: 2017 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

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

	// make sure that the count for every event type is reported, even as 0, so
	// that users know which (possibly rare) events can occur
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
