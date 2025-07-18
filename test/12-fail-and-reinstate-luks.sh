#!/bin/bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

cd "$(dirname "$(readlink -f "$0")")" || exit 1
# shellcheck source=./test/lib/common.sh
source ./lib/common.sh
# shellcheck source=./test/lib/cleanup.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"
DEV2="$(readlink -f "${DIR}/loop2")"

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2, swift3 ]
    keys:
        - secret: supersecretpassword
EOF

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
> ERROR: cannot determine serial number for ${DEV2}, will use device ID {{hash2}} instead
> INFO: LUKS container at ${DEV2} opened as /dev/mapper/{{hash2}}
> INFO: mounted /dev/mapper/{{hash2}} to /run/swift-storage/{{hash2}} in host mount namespace
> INFO: mounted /dev/mapper/{{hash2}} to /run/swift-storage/{{hash2}} in local mount namespace
> INFO: invalid assignment for ${DEV1} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for ${DEV2} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'swift1' to ${DEV1}
> INFO: assigning swift-id 'swift2' to ${DEV2}
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1 in host mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1 in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted /dev/mapper/{{hash2}} to /srv/node/swift2 in host mount namespace
> INFO: mounted /dev/mapper/{{hash2}} to /srv/node/swift2 in local mount namespace

$ source lib/common.sh; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check

$ source lib/common.sh; expect_open_luks_count 2; expect_mountpoint /srv/node/swift{1,2}; as_root mount -o remount,ro /srv/node/swift1; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check
> ERROR: mount of /dev/mapper/{{hash1}} at /srv/node/swift1 is read-only in host mount namespace (could be due to a disk error)
> INFO: flagging ${DEV1} as broken because of previous error
> INFO: To reinstate this drive into the cluster, delete the symlink at /run/swift-storage/broken/{{hash1}}
> INFO: unmounted /srv/node/swift1 in host mount namespace
> INFO: unmounted /srv/node/swift1 in local mount namespace
> INFO: LUKS container /dev/mapper/{{hash1}} closed

$ source lib/common.sh; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check

$ source lib/common.sh; expect_open_luks_count 1; expect_no_mountpoint /srv/node/swift1; expect_symlink /run/swift-storage/broken/* "${DEV1}"; expect_symlink /run/swift-storage/state/unmount-propagation/swift1 "${DEV1}"; reinstate_drive "${DEV1}"
> INFO: event received: device reinstated: ${DEV1}
> INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1 in host mount namespace
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1 in local mount namespace
EOF

expect_open_luks_count 2
expect_mountpoint /srv/node/swift{1,2}
expect_deleted    /run/swift-storage/broken/* /run/swift-storage/state/unmount-propagation/*
