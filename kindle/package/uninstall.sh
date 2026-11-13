#!/bin/sh
# KPM passes "upgrade" when a new version replaces this one; the app folder stays then.
rm -f "/mnt/us/documents/Claude Dashboard.sh"
if [ "$1" != "upgrade" ]; then
    rm -rf /mnt/us/claudedash
fi
