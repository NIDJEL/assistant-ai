#!/bin/sh
set -eu
cd /opt/rhythm/current
install -d -m 755 /var/www/letsencrypt
if [ -e /etc/nginx/sites-enabled/default ]; then
    mv /etc/nginx/sites-enabled/default /root/rhythm-config-backup/nginx-default
fi
if [ ! -f /etc/letsencrypt/live/meinangelplatz.top/fullchain.pem ]; then
    cat > /etc/nginx/conf.d/rhythm.conf <<'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name meinangelplatz.top;
    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
        default_type text/plain;
    }
    location / { return 308 https://meinangelplatz.top$request_uri; }
}
EOF
    nginx -t
    systemctl reload nginx
    certbot certonly --non-interactive --agree-tos --register-unsafely-without-email \
        --webroot -w /var/www/letsencrypt -d meinangelplatz.top --key-type ecdsa
fi
install -m 644 deploy/nginx-proxy.conf /etc/nginx/snippets/rhythm-proxy.conf
install -m 644 deploy/nginx.conf /etc/nginx/conf.d/rhythm.conf
nginx -t
systemctl reload nginx
install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/20-reload-nginx <<'EOF'
#!/bin/sh
set -eu
/usr/sbin/nginx -t
/bin/systemctl reload nginx
EOF
chmod 755 /etc/letsencrypt/renewal-hooks/deploy/20-reload-nginx
systemctl enable --now certbot.timer
certbot renew --dry-run --run-deploy-hooks
echo 'HTTPS and renewal validation complete'
