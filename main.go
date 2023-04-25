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
	"net/http"
	std_os "os"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

func main() {
	//set working directory to the chroot directory; this simplifies file
	//system operations because we can just use relative paths to refer to
	//stuff inside the chroot
	workingDir := "/"
	if Config.ChrootPath != "" {
		workingDir = Config.ChrootPath
	}
	err := std_os.Chdir(workingDir)
	if err != nil {
		util.LogFatal("chdir to %s: %s", workingDir, err.Error())
	}

	//prepare directories that the converger wants to write to
	command.Command{ExitOnError: true}.Run("mkdir", "-p",
		"/run/swift-storage/broken",
		"/run/swift-storage/state/unmount-propagation",
		"/var/cache/swift",
		"/var/lib/swift-storage/broken",
	)

	//swift cache path must be accesible from user swift
	osi, err := os.NewLinux()
	if err != nil {
		util.LogFatal(err.Error())
	}
	osi.Chown("/var/cache/swift", Config.Owner.User, Config.Owner.Group)

	//start the metrics endpoint
	if Config.MetricsListenAddress != "" {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			util.LogInfo("listening on " + Config.MetricsListenAddress + " for metric shipping")
			err := http.ListenAndServe(Config.MetricsListenAddress, nil) //nolint: gosec // no timeout is required
			if err != nil {
				util.LogFatal("cannot listen on %s for metric shipping: %s", Config.MetricsListenAddress, err.Error())
			}
		}()
	}

	//start the collectors
	queue := make(chan []Event, 10)
	go CollectDriveEvents(osi, queue)
	go CollectReinstatements(queue)
	go ScheduleWakeups(queue)
	go WatchKernelLog(osi, queue)

	if util.InTestMode() {
		util.SetupTestMode()
	}

	//the converger runs in the main thread
	RunConverger(queue, osi)
}
