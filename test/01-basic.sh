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

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

################################################################################
# phase 1.1: check initial formatting (swift-id auto-assignment is not enabled,
# so this should not perform final mounts)

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
> ERROR: cannot determine serial number for {{dev1}}, will use device ID {{hash1}} instead
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in host mount namespace
> INFO: mounted {{dev1}} to /run/swift-storage/{{hash1}} in local mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in host mount namespace
> INFO: mounted {{dev2}} to /run/swift-storage/{{hash2}} in local mount namespace
> ERROR: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device
> ERROR: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device
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
> INFO: discovered {{dev1}} to be mounted at /run/swift-storage/{{hash1}} already in host mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /run/swift-storage/{{hash2}} already in host mount namespace
> ERROR: invalid assignment for {{dev1}} (mounted at /run/swift-storage/{{hash1}}): no swift-id file found on device
> ERROR: invalid assignment for {{dev2}} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device
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
> INFO: discovered {{dev1}} to be mounted at /run/swift-storage/{{hash1}} already in host mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /run/swift-storage/{{hash2}} already in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash1}} in local mount namespace
> INFO: mounted {{dev1}} to /srv/node/{{id1}} in host mount namespace
> INFO: mounted {{dev1}} to /srv/node/{{id1}} in local mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in host mount namespace
> INFO: unmounted /run/swift-storage/{{hash2}} in local mount namespace
> INFO: mounted {{dev2}} to /srv/node/{{id2}} in host mount namespace
> INFO: mounted {{dev2}} to /srv/node/{{id2}} in local mount namespace
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
> INFO: discovered {{dev1}} to be mounted at /srv/node/{{id1}} already in host mount namespace
> INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
> ERROR: cannot determine serial number for {{dev2}}, will use device ID {{hash2}} instead
> INFO: discovered {{dev2}} to be mounted at /srv/node/{{id2}} already in host mount namespace
EOF

expect_mountpoint    /srv/node/swift1 /srv/node/swift2
expect_no_mountpoint /srv/node/swift3 /run/swift-storage/*
expect_ownership     root:root /srv/node/swift1 /srv/node/swift2
expect_open_luks_count 0
