#!/bin/bash
export USBMUXD_SOCKET_ADDRESS=/var/run/usbmuxd

usbmuxd&
sleep 2
# shellcheck disable=SC2164
cd /app
./peer