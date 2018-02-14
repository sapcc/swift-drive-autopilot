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
	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

//CreateLUKSContainer implements the Interface interface.
func (l *Linux) CreateLUKSContainer(devicePath, key string) bool {
	_, ok := command.Command{Stdin: key + "\n"}.Run("cryptsetup", "luksFormat", devicePath)
	return ok
}

//OpenLUKSContainer implements the Interface interface.
func (l *Linux) OpenLUKSContainer(devicePath, mappingName string, keys []string) (string, bool) {
	//try each key until one works
	for idx, key := range keys {
		util.LogDebug("trying to luksOpen %s as %s with key %d...", devicePath, mappingName, idx)
		_, ok := command.Command{
			Stdin:   key + "\n",
			SkipLog: true,
		}.Run("cryptsetup", "luksOpen", devicePath, mappingName)
		if ok {
			return "/dev/mapper/" + mappingName, true
		}
	}

	//no key worked
	return "", false
}

//CloseLUKSContainer implements the Interface interface.
func (l *Linux) CloseLUKSContainer(mappingName string) bool {
	_, ok := command.Run("cryptsetup", "close", mappingName)
	return ok
}
