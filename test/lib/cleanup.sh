#!/bin/bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

if mount | grep -qE "on (/run/swift-storage|/srv/node|${DIR})/"; then
    for MOUNTPOINT in $(mount | grep -E "on (/run/swift-storage|/srv/node|${DIR})/" | cut -d' ' -f3); do
        log_debug "Cleanup: mountpoint ${MOUNTPOINT}"
        as_root umount "${MOUNTPOINT}"
    done
fi

# device names starting with "autopilot-test" are used by some testcases at test setup time
if as_root dmsetup ls --target=crypt | grep -qE '^(autopilot-test|[0-9a-f]{32}\s)'; then
    for MAPNAME in $(as_root dmsetup ls --target=crypt | grep -E '^(autopilot-test|[0-9a-f]{32}\s)' | cut -f1); do
        log_debug "Cleanup: LUKS container /dev/mapper/${MAPNAME}"
        as_root cryptsetup close "${MAPNAME}"
    done
fi

if losetup | grep -qF "${DIR}"; then
    for DEVICE in $(losetup | grep -F "${DIR}" | cut -d' ' -f1); do
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

if [ -d /var/cache/swift ]; then
    log_debug "Cleanup: /var/cache/swift"
    as_root rm -rf -- /var/cache/swift
fi

log_debug "Cleanup: disk images in ${DIR}"
( cd "${DIR}" && rm -f -- image? )
log_debug "Cleanup: loop device links in ${DIR}"
( cd "${DIR}" && rm -f -- loop? )
