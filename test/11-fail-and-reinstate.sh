#!/bin/bash
set -euo pipefail

THISDIR="$(dirname "$(readlink -f $0)")"
source "${THISDIR}/lib/common.sh"
source "${THISDIR}/lib/cleanup.sh"

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"

# spawn another subshell that will trigger the device failure (simulated by a
# read-only remount) and reinstatement
(
    sleep 5
    as_root mount -o remount,ro /srv/node/swift1
    sleep 7
    reinstate_drive "${DEV1}"
) &

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2, swift3 ]
EOF

run_and_expect <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}}
INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}}
INFO: no swift-id file found on new device ${DEV1} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
INFO: no swift-id file found on new device {{dev2}} (mounted at /run/swift-storage/{{hash2}}), will try to assign one
INFO: assigning swift-id 'swift1' to ${DEV1}
INFO: assigning swift-id 'swift2' to {{dev2}}
INFO: mounted ${DEV1} to /srv/node/swift1
INFO: unmounted /run/swift-storage/{{hash1}}
INFO: mounted {{dev2}} to /srv/node/swift2
INFO: unmounted /run/swift-storage/{{hash2}}

INFO: event received: scheduled consistency check
ERROR: mount of ${DEV1} at /srv/node/swift1 is read-only (could be due to a disk error)
INFO: flagging ${DEV1} as broken because of previous error
INFO: To reinstate this drive into the cluster, delete the symlink at /run/swift-storage/broken/{{hash1}}
INFO: unmounted /srv/node/swift1

INFO: event received: device reinstated: ${DEV1}
INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}}
INFO: mounted ${DEV1} to /srv/node/swift1
INFO: unmounted /run/swift-storage/{{hash1}}
EOF
