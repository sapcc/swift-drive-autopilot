#!/bin/bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

run_and_expect_failure <<-EOF
> FATAL: no drives found matching the configured patterns: {{dir}}/loop?
EOF
