# 1. Клонируем/создаем проект
cd /opt
git clone https://github.com/yourusername/venti.git  # или создаем вручную
cd venti

# 2. Устанавливаем зависимости
go mod download

# 3. Собираем бинарник
make build

# 4. Устанавливаем в систему
sudo make install

# 5. Создаем тестовый Perl скрипт
sudo mkdir -p /var/www/scripts
sudo cat > /var/www/scripts/test.pl << 'EOF'
#!/usr/bin/perl

print "Content-Type: text/html\n\n";
print "<h1>Hello from Venti!</h1>";
print "<p>Time: " . localtime() . "</p>";
EOF

sudo chmod +x /var/www/scripts/test.pl
sudo chown -R venti:venti /var/www/scripts

# 6. Создаем systemd сервис
sudo cat > /etc/systemd/system/venti.service << 'EOF'
[Unit]
Description=Venti - Anemo Archon FastCGI Pool
After=network.target

[Service]
Type=simple
User=venti
Group=venti
ExecStart=/usr/local/bin/venti
Restart=always
RestartSec=5
LimitNOFILE=65536

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/venti /run/venti

[Install]
WantedBy=multi-user.target
EOF

# 7. Создаем пользователя venti
sudo useradd -r -s /bin/false venti

# 8. Создаем необходимые директории
sudo mkdir -p /var/log/venti
sudo mkdir -p /run/venti
sudo chown -R venti:venti /var/log/venti
sudo chown -R venti:venti /run/venti

# 9. Запускаем сервис
sudo systemctl daemon-reload
sudo systemctl start venti
sudo systemctl enable venti

# 10. Проверяем статус
sudo systemctl status venti

# 11. Смотрим логи
sudo journalctl -u venti -f



# Добавляем конфигурацию для nginx
sudo cat > /etc/nginx/sites-available/venti << 'EOF'
server {
    listen 80;
    server_name _;

    location / {
        root /var/www/scripts;
        index index.html;
    }

    location ~ \.pl$ {
        fastcgi_pass unix:/run/venti/venti.sock;
        
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param SERVER_SOFTWARE "Venti/1.0";
        
        fastcgi_buffering off;
        fastcgi_keep_conn on;
    }
}
EOF

# Включаем конфиг
sudo ln -s /etc/nginx/sites-available/venti /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx

