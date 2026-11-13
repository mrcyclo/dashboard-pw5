#!/bin/sh
# Runs from the unpacked package. Keeps cache/, state.json and config.json from a previous install.
APP=/mnt/us/claudedash
mkdir -p "$APP"
cp -f ./claudedash/* "$APP/"
cp -f ./launch.sh "$APP/launch.sh"
cp -f "./scriptlets/Claude Dashboard.sh" "/mnt/us/documents/Claude Dashboard.sh"
