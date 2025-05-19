// SPDX-FileCopyrightText: 2016 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/sapcc/go-api-declarations/bininfo"
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
		// this is a struct to later support the addition of a Method field to
		// specify the key derivation method
		Secret secrets.FromEnv `yaml:"secret"`
	} `yaml:"keys"`
	SwiftIDPool          []string `yaml:"swift-id-pool"`
	MetricsListenAddress string   `yaml:"metrics-listen-address"`
}

// Config is the global Configuration instance that's filled by main() at
// program start.
var Config Configuration

func init() {
	bininfo.HandleVersionArgument()

	// expect one argument (config file name)
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <config-file>\n", os.Args[0])
		os.Exit(1)
	}

	// read config file
	configBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		logg.Fatal("read configuration file: %s", err.Error())
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		logg.Fatal("parse configuration: %s", err.Error())
	}

	// if there are multiple "spare" entries in the SwiftIDPool, disambiguate
	// them into "spare/0", "spare/1", and so on
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
