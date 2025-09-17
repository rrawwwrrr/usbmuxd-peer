# sysctl -w net.ipv6.conf.all.disable_ipv6=0
# sysctl -w net.ipv6.conf.default.disable_ipv6=0
mkdir -p /var/logs/
export ENABLE_GO_IOS_AGENT=user
usbmuxd&
echo "Грузим образ xcode"
/goios image auto >> /var/logs/image 2>&1
sleep 2
echo "Запускаем стрим экрана"
/goios screenshot --stream >> /var/logs/screenshot 2>&1 &
sleep 2
echo "Пробрасываем порт 7777"
/goios forward 7777 8100 >> /var/logs/forward 2>&1 &
sleep 2
echo "Запускаем WebDriverAgent"
/goios runwda >> /var/logs/wda 2>&1 &
tail -f /var/logs/*
