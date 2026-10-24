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
| `euromni-db-backup` | Sauvegarde PostgreSQL/FileStore, vérification et restauration du FileStore |
| `euromni-staff-bootstrap` | Création initiale du compte owner |
| `euromni-artifact-reconciler` | Réconciliation planifiée du FileStore Wasm |

Les distributions Windows et Linux amd64 sont produites sur des runners natifs
par le workflow GitHub Actions `CMS standalone distribution`, déclenché
manuellement ou par un tag `cms-v*`. Le runner Linux est Ubuntu 22.04 pour
garder une base glibc LTS ; le runner Windows est Windows Server 2022. Chaque
archive est publiée avec un fichier `.sha256` adjacent dans le même artifact.
Sous Linux, vérifier avec `sha256sum -c euromni-cms-linux-amd64.tar.gz.sha256`.
Sous PowerShell, comparer le premier champ de
`euromni-cms-windows-amd64.zip.sha256` avec
`(Get-FileHash .\euromni-cms-windows-amd64.zip -Algorithm SHA256).Hash`.
Ces sommes détectent les corruptions accidentelles ; elles ne sont pas une
attestation signée et ne prouvent pas à elles seules l’origine du fichier.
Cette première tranche ne déclare pas les autres OS ou architectures supportés.

La validation locale du 17 octobre 2026 a d'abord démarré le binaire Windows
sans `DATABASE_URL` : `/health`, `/admin/` et `/v1/products` ont répondu HTTP
200. Une validation distincte a ensuite construit les exécutables Windows,
appliqué les migrations sur PostgreSQL 16.15 isolé, configuré le owner de la
boutique exemple, puis créé un produit et une commande via l'API. Après
redémarrage du CMS, l'authentification, le produit, le client et la ligne de
commande étaient toujours présents. Enfin, un dump PostgreSQL au format custom
a été restauré dans une autre base ; le produit, le compte owner et les
snapshots de commande ont tous été retrouvés. L'instance et les fichiers
temporaires ont été supprimés après le test.

Le premier workflow natif Windows/Linux a réussi sur le commit
[`604b272`](https://github.com/GorParis/EUROMNI-GorParis/commit/604b2723aba9fd6ea464e0fbbfaf025e866fd19c)
le 17 octobre 2026. Ses deux archives sont disponibles comme artifacts dans
[l'exécution GitHub Actions](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39015220199)
jusqu'au 16 novembre 2026. Le runner Linux valide la compilation et le paquet
Linux amd64 ; le runner Windows valide la compilation et le paquet Windows
amd64.

Le workflow a ensuite été renforcé pour extraire et démarrer les binaires
contenus dans chaque archive sur leur runner natif. Le run
[`#39017221737`](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39017221737),
sur le commit [`35ff3c6`](https://github.com/GorParis/EUROMNI-GorParis/commit/35ff3c66eb301da5e6038979647a434214f7d34f),
a réussi sur Ubuntu 22.04 et Windows Server 2022 : `/health`, `/admin/` et
`/v1/products` ont répondu HTTP 200 sur les deux binaires extraits. Les
artifacts Windows et Linux de ce run sont disponibles jusqu'au 16 novembre
2026 dans [l'exécution GitHub Actions](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39017221737).
Le paquet Windows téléchargé depuis le premier run a également été installé
contre PostgreSQL 16.15 isolé : migrations, bootstrap, catalogue HTTP et
authentification owner ont réussi. L'exercice de persistance et de
restauration PostgreSQL est consigné dans
[`local-postgresql.md`](./local-postgresql.md).

## Installation

1. Extraire l’archive dans un répertoire d’application dédié. Garder les
   exécutables et leur configuration séparés des répertoires de données.
2. Installer PostgreSQL 16+ comme service local ou distant, puis créer une base
   et un rôle dédiés. PostgreSQL est le seul service nécessaire au CMS.
3. Définir `DATABASE_URL` dans l’environnement du processus, sans l’inscrire
   dans le dépôt ou la ligne de commande, puis exécuter
   `euromni-db-migrate` une première fois et après chaque mise à jour.
4. Créer le premier owner avec `euromni-staff-bootstrap`, en transmettant le
   mot de passe sur stdin. Ne pas passer le secret en argument. Sur une base
   fraîche, les migrations créent la boutique exemple `atelier-demo` et son
   owner `owner@atelier-demo.example` sans mot de passe ; le bootstrap doit
   recevoir cet identifiant de boutique et cette adresse. Il ne peut pas
   remplacer un owner déjà initialisé ni ajouter un autre owner initial à cette
   boutique.
5. Définir `LISTEN_ADDR` si nécessaire, puis lancer `euromni-cms`. Sans cette
   variable, le service écoute sur `127.0.0.1:8081`.

Exemple Linux :

```sh
export DATABASE_URL='postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable'
./euromni-db-migrate
read -rsp 'Mot de passe owner: ' OWNER_PASSWORD; printf '\n'
printf '%s\n' "$OWNER_PASSWORD" | ./euromni-staff-bootstrap \
  --shop 'e0e00000-0001-4000-8000-000000000001' \
  --email 'owner@atelier-demo.example' --name 'Owner Atelier Demo' --password-stdin
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
    .\euromni-staff-bootstrap.exe --shop "e0e00000-0001-4000-8000-000000000001" `
      --email "owner@atelier-demo.example" --name "Owner Atelier Demo" --password-stdin
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
persistant et sa politique de sauvegarde. Le paquet inclut
`euromni-db-backup` ; la création d'un dump requiert aussi `pg_dump` dans le
`PATH`. Remplacer les exécutables lors d’une mise à jour ne modifie ni n’efface
les données ; lancer le migrateur avant le redémarrage du serveur. Les limites,
la rétention et les commandes de vérification/restauration sont décrites dans
[la documentation PostgreSQL](./local-postgresql.md).

L'exercice de sauvegarde/restauration PostgreSQL est documenté dans
[`local-postgresql.md`](./local-postgresql.md). La rétention locale est définie
et automatisable par un ordonnanceur externe ; chiffrement et stockage hors
machine restent à choisir avant l'exploitation.

Le FileStore des artefacts est un stockage distinct : le réconciliateur exige
`EUROMNI_ARTIFACT_DIR`, qui doit désigner le même répertoire privé que celui
utilisé par le processus qui publie les artefacts. Les permissions de ce
répertoire doivent respecter les exigences de sécurité de la plateforme.

La réception prototype de builds signés est désactivée tant que
`EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR` n'est pas configuré. Pour l'activer,
fournir également `EUROMNI_BUILD_TRUST_FILE`, un JSON de clés publiques
approuvées propre à cette installation ; une liste vide n'approuve aucun
signataire. Exemple Linux :

```sh
export EUROMNI_BUILD_TRUST_FILE='/etc/euromni/trusted-build-keys.json'
export EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR='/var/lib/euromni/build-package-candidates'
./euromni-cms
```

Protéger le fichier de confiance et les deux répertoires avec les droits du
compte de service. Le stockage de candidats est privé et séparé du FileStore
des artefacts publiés, plafonné provisoirement à 256 MiB par boutique et non
coordonné entre instances. Un `owner` ou `admin` peut lister et relire un
candidat, l'approuver ou le rejeter avec un motif obligatoire ; un rejet peut
être rouvert et chaque décision est tracée par acteur, rôle et horodatage. Ces
actions ne publient, n'installent ni ne promeuvent le build. La trace est un
sidecar local, borné et non coordonné entre instances, pas un journal centralisé
ni inviolable contre un administrateur système.
La purge automatique et le passage au registre restent absents ; ne pas activer
ce prototype en production.

Cette documentation décrit le paquet autonome, mais l'installation comme
service Windows ou unité systemd, la rotation des secrets et une politique de
sauvegarde protégée restent à valider avant un usage client.
