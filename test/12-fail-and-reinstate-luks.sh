#!/bin/bash
set -euo pipefail

THISDIR="$(dirname "$(readlink -f $0)")"
source "${THISDIR}/lib/common.sh"
source "${THISDIR}/lib/cleanup.sh"

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"
DEV2="$(readlink -f "${DIR}/loop2")"

# Because it is difficult to estimate the running time of the initial
# luksFormat and luksOpen, perform these steps before spawning the subshell.

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2, swift3 ]
    keys:
        - secret: supersecretpassword
EOF

run_and_expect <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}}
INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
INFO: LUKS container at ${DEV2} opened as /dev/mapper/{{hash2}}
INFO: mounted /dev/mapper/{{hash2}} to /run/swift-storage/{{hash2}}
INFO: no swift-id file found on new device ${DEV1} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
INFO: no swift-id file found on new device ${DEV2} (mounted at /run/swift-storage/{{hash2}}), will try to assign one
INFO: assigning swift-id 'swift1' to ${DEV1}
INFO: assigning swift-id 'swift2' to ${DEV2}
INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1
INFO: unmounted /run/swift-storage/{{hash1}}
INFO: mounted /dev/mapper/{{hash2}} to /srv/node/swift2
INFO: unmounted /run/swift-storage/{{hash2}}
EOF

# spawn another subshell that will trigger the device failure (simulated by a
# read-only remount) and reinstatement
(
    sleep 5
    as_root mount -o remount,ro /srv/node/swift1
    sleep 7
    reinstate_drive "${DEV1}"
) &

run_and_expect <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
INFO: discovered ${DEV1} to be mapped to /dev/mapper/{{hash1}} already
INFO: discovered /dev/mapper/{{hash1}} to be mounted at /srv/node/swift1 already
INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
INFO: discovered ${DEV2} to be mapped to /dev/mapper/{{hash2}} already
INFO: discovered /dev/mapper/{{hash2}} to be mounted at /srv/node/swift2 already

INFO: event received: scheduled consistency check
ERROR: mount of /dev/mapper/{{hash1}} at /srv/node/swift1 is read-only (could be due to a disk error)
INFO: flagging ${DEV1} as broken because of previous error
INFO: To reinstate this drive into the cluster, delete the symlink at /run/swift-storage/broken/{{hash1}}
INFO: unmounted /srv/node/swift1
INFO: LUKS container /dev/mapper/{{hash1}} closed

INFO: event received: device reinstated: ${DEV1}
INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}}
INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1
INFO: unmounted /run/swift-storage/{{hash1}}
EOF
