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

package cluster

import (
	std_os "os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
	"github.com/sapcc/swift-drive-autopilot/pkg/os"
	"github.com/sapcc/swift-drive-autopilot/pkg/util"
)

type implementation struct {
	osi              os.Interface //TODO necessary?
	driveStates      map[string]DriveStatus
	driveStatesMutex sync.Mutex
}

func (i *implementation) Initialize() error {
	i.driveStates = make(map[string]DriveStatus)

	//prepare directories that we want to read/write
	command.Command{ExitOnError: true}.Run("mkdir", "-p",
		"/run/swift-storage/broken",
		"/run/swift-storage/state/unmount-propagation",
	)
	return nil
}

//GetDriveState implements the Interface interface.
func (i *implementation) GetDriveStatus(driveID string) DriveStatus {
	i.driveStatesMutex.Lock()
	defer i.driveStatesMutex.Unlock()

	status, exists := i.driveStates[driveID]
	if !exists {
		status = readDriveStatusFromDisk(driveID)
		i.driveStates[driveID] = status
		if status == DriveBroken {
			util.LogInfo("%s was flagged as broken by a previous run of swift-drive-autopilot", driveID)
			flagPath := filepath.Join("/run/swift-storage/broken", driveID)
			util.LogInfo("To reinstate this drive into the cluster, delete the symlink at " + flagPath)
		}
	}
	return status
}

func readDriveStatusFromDisk(driveID string) DriveStatus {
	_, err := std_os.Stat(filepath.Join("run/swift-storage/broken", driveID))
	switch {
	case err == nil:
		return DriveBroken
	case std_os.IsNotExist(err):
		return DriveReady
	default:
		util.LogError(err.Error())
		return DriveBroken
	}
}

//SetDriveStatus implements the Interface interface.
func (i *implementation) SetDriveStatus(driveID string, status DriveStatus, devicePath string) {
	i.driveStatesMutex.Lock()
	defer i.driveStatesMutex.Unlock()

	previousStatus, exists := i.driveStates[driveID]
	if !exists {
		previousStatus = DriveReady
	}
	i.driveStates[driveID] = status

	if previousStatus == status {
		//nothing to do
		return
	}

	//set or clear broken flag file
	flagPath := filepath.Join("/run/swift-storage/broken", driveID)

	switch status {
	case DriveReady:
		command.Run("rm", "-f", "--", flagPath)
	case DriveBroken:
		util.LogInfo("flagging %s as broken because of previous error", devicePath)
		_, ok := command.Run("ln", "-sfT", devicePath, flagPath)
		if ok {
			util.LogInfo("To reinstate this drive into the cluster, delete the symlink at " + flagPath)
		}
	}
}

//CollectDriveStateChanges implements the Interface interface.
func (i *implementation) CollectDriveStateChanges(states chan<- []DriveState) {
	interval := util.GetJobInterval(5*time.Second, 1*time.Second)
	for {
		var changes []DriveState

		//check all devices that we consider broken
		i.driveStatesMutex.Lock()

		for driveID, status := range i.driveStates {
			if status != DriveBroken {
				continue
			}
			if readDriveStatusFromDisk(driveID) == DriveReady {
				//operator wants to reinstate drive
				i.driveStates[driveID] = DriveReady
				changes = append(changes, DriveState{DriveID: driveID, Status: DriveReady})
			}
		}

		i.driveStatesMutex.Unlock()

		//wake up the converger thread
		if len(changes) > 0 {
			states <- changes
		}

		//sleep for 5 seconds before re-running
		time.Sleep(interval)
	}
}

//AnnounceNextGeneration implements the Interface interface.
func (i *implementation) AnnounceNextGeneration(assignments map[string]string) {
	panic("TODO")
}
