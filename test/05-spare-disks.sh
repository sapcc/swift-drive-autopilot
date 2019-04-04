#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images 1 2 3 4 5
make_loop_devices 1 2 3

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ swift1, spare, swift2, spare, swift3 ]
EOF

# What we check here:
# 1. initial setup: swift-id-pool order should be respected for the first 3 drives
# 2. simulate a removal of disk 3 ("swift2")
# 3. simulate a manual assignment of the spare disk to "swift2"
# 4. add more drives: should respect swift-id-pool order again and fill up the reserve instead of assigning/mounting "swift3"
run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in host mount namespace
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop3 -> {{dev3}}
> ERROR: cannot determine serial number for {{dev3}}, will use device ID {{hash3}} instead
> INFO: mounted {{dev3}} to /run/swift-storage/{{hash3}} in host mount namespace
> INFO: mounted {{dev3}} to /run/swift-storage/{{hash3}} in local mount namespace
> INFO: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for {{dev3}} (mounted at /run/swift-storage/{{hash3}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'swift1' to {{dev1}}
> INFO: assigning swift-id 'spare' to {{dev2}}
> INFO: assigning swift-id 'swift2' to {{dev3}}
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift1 in host mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift1 in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash3}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash3}} in local mount namespace
> INFO: mounted {{dev3}} to /srv/node/swift2 in host mount namespace
> INFO: mounted {{dev3}} to /srv/node/swift2 in local mount namespace

$ source lib/common.sh; expect_mountpoint /srv/node/swift{1,2} /run/swift-storage/{{hash2}}; expect_no_mountpoint /srv/node/swift3; expect_deleted /srv/node/spare; expect_file_with_content /run/swift-storage/{{hash2}}/swift-id 'spare'; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check

$ source lib/common.sh; rm ${DIR}/loop3; as_root touch /run/swift-storage/check-drives
> INFO: event received: device removed: {{dev3}}
> INFO: unmounted /srv/node/swift2 in host mount namespace
> INFO: unmounted /srv/node/swift2 in local mount namespace

$ source lib/common.sh; expect_no_mountpoint /srv/node/swift2; echo swift2 | as_root tee /run/swift-storage/{{hash2}}/swift-id > /dev/null; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift2 in host mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift2 in local mount namespace

$ source lib/common.sh; make_loop_devices 4 5; as_root touch /run/swift-storage/check-drives
> INFO: event received: new device found: ${DIR}/loop4 -> {{dev4}}
> ERROR: cannot determine serial number for {{dev4}}, will use device ID {{hash4}} instead
> INFO: mounted {{dev4}} to /run/swift-storage/{{hash4}} in host mount namespace
> INFO: mounted {{dev4}} to /run/swift-storage/{{hash4}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop5 -> {{dev5}}
> ERROR: cannot determine serial number for {{dev5}}, will use device ID {{hash5}} instead
> INFO: mounted {{dev5}} to /run/swift-storage/{{hash5}} in host mount namespace
> INFO: mounted {{dev5}} to /run/swift-storage/{{hash5}} in local mount namespace
> INFO: invalid assignment for {{dev4}} (mounted at /run/swift-storage/{{hash4}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for {{dev5}} (mounted at /run/swift-storage/{{hash5}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'spare' to {{dev4}}
> INFO: assigning swift-id 'spare' to {{dev5}}

$ source lib/common.sh; expect_mountpoint /srv/node/swift{1,2} /run/swift-storage/{{{hash4}},{{hash5}}}; expect_no_mountpoint /srv/node/swift3; expect_file_with_content /run/swift-storage/{{hash4}}/swift-id 'spare'; expect_file_with_content /run/swift-storage/{{hash5}}/swift-id 'spare'; as_root touch /run/swift-storage/wakeup
> INFO: event received: scheduled consistency check
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3
expect_deleted       /srv/node/spare
