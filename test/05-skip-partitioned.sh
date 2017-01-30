#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"

# create a partition table on loop1 -- this should cause the autopilot to skip
# this device (in an effort to protect the OS installation)
sed -e 's/\s*\([^#][\S]*\).*/\1/' <<-EOF | as_root fdisk "${DEV1}"
    o   # create new partition table
    n   # new partition
        # default - primary partition
        # default - partition number 1
        # default - start at beginning of disk
        # default - extend partition to end of disk
    w   # write partition table
    q   # done
EOF

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, swift2, swift3 ]
EOF

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev1}}
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}}
> INFO: no swift-id file found on new device {{dev1}} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
> INFO: assigning swift-id 'swift1' to {{dev1}}
> INFO: mounted {{dev1}} to /srv/node/swift1
> INFO: unmounted /run/swift-storage/{{hash1}}
EOF

expect_mountpoint    /srv/node/swift1
expect_no_mountpoint /srv/node/swift2
