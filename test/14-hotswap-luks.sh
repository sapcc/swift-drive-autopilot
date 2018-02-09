#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
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

# Hotswapping would mean that the device file of the drive disappears. We
# cannot reasonably simulate that, so we use symlinks to device files instead
# and delete the symlink to simulate device removal.

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}}
> INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
> ERROR: cannot determine serial number for ${DEV2}, will use device ID {{hash2}} instead
> INFO: LUKS container at ${DEV2} opened as /dev/mapper/{{hash2}}
> INFO: mounted /dev/mapper/{{hash2}} to /run/swift-storage/{{hash2}}
> INFO: no swift-id file found on new device ${DEV1} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
> INFO: no swift-id file found on new device ${DEV2} (mounted at /run/swift-storage/{{hash2}}), will try to assign one
> INFO: assigning swift-id 'swift1' to ${DEV1}
> INFO: assigning swift-id 'swift2' to ${DEV2}
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1
> INFO: unmounted /run/swift-storage/{{hash1}}
> INFO: mounted /dev/mapper/{{hash2}} to /srv/node/swift2
> INFO: unmounted /run/swift-storage/{{hash2}}
> INFO: event received: scheduled consistency check

$ source lib/common.sh; expect_open_luks_count 2; expect_mountpoint /srv/node/swift{1,2}; rm "${DIR}/loop1"
> INFO: event received: device removed: ${DEV1}
> INFO: unmounted /srv/node/swift1
> INFO: LUKS container /dev/mapper/{{hash1}} closed

$ source lib/common.sh; expect_open_luks_count 1; expect_no_mountpoint /srv/node/swift1; ln -s "${DEV1}" "${DIR}/loop1"
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/swift1
> INFO: unmounted /run/swift-storage/{{hash1}}
EOF

expect_open_luks_count 2
expect_mountpoint /srv/node/swift{1,2}
