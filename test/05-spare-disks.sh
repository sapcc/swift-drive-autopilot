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
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}}
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}}
> INFO: event received: new device found: ${DIR}/loop3 -> {{dev3}}
> INFO: mounted {{dev3}} to /run/swift-storage/{{hash3}}
> INFO: no swift-id file found on new device {{dev1}} (mounted at /run/swift-storage/{{hash1}}), will try to assign one
> INFO: no swift-id file found on new device {{dev2}} (mounted at /run/swift-storage/{{hash2}}), will try to assign one
> INFO: no swift-id file found on new device {{dev3}} (mounted at /run/swift-storage/{{hash3}}), will try to assign one
> INFO: assigning swift-id 'swift1' to {{dev1}}
> INFO: assigning swift-id 'spare' to {{dev2}}
> INFO: assigning swift-id 'swift2' to {{dev3}}
> INFO: mounted {{dev1}} to /srv/node/swift1
> INFO: unmounted /run/swift-storage/{{hash1}}
> INFO: mounted {{dev3}} to /srv/node/swift2
> INFO: unmounted /run/swift-storage/{{hash3}}

$ source lib/common.sh; expect_mountpoint /srv/node/swift{1,2} /run/swift-storage/{{hash2}}; expect_no_mountpoint /srv/node/swift3; expect_file_with_content /run/swift-storage/{{hash2}}/swift-id 'spare'
> INFO: event received: scheduled consistency check

$ source lib/common.sh; rm ${DIR}/loop3
> INFO: event received: device removed: {{dev3}}
> INFO: unmounted /srv/node/swift2

$ source lib/common.sh; expect_no_mountpoint /srv/node/swift2; echo swift2 | as_root tee /run/swift-storage/{{hash2}}/swift-id > /dev/null
> INFO: event received: scheduled consistency check
> INFO: mounted {{dev2}} to /srv/node/swift2
> INFO: unmounted /run/swift-storage/{{hash2}}

$ source lib/common.sh; make_loop_devices 4 5
> INFO: event received: new device found: ${DIR}/loop4 -> {{dev4}}
> INFO: mounted {{dev4}} to /run/swift-storage/{{hash4}}
> INFO: event received: new device found: ${DIR}/loop5 -> {{dev5}}
> INFO: mounted {{dev5}} to /run/swift-storage/{{hash5}}
> INFO: no swift-id file found on new device {{dev4}} (mounted at /run/swift-storage/{{hash4}}), will try to assign one
> INFO: no swift-id file found on new device {{dev5}} (mounted at /run/swift-storage/{{hash5}}), will try to assign one
> INFO: assigning swift-id 'spare' to {{dev4}}
> INFO: assigning swift-id 'spare' to {{dev5}}

$ source lib/common.sh; expect_mountpoint /srv/node/swift{1,2} /run/swift-storage/{{{hash4}},{{hash5}}}; expect_no_mountpoint /srv/node/swift3; expect_file_with_content /run/swift-storage/{{hash4}}/swift-id 'spare'; expect_file_with_content /run/swift-storage/{{hash5}}/swift-id 'spare'
> INFO: event received: scheduled consistency check
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3 /srv/node/spare
