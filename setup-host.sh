#!/bin/bash
# Configuração única do host Linux para o backup agendado.
# Rode com: sudo bash setup-host.sh
#
# Instala:
#  1. Regra sudoers permitindo ao usuário lucas rodar ntfsfix -d no volume
#     Dados (e somente isso) sem senha — o run-backup.sh usa antes de montar,
#     para limpar a flag "dirty" do NTFS que impede a montagem.
#  2. Regra polkit permitindo ao usuário lucas montar volumes via udisks2
#     fora de sessão ativa — o backup das 9h pode rodar antes do login.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
    echo "Este script precisa de root. Rode: sudo bash $0" >&2
    exit 1
fi

cat > /etc/sudoers.d/db-backup <<'EOF'
lucas ALL=(root) NOPASSWD: /usr/bin/ntfsfix -d /dev/disk/by-uuid/01DCF4F5AAE8F170
EOF
chmod 440 /etc/sudoers.d/db-backup
if ! visudo -cf /etc/sudoers.d/db-backup; then
    rm -f /etc/sudoers.d/db-backup
    echo "ERRO: regra sudoers inválida — removida. Nada foi alterado." >&2
    exit 1
fi
echo "OK: /etc/sudoers.d/db-backup instalado."

cat > /etc/polkit-1/rules.d/49-db-backup-mount.rules <<'EOF'
// Permite ao usuário lucas montar volumes via udisks2 mesmo fora de uma
// sessão ativa (o backup agendado pode rodar antes do login no desktop).
polkit.addRule(function(action, subject) {
    if (action.id.indexOf("org.freedesktop.udisks2.filesystem-mount") == 0 &&
        subject.user == "lucas") {
        return polkit.Result.YES;
    }
});
EOF
echo "OK: /etc/polkit-1/rules.d/49-db-backup-mount.rules instalado."

echo "Concluído. Teste com: sudo -n -u lucas sudo -n /usr/bin/ntfsfix -d /dev/disk/by-uuid/01DCF4F5AAE8F170"
