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
	"io/ioutil"
	sys_os "os"
	"path/filepath"
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//Linux is an Interface implementation for when the autopilot runs in
//productive mode on Linux hosts.
type Linux struct {
	ActiveMountPoints    map[MountScope][]MountPoint
	ActiveLUKSMappings   map[string]string
	MountPropagationMode MountPropagationMode
}

//NewLinux initializes the OS interface for Linux.
func NewLinux() (*Linux, error) {
	mpm, err := detectMountPropagationMode()
	if err != nil {
		return nil, fmt.Errorf("mount propagation detection failed: %s", err.Error())
	}
	if !util.InTestMode() {
		util.LogInfo("mount propagation mode is: " + string(mpm))
	}

	return &Linux{
		MountPropagationMode: mpm,
	}, nil
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

//MountPropagationMode indicates whether this process and processes
//within the chroot have different mount namespaces.
type MountPropagationMode string

const (
	//OneMountNamespace indicates that Config.ChrootPath is not set.
	OneMountNamespace MountPropagationMode = "none"
	//ConnectedMountNamespaces indicates that mounts performed in the host mount
	//namespace (i.e. in the chroot) will automatically appear in the local mount
	//namespace.
	ConnectedMountNamespaces = "connected"
	//SeparateMountNamespaces indicates that mounts performed in the host mount
	//namespace (i.e. in the chroot) will NOT automatically appear in the local
	//mount namespace.
	SeparateMountNamespaces = "separate"
)

func detectMountPropagationMode() (MountPropagationMode, error) {
	chrootPath, _ := sys_os.Getwd()
	chrootPath = filepath.Clean(chrootPath)
	if chrootPath == "/" {
		return OneMountNamespace, nil
	}

	buf, err := ioutil.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}

	//parse mountinfo; format is documented at
	//<https://www.kernel.org/doc/Documentation/filesystems/proc.txt>
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		//find the bind-mount for the chrootPath
		fields := strings.Fields(line)
		if filepath.Clean(fields[4]) != chrootPath {
			continue
		}

		//check the optional fields on the chroot's bind-mount
		for idx := 6; fields[idx] != "-"; idx++ {
			field := fields[idx]
			if strings.HasPrefix(field, "shared:") {
				return ConnectedMountNamespaces, nil
			}
			if strings.HasPrefix(field, "master:") { //indicates (r)slave mount propagation
				return ConnectedMountNamespaces, nil
			}
			if strings.HasPrefix(field, "propagate_from:") {
				return ConnectedMountNamespaces, nil
			}
		}

		//no evidence for connected mount namespaces
		return SeparateMountNamespaces, nil
	}

	return "", fmt.Errorf("could not find mount for %s in /proc/self/mountinfo", chrootPath)
}
