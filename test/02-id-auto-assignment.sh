#!/bin/bash
set -euo pipefail

THISDIR="$(dirname "$(readlink -f $0)")"
source "${THISDIR}/lib/common.sh"
source "${THISDIR}/lib/cleanup.sh"

make_disk_images  1 2
make_loop_devices 1 2

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2 ]
EOF

run_and_expect <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}}
INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}}
INFO: no swift-id file found on new device {{dev1}} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
INFO: no swift-id file found on new device {{dev2}} (mounted at /run/swift-storage/{{hash2}}), will try to assign one
INFO: assigning swift-id 'swift1' to {{dev1}}
INFO: assigning swift-id 'swift2' to {{dev2}}
INFO: mounted {{dev1}} to /srv/node/{{id1}}
INFO: unmounted /run/swift-storage/{{hash1}}
INFO: mounted {{dev2}} to /srv/node/{{id2}}
INFO: unmounted /run/swift-storage/{{hash2}}
EOF

expect_mountpoint    /srv/node/swift1
expect_mountpoint    /srv/node/swift2
expect_no_mountpoint /srv/node/swift3
