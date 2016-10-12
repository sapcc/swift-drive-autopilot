package main

import (
	"io/ioutil"
	"log"
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
	//expect one argument (config file name)
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <config-file>\n", os.Args[0])
	}

	//read config file
	configBytes, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(configBytes, &Config)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("%#v\n", Config) //DEBUG
}
