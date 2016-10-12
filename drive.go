package main

import "path/filepath"

//ListDrives returns the list of all Swift storage drives, by expanding the
//shell globs in Config.DriveGlobs.
func ListDrives() ([]string, error) {
	var result []string

	for _, pattern := range Config.DriveGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}

	return result, nil
}
