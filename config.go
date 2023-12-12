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
	"slices"
	"strings"

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/secrets"
	yaml "gopkg.in/yaml.v2"
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
		Secret secrets.FromEnv `yaml:"secret"`
	} `yaml:"keys"`
	// Swift id pools provides functionality to label drives of different types,
	// e.g. nvme, ssd, hdd with different pre and post fix labels
	// These are configured from the yaml and a list of valid labels are generated
	// and placed in the SwiftIDPool []string if not supplied
	SwiftIDPools []struct {
		Type          string   `yaml:"type"`
		Prefix        string   `yaml:"prefix"`  //typically swift
		Postfix       string   `yaml:"postfix"` //typically hdd, ssd, nvme
		Start         int      `yaml:"start"`
		End           int      `yaml:"end"`
		SpareInterval int      `yaml:"spareInterval"` // at what interval spares should be kept
		SwiftIDPool   []string `yaml:"swift-id-pool"`
	} `yaml:"swift-id-pools"`
	MetricsListenAddress string `yaml:"metrics-listen-address"`
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
		logg.Fatal("read configuration file: %s", err.Error())
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		logg.Fatal("parse configuration: %s", err.Error())
	}

	//expand swift-id-pools
	if len(Config.SwiftIDPools) > 0 {
		driveTypes := []string{}
		for idx, driveType := range Config.SwiftIDPools {
			spareIdx := 1

			if !slices.Contains([]string{"hdd", "ssd", "nvme"}, driveType.Type) {
				logg.Error((fmt.Sprintf("Configuration error: Invalid drive type entry %s.", driveType.Type)))
			}
			if slices.Contains(driveTypes, driveType.Type) {
				logg.Fatal((fmt.Sprintf("Configuration error: Duplicate drive type entry for %s.", driveType.Type)))
			}
			driveTypes = append(driveTypes, driveType.Type)
			if driveType.SpareInterval < 0 {
				logg.Fatal("Configuration error: Drive spare interval must be a positive integer.")
			}
			if driveType.Start < 1 || driveType.End < 1 {
				logg.Fatal("Configuration error: Drive index must be a positive integer.")
			}
			if driveType.Start >= driveType.End {
				logg.Fatal("Configuration error: Drive index end must be greater than start.")
			}

			if len(Config.SwiftIDPools[idx].SwiftIDPool) < 1 {
				for i := driveType.Start; i <= driveType.End; i++ {
					poolID := ""
					if driveType.Postfix == "" {
						poolID = fmt.Sprintf("%s-%02d", driveType.Prefix, i)
					} else {
						poolID = fmt.Sprintf("%s-%s-%02d", driveType.Prefix, driveType.Postfix, i)
					}

					Config.SwiftIDPools[idx].SwiftIDPool = append(Config.SwiftIDPools[idx].SwiftIDPool, poolID)

					//if there are multiple "spare" entries in the SwiftIDPool, disambiguate
					//them into "spare/0", "spare/1", and so on
					if driveType.SpareInterval > 0 {
						isDivisibleBySpareInterval := i%driveType.SpareInterval == 0

						if isDivisibleBySpareInterval {
							spareID := ""
							if driveType.Postfix == "" {
								spareID = fmt.Sprintf("spare/%d", spareIdx)
							} else {
								spareID = fmt.Sprintf("spare-%s/%d", driveType.Postfix, spareIdx)
							}
							Config.SwiftIDPools[idx].SwiftIDPool = append(Config.SwiftIDPools[idx].SwiftIDPool, spareID)
							spareIdx++
						}
					}
				}
			} else {
				// Need to check for spare's and update labels
				for idy, str := range Config.SwiftIDPools[idx].SwiftIDPool {
					if strings.Contains(str, "spare") {
						Config.SwiftIDPools[idx].SwiftIDPool[idy] = fmt.Sprintf("%s/%d", str, spareIdx)
						spareIdx++
					}
				}
			}
			//logg.Info(fmt.Sprintf("%+v\n", Config.SwiftIDPools[idx]))
		}
	}
}
