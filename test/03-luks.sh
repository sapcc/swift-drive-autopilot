#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

make_disk_images  1 2
make_loop_devices 1 2

DEV1="$(readlink -f "${DIR}/loop1")"
DEV2="$(readlink -f "${DIR}/loop2")"

# the test covers the opening of an existing device, and the provisioning of a
# new device; set up the first device as LUKS+XFS with swift-id
if [[ "${DEV1}" != /dev/loop* ]]; then
    # double-check that we won't overwrite a system partition or something
    echo "expected loop device to be called '/dev/loopX', but is actually '${DEV1}'" >&2
    exit 1
fi
echo supersecretpassword | as_root cryptsetup luksFormat --iter-time 100 "${DEV1}"
echo supersecretpassword | as_root cryptsetup luksOpen "${DEV1}" autopilot-test1
as_root mkfs.xfs /dev/mapper/autopilot-test1 >/dev/null
mkdir -p "${DIR}/mount"
as_root mount /dev/mapper/autopilot-test1 "${DIR}/mount"
echo existing | as_root tee "${DIR}/mount/swift-id" > /dev/null
as_root umount "${DIR}/mount"
as_root cryptsetup close autopilot-test1

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
    swift-id-pool: [ new ]
    keys:
        - secret: supersecretpassword
EOF

run_and_expect <<-EOF
> INFO: event received: new device found: ${DIR}/loop1 -> ${DEV1}
> ERROR: cannot determine serial number for ${DEV1}, will use device ID {{hash1}} instead
> INFO: LUKS container at ${DEV1} opened as /dev/mapper/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /run/swift-storage/{{hash1}}
> INFO: event received: new device found: ${DIR}/loop2 -> ${DEV2}
> ERROR: cannot determine serial number for ${DEV2}, will use device ID {{hash2}} instead
> INFO: LUKS container at ${DEV2} opened as /dev/mapper/{{hash2}}
> INFO: mounted /dev/mapper/{{hash2}} to /run/swift-storage/{{hash2}}
> INFO: invalid assignment for ${DEV2} (mounted at /run/swift-storage/{{hash2}}): no swift-id file found on device, will try to assign one
> INFO: assigning swift-id 'new' to ${DEV2}
> INFO: unmounted /run/swift-storage/{{hash1}}
> INFO: mounted /dev/mapper/{{hash1}} to /srv/node/existing
> INFO: unmounted /run/swift-storage/{{hash2}}
> INFO: mounted /dev/mapper/{{hash2}} to /srv/node/new
$ source lib/common.sh; expect_symlink_content "/srv/node/existing/drive-id" "{{hash1}}"; expect_symlink_content "/srv/node/new/drive-id" "{{hash2}}"
EOF

expect_open_luks_count 2
expect_mountpoint /srv/node/existing
expect_mountpoint /srv/node/new

# check that /srv/node/new is really backed up by a LUKS container
MOUNTED_DEVICE="$(awk '$2~/srv\/node\/new/{print$1}' /proc/mounts)"
if [[ "${MOUNTED_DEVICE}" != /dev/mapper/* ]]; then
    echo "expected mountpoint /srv/node/new to be backed by LUKS container, but actually backed by '${MOUNTED_DEVICE}'" >&2
    exit 1
fi
