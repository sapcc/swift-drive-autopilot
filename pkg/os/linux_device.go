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
	"strings"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
)

//ClassifyDevice implements the Interface interface.
func (l *Linux) ClassifyDevice(devicePath string) DeviceType {
	//ask file(1) to identify the contents of this device
	//BUT: do not run file(1) in the chroot (e.g. CoreOS does not have file(1))
	relDevicePath := strings.TrimPrefix(devicePath, "/")
	desc, ok := command.Command{
		NoChroot: true,
	}.Run("file", "-bLs", relDevicePath)
	if !ok {
		return DeviceTypeUnreadable
	}

	//convert into DeviceType
	switch {
	case strings.HasPrefix(desc, "LUKS encrypted file"):
		return DeviceTypeLUKS
	case strings.Contains(desc, "filesystem data"):
		return DeviceTypeFilesystem
	default:
		return DeviceTypeUnknown
	}
}

//FormatDevice implements the Interface interface.
func (l *Linux) FormatDevice(devicePath string) bool {
	//TODO: remove `-f` (currently needed to work around
	//https://github.com/karelzak/util-linux/issues/1159 until Flatcar updates
	//util-linux to 2.36 or newer
	_, ok := command.Run("mkfs.xfs", "-f", devicePath)
	return ok
}
