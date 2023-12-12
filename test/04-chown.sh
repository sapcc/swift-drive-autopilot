#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pools:
    - type: ssd
      prefix: swift
      postfix: ssd
      start: 1
      end: 3
      spareInterval: 2
      swift-id-pool: [ swift1, swift2, swift3 ]
    chown:
      user: nobody
      group: users
EOF

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in host mount namespace
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in local mount namespace
> INFO: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device, will try to assign one
> INFO: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'swift1' to {{dev1}}
> INFO: assigning swift-id 'swift2' to {{dev2}}
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift1 in host mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift1 in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift2 in host mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift2 in local mount namespace
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3 /run/swift-storage/*
expect_ownership     nobody:users /srv/node/swift1 /srv/node/swift2

expect_directories         /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_ownership root:root /run/swift-storage/broken /run/swift-storage/state/unmount-propagation
expect_ownership nobody:users /var/cache/swift

expect_file_with_content /run/swift-storage/state/flag-ready ''
expect_file_with_content /srv/node/swift1/swift-id           'swift1'
expect_file_with_content /srv/node/swift2/swift-id           'swift2'
