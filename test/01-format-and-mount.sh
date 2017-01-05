#!/bin/bash
set -euo pipefail

THISDIR="$(dirname "$(readlink -f $0)")"
source "${THISDIR}/lib/common.sh"
source "${THISDIR}/lib/cleanup.sh"

make_disk_images  1 2
make_loop_devices 1 2

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

run_and_expect <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
INFO: mounted {{dev1}} to /run/swift-storage/{{id1}}
INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
INFO: mounted {{dev2}} to /run/swift-storage/{{id2}}
ERROR: no swift-id file found on device {{dev1}} (mounted at /run/swift-storage/{{id1}})
ERROR: no swift-id file found on device {{dev2}} (mounted at /run/swift-storage/{{id2}})
EOF
