#!/usr/bin/env bash

set -eu

cat <<EOF >/etc/nginx/nginx.conf
user  nginx;
worker_processes  1;

error_log  /var/log/nginx/error.log warn;
pid        /var/run/nginx.pid;


events {
    worker_connections  1024;
}


http {
    include       /etc/nginx/mime.types;
    default_type  application/octet-stream;

    log_format  main  '\$remote_addr - \$remote_user [\$time_local] "\$request" '
                      '\$status \$body_bytes_sent "\$http_referer" '
                      '"\$http_user_agent" "\$http_x_forwarded_for"';

    access_log  /var/log/nginx/access.log  main;

    sendfile        on;

    keepalive_timeout  65;

    upstream app {
      server 127.0.0.1:$UPSTREAM_PORT;
      keepalive 64;
    }

    server {
      listen *:$LISTEN_PORT;
      server_name _;
      auth_basic "Restricted Area";
      auth_basic_user_file /etc/nginx/.htpasswd;

      location / {
        proxy_pass http://app;
      }
    }
}
EOF

exec nginx -g "daemon off;"
