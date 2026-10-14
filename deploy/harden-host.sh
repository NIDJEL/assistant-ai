#!/bin/sh
set -eu
[ "$(id -u)" -eq 0 ] || { echo 'Run as root'; exit 1; }
# Verify the new administrator and key before removing password/root access.
test -s /home/rhythm-admin/.ssh/authorized_keys
visudo -cf /etc/sudoers.d/rhythm-admin
install -d -m 700 /root/rhythm-config-backup
cp -an /etc/ssh/sshd_config /root/rhythm-config-backup/
cat > /etc/ssh/sshd_config.d/00-rhythm-hardening.conf <<'EOF'
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
AuthenticationMethods publickey
PermitEmptyPasswords no
X11Forwarding no
AllowAgentForwarding no
AllowTcpForwarding no
PermitTunnel no
MaxAuthTries 3
LoginGraceTime 30
MaxSessions 5
ClientAliveInterval 300
ClientAliveCountMax 2
AllowUsers rhythm-admin
EOF
sshd -t
systemctl reload ssh
passwd -l root
ufw default deny incoming
ufw default allow outgoing
ufw limit 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
cat > /etc/fail2ban/jail.d/rhythm.local <<'EOF'
[sshd]
enabled = true
backend = systemd
maxretry = 5
findtime = 10m
bantime = 1h
bantime.increment = true
bantime.maxtime = 1d
EOF
systemctl enable --now fail2ban
fail2ban-client reload
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOF
cat > /etc/apt/apt.conf.d/52rhythm-security <<'EOF'
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-Time "04:30";
EOF
systemctl enable --now apt-daily.timer apt-daily-upgrade.timer certbot.timer
cat > /etc/sysctl.d/60-rhythm-security.conf <<'EOF'
kernel.kptr_restrict = 2
kernel.dmesg_restrict = 1
fs.protected_hardlinks = 1
fs.protected_symlinks = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.default.accept_redirects = 0
net.ipv4.tcp_syncookies = 1
EOF
sysctl --system >/dev/null
install -d /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/rhythm.conf <<'EOF'
[Journal]
SystemMaxUse=500M
MaxRetentionSec=30day
EOF
systemctl restart systemd-journald
ufw status verbose
sshd -T | grep -E 'permitrootlogin|passwordauthentication|authenticationmethods|allowusers'
