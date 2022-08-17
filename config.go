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

	"github.com/sapcc/go-bits/secrets"
	yaml "gopkg.in/yaml.v2"

	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

// Configuration represents the content of the config file.
type Configuration struct {
	ChrootPath string   `yaml:"chroot"`
	DriveGlobs []string `yaml:"drives"`
	Owner      struct {
		User  string `yaml:"user"`
		Group string `yaml:"group"`
	} `yaml:"chown"`
	Keys []struct {
		//this is a struct to later support the addition of a Method field to
		//specify the key derivation method
		Secret secrets.AuthPassword `yaml:"secret"`
	} `yaml:"keys"`
	SwiftIDPool          []string `yaml:"swift-id-pool"`
	MetricsListenAddress string   `yaml:"metrics-listen-address"`
}

// Config is the global Configuration instance that's filled by main() at
// program start.
var Config Configuration

func init() {
	//expect one argument (config file name)
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <config-file>\n", os.Args[0])
		os.Exit(1)
	}

	//read config file
	configBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		util.LogFatal("read configuration file: %s", err.Error())
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		util.LogFatal("parse configuration: %s", err.Error())
	}

	//if there are multiple "spare" entries in the SwiftIDPool, disambiguate
	//them into "spare/0", "spare/1", and so on
	if len(Config.SwiftIDPool) > 0 {
		spareIdx := 0
		for idx, str := range Config.SwiftIDPool {
			if str == "spare" {
				Config.SwiftIDPool[idx] = fmt.Sprintf("spare/%d", spareIdx)
				spareIdx++
			}
		}
	}
}
