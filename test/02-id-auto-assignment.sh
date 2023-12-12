#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pools:
    - type: hdd
      prefix: swift
      start: 1
      end: 3
      spareInterval: 2
    - type: ssd
      prefix: swift
      postfix: ssd
      start: 1
      end: 3
      spareInterval: 2
    - type: nvme
      prefix: swift
      postfix: nvme
      start: 1
      end: 3
      spareInterval: 0
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
> INFO: assigning swift-id 'swift-ssd-01' to {{dev1}}
> INFO: assigning swift-id 'swift-ssd-02' to {{dev2}}
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift-ssd-01 in host mount namespace
> INFO: mounted {{dev1}} to /srv/node/swift-ssd-01 in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift-ssd-02 in host mount namespace
> INFO: mounted {{dev2}} to /srv/node/swift-ssd-02 in local mount namespace
EOF

expect_mountpoint    /srv/node/swift-ssd-01 /srv/node/swift-ssd-02
expect_no_mountpoint /srv/node/swift-ssd-03 /run/swift-storage/*
expect_ownership     root:root /srv/node/swift-ssd-01 /srv/node/swift-ssd-02

expect_directories         /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_ownership root:root /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift

expect_file_with_content /run/swift-storage/state/flag-ready ''
expect_file_with_content /srv/node/swift-ssd-01/swift-id           'swift-ssd-01'
expect_file_with_content /srv/node/swift-ssd-02/swift-id           'swift-ssd-02'
