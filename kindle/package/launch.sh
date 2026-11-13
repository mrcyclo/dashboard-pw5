#!/bin/sh
# Runs the dashboard, then makes sure the Kindle UI comes back even if the app crashed.
APP=/mnt/us/claudedash
MARKER=/var/tmp/claudedash.paused
DUMP=/var/tmp/claudedash-fb.dump
FBINK=/mnt/us/libkh/bin/fbink

# /mnt/us is FAT, so the binary runs from a copy in /tmp.
cp -f "$APP/claudedash" /tmp/claudedash-bin
chmod +x /tmp/claudedash-bin
/tmp/claudedash-bin >> "$APP/claudedash.out" 2>&1

if [ -f "$MARKER" ]; then
    iptables -D INPUT -p udp --dport 8378 -j ACCEPT
    lipc-set-prop com.lab126.powerd preventScreenSaver 0
    killall -CONT volumd
    killall -CONT awesome
    if [ -f "$DUMP" ]; then
        cat "$DUMP" > /dev/fb0
        rm -f "$DUMP"
    fi
    "$FBINK" -q -s -f
    lipc-set-prop com.lab126.pillow disableEnablePillow enable
    lipc-set-prop com.lab126.appmgrd start app://com.lab126.booklet.home
    rm -f "$MARKER"
fi
