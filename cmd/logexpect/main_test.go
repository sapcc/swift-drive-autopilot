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
	"bytes"
	"testing"
)

func runMatchPatterns(input string, patterns ...string) error {
	return matchPatterns(bytes.NewReader([]byte(input)), patterns)
}

func TestPlainPatterns(t *testing.T) {
	//also tests that trailing input lines are ignored
	input := `hello
		world
		more input than needed
		much more input than needed
	`
	err := runMatchPatterns(input,
		"hello",
		"world",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestTimestampTrimming(t *testing.T) {
	input := `2017/01/05 13:53:31 INFO: hello
		2017/01/05 13:53:32 ERROR: world
	`
	err := runMatchPatterns(input,
		"INFO: hello",
		"ERROR: world",
	)
	if err != nil {
		t.Error(err)
	}
}

func TestVariables(t *testing.T) {
	//check simple variable capture and re-use
	input := `device is keks
		device is really keks
		device is not kuller
	`
	err := runMatchPatterns(input,
		"device is {{dev}}",
		"device is really {{dev}}",
		"device is not {{other}}",
	)
	if err != nil {
		t.Error(err)
	}

	//check that variable reoccurrence with different value fails
	err = runMatchPatterns(input,
		"device is {{dev}}",
		"device is really {{dev}}",
		"device is not {{dev}}",
	)
	if err == nil {
		t.Error("same variable should not match different strings at different times")
	}

	//check multiple occurrence of one variable in the same line
	err = runMatchPatterns("to be or not to be - that is the question",
		"{{issue}} or not {{issue}} - that is the question",
	)
	if err != nil {
		t.Error(err)
	}
}
