#!/bin/bash
set -euo pipefail

THISDIR="$(dirname "$(readlink -f $0)")"

function as_root {
    if [ "${EUID}" == 0 ]; then
        "$@"
    else
        sudo "$@"
    fi
}

function log_debug {
    if [ "${DEBUG:-0}" == 1 ]; then
        echo ">> $@" >&2
    fi
}

# choose temporary directory (this is the same for all test-runs, so that a
# subsequent run can cleanup the assets from the previous one)
DIR="${TMPDIR:-/tmp}/swift-drive-autopilot-test"
log_debug "Test assets will be placed below ${DIR}"
mkdir -p "${DIR}"

################################################################################
# cleanup phase

if mount | grep -qE 'on (/run/swift-storage|/srv/node)/'; then
    mount | grep -E 'on (/run/swift-storage|/srv/node)/' | cut -d' ' -f3 | while read MOUNTPOINT; do
        log_debug "Cleanup: mountpoint ${MOUNTPOINT}"
        as_root umount "${MOUNTPOINT}"
    done
fi

if losetup | grep -qF "${DIR}"; then
    losetup | grep -F "${DIR}" | cut -d' ' -f1 | while read DEVICE; do
        log_debug "Cleanup: loop device ${DEVICE}"
        as_root losetup -d "${DEVICE}"
    done
fi

log_debug "Cleanup: disk images at ${DIR}/image?"
( cd "${DIR}"; rm -rf -- image? )

log_debug "Cleanup: /run/swift-storage"
as_root rm -rf -- /run/swift-storage

log_debug "Cleanup: /srv/node"
as_root rm -rf -- /srv/node

################################################################################
# setup phase

log_debug "Allocating disk images"
fallocate -l 100M "${DIR}/image1"
fallocate -l 100M "${DIR}/image2"

log_debug "Preparing loop devices"
ln -sfT "$(as_root losetup --find --show "${DIR}/image1")" "${DIR}/loop1"
ln -sfT "$(as_root losetup --find --show "${DIR}/image2")" "${DIR}/loop2"

log_debug "Preparing autopilot config"
cat > "${DIR}/config.yaml" <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

cat > "${DIR}/pattern" <<-EOF
INFO: event received: new device found: ${DIR}/loop1 -> {{dev1}}
INFO: mounted {{dev1}} to /run/swift-storage/{{id1}}
INFO: event received: new device found: ${DIR}/loop2 -> {{dev2}}
INFO: mounted {{dev2}} to /run/swift-storage/{{id2}}
ERROR: no swift-id file found on device {{dev1}} (mounted at /run/swift-storage/{{id1}})
ERROR: no swift-id file found on device {{dev2}} (mounted at /run/swift-storage/{{id2}})
EOF

################################################################################
# run phase

log_debug "Starting autopilot (log output will be copied to ${DIR}/log)"
as_root "${THISDIR}/../swift-drive-autopilot" "${DIR}/config.yaml" 2>&1 | tee "${DIR}/log" | "${THISDIR}/logexpect" "${DIR}/pattern"
log_debug "Success!"
