#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .test-build

g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/firmware_state_machine_test.cpp \
  -o .test-build/firmware_state_machine_test
.test-build/firmware_state_machine_test

g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/protocol_v2_test.cpp \
  -o .test-build/protocol_v2_test
.test-build/protocol_v2_test

g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/protocol_v3_test.cpp \
  -o .test-build/protocol_v3_test
.test-build/protocol_v3_test

g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/input_debounce_test.cpp \
  -o .test-build/input_debounce_test
.test-build/input_debounce_test

g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/input_semantics_test.cpp \
  -o .test-build/input_semantics_test
.test-build/input_semantics_test


g++ -std=gnu++17 -Wall -Wextra -Werror \
  -Itests/native -Iinclude \
  tests/native/firmware_hid_v3_state_machine_test.cpp \
  -o .test-build/firmware_hid_v3_state_machine_test
.test-build/firmware_hid_v3_state_machine_test
