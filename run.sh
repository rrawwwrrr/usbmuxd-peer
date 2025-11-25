#!/bin/bash
export USBMUXD_SOCKET_ADDRESS=/var/run/usbmuxd

cd /app || exit

cleanup() {
    echo "Stopping usbmuxd..."
    kill -s SIGTERM $UMUXD_PID
    wait $UMUXD_PID
    echo "usbmuxd stopped."
}

trap 'cleanup' EXIT

echo "lsusb"
lsusb
echo "ls -lht /dev/bus/usb/001/"
ls -lht /dev/bus/usb/001/
usbmuxd -f &
UMUXD_PID=$!

./peer