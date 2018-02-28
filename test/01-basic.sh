#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

################################################################################
# phase 1.1: check initial formatting (swift-id auto-assignment is not enabled,
# so this should not perform final mounts)

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}}
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}}
> ERROR: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device
> ERROR: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device
$ source lib/common.sh; expect_symlink_content "/run/swift-storage/{{hash1}}/drive-id" "{{hash1}}"; expect_symlink_content "/run/swift-storage/{{hash2}}/drive-id" "{{hash2}}"
EOF

expect_no_mounts

expect_directories         /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_ownership root:root /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_file_with_content   /run/swift-storage/state/flag-ready ''

################################################################################
# phase 1.2: check idempotency of temporary mount (i.e. autopilot should not
# change existing mounts when restarted)

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: discovered {{dev1}} to be mounted at /run/swift-storage/{{hash1}} already
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /run/swift-storage/{{hash2}} already
> ERROR: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device
> ERROR: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device
$ source lib/common.sh; expect_symlink_content "/run/swift-storage/{{hash1}}/drive-id" "{{hash1}}"; expect_symlink_content "/run/swift-storage/{{hash2}}/drive-id" "{{hash2}}"
EOF

expect_no_mounts

expect_directories         /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_ownership root:root /run/swift-storage/broken /run/swift-storage/state/unmount-propagation /var/cache/swift
expect_file_with_content   /run/swift-storage/state/flag-ready ''

################################################################################
# phase 2.1: assign swift-ids and check final mount

IDX=0
for TEMP_MOUNTPOINT in /run/swift-storage/????????????????????????????????; do
    IDX=$((IDX+1))
    expect_mountpoint "${TEMP_MOUNTPOINT}"
    echo "swift${IDX}" | as_root tee "${TEMP_MOUNTPOINT}/swift-id" >/dev/null
done

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: discovered {{dev1}} to be mounted at /run/swift-storage/{{hash1}} already
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /run/swift-storage/{{hash2}} already
> INFO: unmounted /run/swift-storage/{{hash1}}
> INFO: mounted {{dev1}} to /srv/node/{{id1}}
> INFO: unmounted /run/swift-storage/{{hash2}}
> INFO: mounted {{dev2}} to /srv/node/{{id2}}
$ source lib/common.sh; expect_symlink_content "/srv/node/{{id1}}/drive-id" "{{hash1}}"; expect_symlink_content "/srv/node/{{id2}}/drive-id" "{{hash2}}"
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3 /run/swift-storage/*
expect_ownership     root:root /srv/node/swift1 /srv/node/swift2
expect_open_luks_count 0

################################################################################
# phase 2.2: check idempotency of final mount (i.e. autopilot should not change
# existing mounts when restarted)

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: discovered {{dev1}} to be mounted at /srv/node/{{id1}} already
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /srv/node/{{id2}} already
$ source lib/common.sh; expect_symlink_content "/srv/node/{{id1}}/drive-id" "{{hash1}}"; expect_symlink_content "/srv/node/{{id2}}/drive-id" "{{hash2}}"
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3 /run/swift-storage/*
expect_ownership     root:root /srv/node/swift1 /srv/node/swift2
expect_open_luks_count 0
