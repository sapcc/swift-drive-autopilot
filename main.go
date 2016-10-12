package main

import (
	"fmt"
	"io/ioutil"
	"os"

	yaml "gopkg.in/yaml.v2"
)

//Configuration represents the content of the config file.
type Configuration struct {
	ChrootPath string
	DriveGlobs []string `yaml:"drives"`
}

//Config is the global Configuration instance that's filled by main() at
//program start.
var Config Configuration

func main() {
	err := actualMain()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func actualMain() error {
	//expect one argument (config file name)
	if len(os.Args) != 2 {
		return fmt.Errorf("Usage: %s <config-file>\n", os.Args[0])
	}

	//read config file
	configBytes, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		return err
	}

	fmt.Printf("%#v\n", Config)
	return nil
}
