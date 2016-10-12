package main

import (
	"fmt"
	"io/ioutil"
	"os"

	yaml "gopkg.in/yaml.v2"
)

//Config represents the content of the config file.
type Config struct {
	DriveGlobs []string `yaml:"drives"`
}

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
	var config Config
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		return err
	}

	fmt.Printf("%#v\n", config)
	return nil
}
