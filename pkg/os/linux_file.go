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

package os

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

//ReadSwiftID implements the Interface interface.
func (l *Linux) ReadSwiftID(mountPath string) (string, error) {
	buf, err := ioutil.ReadFile(swiftIDPathIn(mountPath))
	switch {
	case err == nil:
		return strings.TrimSpace(string(buf)), nil
	case os.IsNotExist(err): //not an error
		return "", nil
	default:
		return "", err
	}
}

//WriteSwiftID implements the Interface interface.
func (l *Linux) WriteSwiftID(mountPath, swiftID string) error {
	return ioutil.WriteFile(swiftIDPathIn(mountPath), []byte(swiftID+"\n"), 0644)
}

func swiftIDPathIn(mountPath string) string {
	path := filepath.Join(mountPath, "swift-id")
	//make path relative to working directory to account for chrootPath
	return strings.TrimPrefix(path, "/")
}
