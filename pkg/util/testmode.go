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

package util

import (
	"os"
	std_os "os"
	"os/signal"
	"syscall"
	"time"
)

//SetupTestMode performs various setup tasks that are only required for the
//integration tests.
func SetupTestMode() {
	//During integration tests, the autopilot will be killed by SIGPIPE after
	//`logexpect` has seen all the log lines that it wanted (or after it found
	//the first error), but that only happens when a write() syscall is issued
	//on stdout after `logexpect` has exited. This would usually only occur
	//after 30 seconds, with the next "event received: scheduled healthcheck",
	//but we don't want to wait so long. This goroutine will write empty lines
	//to stdout all the time, and `logexpect` will ignore these.
	go func() {
		for {
			time.Sleep(1 * time.Second)
			os.Stdout.Write([]byte("\n"))
		}
	}()

	//This makes sure that SIGPIPE is honored and results in a clean exit.
	//TODO: This could be extended to properly shut down the converger by
	//posting a ShutdownEvent or similar, and could then also be used for
	//SIGINT/SIGTERM.
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGPIPE)
	go func(c <-chan os.Signal) {
		<-c
		os.Exit(0)
	}(c)
}

//StandardTrigger produces a channel that triggers in the specified
//`normalInterval` during productive runs, or whenever the file
//at `testModeTriggerPath` is touched during integration tests.
func StandardTrigger(normalInterval time.Duration, testModeTriggerPath string, atStartup bool) <-chan struct{} {
	trigger := make(chan struct{}, 1)

	if atStartup {
		trigger <- struct{}{}
	}
	if InTestMode() {
		go testTrigger(testModeTriggerPath, trigger)
	} else {
		go prodTrigger(normalInterval, trigger)
	}

	return trigger
}

func prodTrigger(interval time.Duration, trigger chan<- struct{}) {
	for {
		time.Sleep(interval)
		trigger <- struct{}{}
	}
}

func testTrigger(path string, trigger chan<- struct{}) {
	lastModTime := time.Unix(0, 0)
	for {
		time.Sleep(1 * time.Second)

		fi, err := std_os.Stat(path)
		var mtime time.Time
		switch {
		case err == nil:
			mtime = fi.ModTime()
		case std_os.IsNotExist(err):
			mtime = time.Unix(0, 0)
		default:
			LogError(err.Error())
			continue
		}

		if !mtime.Equal(lastModTime) {
			trigger <- struct{}{}
		}
		lastModTime = mtime
	}
}

//GetJobInterval is used by various collector jobs to tighten their work
//schedule during integration tests.
func GetJobInterval(normalInterval, testModeInterval time.Duration) time.Duration {
	if InTestMode() {
		return testModeInterval
	}
	return normalInterval
}

//InTestMode returns true during integration tests.
func InTestMode() bool {
	return os.Getenv("TEST_MODE") == "1"
}
