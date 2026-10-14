# Prototype SQLite mono-boutique

`internal/sqlitestore` est une expérience isolée pour évaluer une installation
légère. Elle implémente les interfaces de lecture du catalogue et du panier,
ainsi que le checkout, sans remplacer PostgreSQL dans le CMS.

## Périmètre couvert

- Catalogue, traductions, catégories, variantes, prix et médias.
- Paniers invités avec jeton stocké sous forme de hash et expiration.
- Checkout transactionnel avec instantané des lignes de commande et décrément
  conditionnel du stock.
- Tests de parcours complet, d’isolation entre boutiques et de deux checkouts
  concurrents pour le dernier article.
- Montants stockés sous forme décimale textuelle et calculés sans flottants.

Les migrations SQLite embarquées sont versionnées séparément de PostgreSQL dans
`internal/sqlitestore/migrations`; `schema_migrations` enregistre leur
application. Une base créée par les premières versions du prototype, sans
registre, reçoit la migration initiale idempotente sans perdre ses tables ni ses
données. Une base déclarant une version plus récente que le binaire est refusée.
Les migrations ne convertissent pas le schéma PostgreSQL; il n’existe pas encore
d’import ou d’export entre les deux moteurs. Le prototype n’est pas connecté à
`cmd/cms`.

## Limites et critères avant usage réel

- PostgreSQL reste le moteur de référence, notamment pour l’architecture
  multi-boutique et les fonctions complètes du CMS.
- SQLite ne couvre pas encore les paiements, taxes, livraisons, comptes clients,
  administration ni le cycle de vie complet des commandes.
- Le backend limite son pool à une connexion et sérialise certaines écritures
  par processus. Les transactions prennent le verrou d’écriture dès leur début
  et SQLite attend jusqu’à cinq secondes en cas de verrou occupé. Un test de
  checkout concurrent entre deux processus Windows confirme qu’un seul peut
  consommer le dernier article; les autres systèmes et systèmes de fichiers
  n’ont pas encore été qualifiés.
- Le backend exige un répertoire dédié et privé pour la base : permissions
  propriétaire seul sur Unix, ACL NTFS protégée pour l’utilisateur courant,
  SYSTEM et Administrateurs sur Windows. Les nouveaux fichiers SQLite et
  auxiliaires WAL/SHM sont créés dans ce répertoire; une configuration dont les
  permissions ne satisfont pas ces contrôles est refusée. Placer une base
  préexistante dans ce répertoire ne corrige pas automatiquement ses ACL propres;
  elles doivent être vérifiées avant ouverture.
- `Store.Backup(ctx, destination)` produit un snapshot cohérent, y compris les
  transactions validées encore dans le WAL. Le snapshot est d’abord créé sous
  un nom temporaire puis publié sans écraser une destination existante. La
  destination doit être nouvelle et située dans un répertoire privé; le chemin
  de la base est refusé. Ne pas sauvegarder une base active en copiant seulement
  son fichier `.sqlite`. La restauration doit être testée en ouvrant une copie
  dans un répertoire privé distinct; conserver aussi une copie hors machine.
- Le pilote SQLite est pur Go, mais le CMS complet dépend toujours de CGo via
  Wasmtime; choisir SQLite ne rend donc pas l’application complète indépendante
  de CGo.

Avant toute activation ou utilisation avec des données clients, il faudra
qualifier les opérations annulées par contexte, tester la sauvegarde/restauration
sur tous les systèmes et systèmes de fichiers visés, et établir une procédure
opérationnelle de rotation des sauvegardes hors machine. Le prototype n’est pas
activé par défaut.
