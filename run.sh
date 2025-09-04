#!/bin/bash
export USBMUXD_SOCKET_ADDRESS=/var/run/usbmuxd

cd /app || exit

# Запускаем usbmuxd в фоне, чтобы иметь возможность запускать peer
usbmuxd -f &
UMUXD_PID=$!

# Запускаем основной процесс
./peer

# При выходе контейнера убиваем usbmuxd
kill $UMUXD_PID