#!/bin/sh
# Recreates the dependency copies that go.mod points at but that are not kept in the repo (vendor/ already
# holds the parts the app uses, so this is only needed to run "go mod vendor" again).
set -e
cd "$(dirname "$0")"
git clone -q --depth 1 -b v0.8.0 https://github.com/golang/sync x-sync
git clone -q --depth 1 -b v0.17.0 https://github.com/golang/text x-text
git clone -q https://github.com/go4org/go4 go4 && (cd go4 && git checkout -q f5505b9728dd)
git clone -q --depth 1 -b v3.0.1 https://github.com/go-yaml/yaml yaml3
for d in go4 x-sync x-text yaml3; do
  m=$(head -1 $d/go.mod | awk '{print $2}' | tr -d '"')
  printf 'module %s\n\ngo 1.18\n' "$m" > $d/go.mod
  rm -rf $d/.git $d/go.sum
done
