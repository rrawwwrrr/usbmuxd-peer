#!/bin/bash
export USBMUXD_SOCKET_ADDRESS=/var/run/usbmuxd

cd /app || exit

# Функция для корректного завершения usbmuxd
cleanup() {
    echo "Stopping usbmuxd..."
    kill -s SIGTERM $UMUXD_PID
    wait $UMUXD_PID
    echo "usbmuxd stopped."
}

# Обработка сигнала завершения контейнера
trap 'cleanup' EXIT

# Запускаем usbmuxd в фоне, чтобы иметь возможность запускать peer
usbmuxd -f &
UMUXD_PID=$!

# Запускаем основной процесс
./peer