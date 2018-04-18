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
	"fmt"
	"path/filepath"
	"strings"
)

//Linux is an Interface implementation for when the autopilot runs in
//productive mode on Linux hosts.
type Linux struct {
	ActiveMountPoints  map[MountScope][]MountPoint
	ActiveLUKSMappings map[string]string
}

//evalSymlinksInChroot is like filepath.EvalSymlinks(), but considers that the
//given path is inside the chroot directory.
func (l *Linux) evalSymlinksInChroot(path string) (string, error) {
	//make path relative to current directory (== chroot directory)
	path = strings.TrimPrefix(path, "/")

	result, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("readlink(%#v) failed: %s", filepath.Join("/", path), err.Error())
	}

	//make path absolute again
	if !strings.HasPrefix(result, "/") {
		result = "/" + result
	}
	return result, nil
}
