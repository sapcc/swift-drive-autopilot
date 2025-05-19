// SPDX-FileCopyrightText: 2016 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"
)

func runMatchCommands(input string, patterns ...string) error {
	return matchCommands(bytes.NewReader([]byte(input)), patterns)
}

func TestPlainPatterns(t *testing.T) {
	// also tests that trailing input lines are ignored
	input := `hello
		world
		more input than needed
		much more input than needed
	`
	err := runMatchCommands(input,
		"> hello",
		"> world",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestTimestampTrimming(t *testing.T) {
	input := `2017/01/05 13:53:31 INFO: hello
		2017/01/05 13:53:32 ERROR: world
	`
	err := runMatchCommands(input,
		"> INFO: hello",
		"> ERROR: world",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestVariables(t *testing.T) {
	// check simple variable capture and re-use
	input := `device is keks
		device is really keks
		device is not kuller
	`
	err := runMatchCommands(input,
		"> device is {{dev}}",
		"> device is really {{dev}}",
		"> device is not {{other}}",
	)
	if err != nil {
		t.Error(err)
	}

	// check that variable reoccurrence with different value fails
	err = runMatchCommands(input,
		"> device is {{dev}}",
		"> device is really {{dev}}",
		"> device is not {{dev}}",
	)
	if err == nil {
		t.Error("same variable should not match different strings at different times")
	}

	// check multiple occurrence of one variable in the same line
	err = runMatchCommands("to be or not to be - that is the question",
		"> {{issue}} or not {{issue}} - that is the question",
	)
	if err != nil {
		t.Error(err)
	}
}
