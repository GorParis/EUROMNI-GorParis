# Base locale du CMS

Le CMS se connecte à PostgreSQL standard via `DATABASE_URL`. Docker Compose est
une commodité locale, pas une dépendance d’exécution : le binaire Go peut tourner
directement sur une machine Windows ou Linux avec un service PostgreSQL local ou
accessible sur le réseau.

## Démarrer

Créer un fichier local non versionné `.env` à partir du modèle `.env.example`, remplacer sa valeur par un secret long composé de caractères sûrs pour une URL, puis lancer depuis la racine du dépôt :

```sh
docker compose --profile cms up -d --wait
```

Compose démarre PostgreSQL et le CMS sur le réseau privé du projet. Seul le port HTTP du CMS est publié, sur loopback ; `docker compose --profile cms port cms 8081` affiche l’adresse locale attribuée.

Au premier démarrage sur un volume vide, les migrations numérotées et le seed d’exemple sont exécutés ; `schema_migrations` conserve les versions enregistrées. Les démarrages suivants réutilisent les données existantes et ne rejouent pas les scripts d’initialisation. Toute nouvelle migration doit être appliquée explicitement, dans l’ordre, et enregistrer sa version dans `schema_migrations` au sein de la même transaction.

## Lancer sans Docker

Installer et démarrer PostgreSQL comme service sur la machine, créer une base et
un rôle dédiés, puis appliquer les migrations versionnées dans l’ordre à cette
base vide. Ne pas rejouer les migrations sur une base initialisée : la table
`schema_migrations` sert de registre. Définir `DATABASE_URL`, initialiser le
premier propriétaire avec `cmd/staff-bootstrap`, puis démarrer le CMS :

PowerShell :

```powershell
$env:DATABASE_URL = "postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable"
Get-ChildItem .\migrations\*.sql | Sort-Object Name | ForEach-Object {
  psql $env:DATABASE_URL -v ON_ERROR_STOP=1 -f $_.FullName
  if ($LASTEXITCODE -ne 0) { throw "Migration failed: $($_.Name)" }
}
```

Linux/macOS :

```sh
export DATABASE_URL='postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable'
for migration in migrations/*.sql; do
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$migration" || exit 1
done
```

Ensuite, depuis la racine du dépôt, configurez l’owner avec la commande du
README en fournissant son secret sur stdin, jamais dans les arguments, puis
démarrez le service Go :

```powershell
$env:LISTEN_ADDR = "127.0.0.1:8081"
go run ./cmd/cms
```

Sous Linux/macOS, définissez `LISTEN_ADDR` avec `export` avant de démarrer. Le
CMS écoute par défaut sur `127.0.0.1:8081`; `LISTEN_ADDR` permet de choisir
explicitement l’adresse et le port. Si `DATABASE_URL` est fourni mais que
PostgreSQL est injoignable, le processus échoue au démarrage au lieu de basculer
silencieusement vers le store mémoire.

Pour un accès depuis un réseau local de confiance, configurez une adresse
d’interface dédiée ou `0.0.0.0:8081`, limitez l’accès avec le pare-feu et placez
un reverse proxy TLS devant le CMS. Ne publiez pas directement le serveur HTTP
sur Internet. Les secrets de base et de bootstrap doivent rester dans le
gestionnaire de secrets ou l’environnement du processus, jamais dans Git.

## Persistance et sauvegardes

Avec Compose, les données PostgreSQL résident dans le volume Docker nommé du
projet. `stop` et `down` retirent ou arrêtent les conteneurs mais conservent ce
volume. **Ne pas utiliser `down -v`** pour une base qui contient des commandes
ou des preuves de transaction : cette option supprime les données.

Un volume local ou un disque de machine protège contre certaines pannes, mais
ne constitue pas une sauvegarde. Une exploitation client exige une politique de
sauvegardes vérifiées et hors machine, une procédure de restauration testée,
ainsi que la conservation des snapshots de commande et de l’historique des
migrations.

Les tests d’intégration utilisent une instance PostgreSQL embarquée dans le processus de test et confinée sous `.tmp/embedded-postgres`. Ils n’utilisent jamais le volume `postgres-data` du CMS.
