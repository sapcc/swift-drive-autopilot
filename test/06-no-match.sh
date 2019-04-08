#!/bin/bash
cd "$(dirname "$(readlink -f $0)")"
source ./lib/common.sh
source ./lib/cleanup.sh

with_config <<-EOF
    drives: [ '${DIR}/loop?' ]
EOF

run_and_expect_failure <<-EOF
> FATAL: no drives found matching the configured patterns: {{dir}}/loop?
EOF
