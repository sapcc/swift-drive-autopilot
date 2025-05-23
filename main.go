// SPDX-FileCopyrightText: 2016 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"net/http"
	std_os "os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

func main() {
	logg.SetLogger(log.New(std_os.Stdout, log.Prefix(), log.Flags())) // use stdout instead of stderr for backwards-compatibility
	logg.ShowDebug = osext.GetenvBool("DEBUG")

	undoMaxprocs := must.Return(maxprocs.Set(maxprocs.Logger(logg.Debug)))
	defer undoMaxprocs()

	// set working directory to the chroot directory; this simplifies file
	// system operations because we can just use relative paths to refer to
	// stuff inside the chroot
	workingDir := "/"
	if Config.ChrootPath != "" {
		workingDir = Config.ChrootPath
	}
	err := std_os.Chdir(workingDir)
	if err != nil {
		logg.Fatal("chdir to %s: %s", workingDir, err.Error())
	}

	// prepare directories that the converger wants to write to
	command.Command{ExitOnError: true}.Run("mkdir", "-p",
		"/run/swift-storage/broken",
		"/run/swift-storage/state/unmount-propagation",
		"/var/cache/swift",
		"/var/lib/swift-storage/broken",
	)

	// swift cache path must be accesible from user swift
	osi := must.Return(os.NewLinux())
	osi.Chown("/var/cache/swift", Config.Owner.User, Config.Owner.Group)

	// start the metrics endpoint
	if Config.MetricsListenAddress != "" {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			ctx := httpext.ContextWithSIGINT(context.Background(), 1*time.Second)
			must.Succeed(httpext.ListenAndServeContext(ctx, Config.MetricsListenAddress, mux))
		}()
	}

	// start the collectors
	queue := make(chan []Event, 10)
	go CollectDriveEvents(osi, queue)
	go CollectReinstatements(queue)
	go ScheduleWakeups(queue)
	go WatchKernelLog(osi, queue)

	if util.InTestMode() {
		util.SetupTestMode()
	}

	// the converger runs in the main thread
	RunConverger(queue, osi)
}
