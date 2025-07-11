#!/bin/bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

THISDIR="$(dirname "$(readlink -f "$0")")"
source "${THISDIR}/lib/common.sh"

# run each testcase in order
cd "${THISDIR}"
for TESTCASE in ??-*.sh; do
    echo -e "\e[1;34mRunning testcase ${TESTCASE}...\e[0m"
    if ! "./${TESTCASE}"; then
        exec >&2
        echo
        echo -e "\e[1;31mTestcase ${TESTCASE} FAILED.\e[0m"
        echo "To rerun in debug mode, execute the following from the repo root:"
        echo "    make all build/logexpect && env DEBUG=1 ./test/${TESTCASE}"
        echo
        exit 1
    fi
done

echo -e "\e[1;32mAll testcases finished.\e[0m"
source "${THISDIR}/lib/cleanup.sh"
