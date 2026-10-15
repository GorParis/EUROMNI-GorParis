# Distribution autonome du CMS

EUROMNI est distribué comme un ensemble de binaires Go natifs. Le CMS, les
migrations SQL, l’interface admin et les assets storefront sont embarqués dans
les exécutables concernés ; l’installation n’a pas besoin du dépôt Git, de Go,
de Node.js ni de Docker. Le binaire est propre à son OS et à son architecture :
la CGo/Wasmtime exige une construction native sur chaque cible et ne permet pas
de supposer qu’un exécutable est universel.

Le paquet `euromni-cms-<OS>-<architecture>` contient :

| Exécutable | Usage |
|---|---|
| `euromni-cms` | Serveur CMS et storefront |
| `euromni-db-migrate` | Installation ou mise à niveau du schéma PostgreSQL |
| `euromni-staff-bootstrap` | Création initiale du compte owner |
| `euromni-artifact-reconciler` | Réconciliation planifiée du FileStore Wasm |

Les distributions Windows et Linux amd64 sont produites sur des runners natifs
par le workflow GitHub Actions `CMS standalone distribution`, déclenché
manuellement ou par un tag `cms-v*`. Le runner Linux est Ubuntu 22.04 pour
garder une base glibc LTS ; le runner Windows est Windows Server 2022. Chaque
archive est publiée comme artifact de workflow. Cette première tranche ne
déclare pas les autres OS ou architectures supportés.

## Installation

1. Extraire l’archive dans un répertoire d’application dédié. Garder les
   exécutables et leur configuration séparés des répertoires de données.
2. Installer PostgreSQL 16+ comme service local ou distant, puis créer une base
   et un rôle dédiés. PostgreSQL est le seul service nécessaire au CMS.
3. Définir `DATABASE_URL` dans l’environnement du processus, sans l’inscrire
   dans le dépôt ou la ligne de commande, puis exécuter
   `euromni-db-migrate` une première fois et après chaque mise à jour.
4. Créer le premier owner avec `euromni-staff-bootstrap`, en transmettant le
   mot de passe sur stdin. Ne pas passer le secret en argument.
5. Définir `LISTEN_ADDR` si nécessaire, puis lancer `euromni-cms`. Sans cette
   variable, le service écoute sur `127.0.0.1:8081`.

Exemple Linux :

```sh
export DATABASE_URL='postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable'
./euromni-db-migrate
read -rsp 'Mot de passe owner: ' OWNER_PASSWORD; printf '\n'
printf '%s\n' "$OWNER_PASSWORD" | ./euromni-staff-bootstrap \
  --shop '<shop-id>' --email '<owner-email>' --name '<owner-name>' --password-stdin
unset OWNER_PASSWORD
./euromni-cms
```

Exemple PowerShell :

```powershell
$env:DATABASE_URL = "postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable"
.\euromni-db-migrate.exe
$ownerPassword = Read-Host "Mot de passe owner" -AsSecureString
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($ownerPassword)
$previousOutputEncoding = $OutputEncoding
$OutputEncoding = New-Object System.Text.UTF8Encoding($false)
try {
  [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer) |
    .\euromni-staff-bootstrap.exe --shop "<shop-id>" --email "<owner-email>" --name "<owner-name>" --password-stdin
} finally {
  $OutputEncoding = $previousOutputEncoding
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
  Remove-Variable ownerPassword
}
.\euromni-cms.exe
```

Pour une utilisation multi-utilisateur, configurer explicitement `LISTEN_ADDR`,
limiter l’accès avec le pare-feu et placer un reverse proxy TLS devant le CMS.
Ne pas exposer directement le serveur HTTP sur Internet.

## Données et redémarrage

Les assets et migrations sont embarqués dans les exécutables. Les données
commerciales résident dans PostgreSQL, qui doit utiliser son propre stockage
persistant et sa politique de sauvegarde. Remplacer les exécutables lors d’une
mise à jour ne modifie ni n’efface les données ; lancer le migrateur avant le
redémarrage du serveur. Tester les sauvegardes et la restauration séparément,
conformément à [la documentation PostgreSQL](./local-postgresql.md).

Le FileStore des artefacts est un stockage distinct : le réconciliateur exige
`EUROMNI_ARTIFACT_DIR`, qui doit désigner le même répertoire privé que celui
utilisé par le processus qui publie les artefacts. Les permissions de ce
répertoire doivent respecter les exigences de sécurité de la plateforme.

Cette documentation décrit le paquet autonome, mais l’installation comme
service Windows ou unité systemd, la rotation des secrets et l’exercice de
restauration restent à valider avant un usage client.
