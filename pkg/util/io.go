// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"os"
	"path/filepath"

	"github.com/sapcc/go-bits/logg"
)

// ForeachSymlinkIn finds all symlinks in the given directory, and calls the
// handler once for each symlink (with its file name and link target). Any
// errors because of filesystem operations will be logged to LogError and false
// will be returned if any such error occurred.
func ForeachSymlinkIn(path string, handler func(name, target string)) (success bool) {
	dir, err := os.Open(path)
	if err != nil {
		logg.Error(err.Error())
		return false
	}
	fis, err := dir.Readdir(-1)
	if err != nil {
		logg.Error(err.Error())
		return false
	}

	success = true
	for _, fi := range fis {
		if (fi.Mode() & os.ModeType) != os.ModeSymlink {
			continue
		}
		linkTarget, err := os.Readlink(filepath.Join(path, fi.Name()))
		if err == nil {
			handler(fi.Name(), linkTarget)
		} else {
			logg.Error(err.Error())
			success = false
		}
	}

	return
}
