package main

import (
	"io/ioutil"
	"log"
	"os"

	yaml "gopkg.in/yaml.v2"
)

//Configuration represents the content of the config file.
type Configuration struct {
	ChrootPath string   `yaml:"chroot"`
	DriveGlobs []string `yaml:"drives"`
}

//Config is the global Configuration instance that's filled by main() at
//program start.
var Config Configuration

func main() {
	//expect one argument (config file name)
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <config-file>\n", os.Args[0])
	}

	//read config file
	configBytes, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Cannot read configuration file: %s", err.Error())
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		log.Fatalf("Cannot parse configuration: %s", err.Error())
	}

	//TODO: execute everything after this point in a loop

	//list drives
	drivePaths, err := ListDrives()
	if err != nil {
		log.Fatalf("Cannot list drives: %s", err.Error())
	}
	log.Printf("DEBUG: drivePaths = %#v\n", drivePaths)

	//look for existing mount points
	allMounts, err := ScanMountPoints()
	if err != nil {
		log.Fatalf("Cannot list mount points: %s", err.Error())
	}
	log.Printf("DEBUG: allMounts = %#v\n", allMounts)

	//try to mount all drives (if not yet mounted)
	failed := false
	var mountPaths []string
	for _, drivePath := range drivePaths {
		mountPath, err := MountDevice(drivePath, allMounts)
		if err == nil {
			mountPaths = append(mountPaths, mountPath)
		} else {
			log.Println(err.Error())
			failed = true
		}
	}

	log.Printf("DEBUG: mountPaths = %#v\n", mountPaths)

	//signal intermittent failures to the caller
	if failed {
		os.Exit(1)
	}
}
