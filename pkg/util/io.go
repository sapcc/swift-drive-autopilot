/*******************************************************************************
*
* Copyright 2018 SAP SE
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
