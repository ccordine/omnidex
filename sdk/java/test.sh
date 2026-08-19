#!/usr/bin/env bash
set -euo pipefail

classes="$(mktemp -d /tmp/omnidex-java-sdk.XXXXXX)"
mapfile -t sources < <(find src/main/java src/test/java -name '*.java' -type f | sort)
javac --release 21 -d "$classes" "${sources[@]}"
java -ea -cp "$classes" com.omnidex.integration.SdkContractTest
