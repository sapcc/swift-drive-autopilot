#!/bin/bash

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

if [ -d /run/swift-storage ]; then
    log_debug "Cleanup: /run/swift-storage"
    as_root rm -rf -- /run/swift-storage
fi

if [ -d /srv/node ]; then
    log_debug "Cleanup: /srv/node"
    as_root rm -rf -- /srv/node
fi

log_debug "Cleanup: disk images in ${DIR}"
( cd "${DIR}"; rm -rf -- image? )

