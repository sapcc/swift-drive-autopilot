#!/bin/bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

cd "$(dirname "$(readlink -f "$0")")" || exit 1
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"
DEV2="$(readlink -f "${DIR}/loop2")"

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2, swift3 ]
EOF

# Hotswapping would mean that the device file of the drive disappears. We
# cannot reasonably simulate that, so we use symlinks to device files instead
# and delete the symlink to simulate device removal.

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
> ERROR: cannot determine serial number for ${DEV2}, will use device ID {{hash2}} instead
> INFO: mounted ${DEV2} to /run/swift-storage/{{hash2}} in host mount namespace
> INFO: mounted ${DEV2} to /run/swift-storage/{{hash2}} in local mount namespace
> INFO: invalid assignment for ${DEV1} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for ${DEV2} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'swift1' to ${DEV1}
> INFO: assigning swift-id 'swift2' to ${DEV2}
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted ${DEV1} to /srv/node/swift1 in host mount namespace
> INFO: mounted ${DEV1} to /srv/node/swift1 in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted ${DEV2} to /srv/node/swift2 in host mount namespace
> INFO: mounted ${DEV2} to /srv/node/swift2 in local mount namespace

$ source lib/common.sh; expect_mountpoint /srv/node/swift{1,2}; rm "${DIR}/loop1"; as_root touch /run/swift-storage/check-drives
> INFO: event received: device removed: ${DEV1}
> INFO: unmounted /srv/node/swift1 in host mount namespace
> INFO: unmounted /srv/node/swift1 in local mount namespace

$ source lib/common.sh; expect_no_mountpoint /srv/node/swift1; ln -s "${DEV1}" "${DIR}/loop1"; as_root touch /run/swift-storage/check-drives
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted ${DEV1} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted ${DEV1} to /srv/node/swift1 in host mount namespace
> INFO: mounted ${DEV1} to /srv/node/swift1 in local mount namespace
EOF

expect_mountpoint /srv/node/swift{1,2}
