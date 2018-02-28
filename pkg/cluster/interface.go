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

import "github.com/sapcc/swift-drive-autopilot/pkg/os"

//Interface describes the set of cluster-level operations that can be executed
//by the autopilot. This interface provides the connection between the
//autopilot on this storage node and the rest of the cluster.
type Interface interface {
	//GetDriveStatus queries the cluster for the last known status of the drive
	//with the given ID.
	GetDriveStatus(driveID string) DriveStatus
	//SetDriveStatus informs the cluster of the current state of a drive.
	//The devicePath is only used to provide useful log messages.
	SetDriveStatus(driveID string, status DriveStatus, devicePath string)
	//CollectDriveStateChanges is run in a separate goroutine and reports drives
	//whose state changes because of outside intervention by the cluster or by
	//human operators.
	CollectDriveStateChanges(states chan<- []DriveState)

	//AnnounceNextGeneration is called by the autopilot when the set of mounted
	//drives has been changed. The argument maps drive IDs to swift IDs, for
	//example assignments["XYZ"] == "swift-01" indicates that the drive with the
	//ID "XYZ" is mounted at /srv/node/swift-01.
	AnnounceNextGeneration(assignments map[string]string)
}

//DriveState is a pair of drive ID and DriveStatus.
type DriveState struct {
	DriveID    string
	DevicePath string
	Status     DriveStatus
}

//DriveStatus enumerates high-level drive states.
type DriveStatus uint

const (
	//DriveReady indicates that the drive is being used, or is ready to be used
	//by the cluster.
	DriveReady DriveStatus = iota
	//DriveBroken indicates that the autopilot has observed an error regarding
	//this drive.
	DriveBroken
	//TODO: add DriveCordoned
)

//InitializeInterface may be called exactly once to obtain the
//cluster.Interface singleton instance.
func InitializeInterface(osi os.Interface) (Interface, error) {
	i := &implementation{osi: osi}
	return i, i.Initialize()
}
