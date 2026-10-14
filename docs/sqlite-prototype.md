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

Le schéma SQLite embarqué dans le binaire est autonome. Il ne lit pas et ne
convertit pas les migrations PostgreSQL; il n’existe pas encore d’import ou
d’export de données entre les deux moteurs. Le prototype n’est pas encore
connecté à `cmd/cms`.

## Limites et critères avant usage réel

- PostgreSQL reste le moteur de référence, notamment pour l’architecture
  multi-boutique et les fonctions complètes du CMS.
- SQLite ne couvre pas encore les paiements, taxes, livraisons, comptes clients,
  administration, migrations versionnées ni le cycle de vie des commandes.
- Le backend limite son pool à une connexion et sérialise certaines écritures
  dans un seul processus. Les garanties d’exploitation multi-processus et les
  comportements sous forte concurrence ne sont pas encore qualifiés.
- Les permissions du fichier et de ses fichiers WAL/SHM, en particulier les
  ACL NTFS sous Windows, doivent être auditées avant d’y stocker des données
  réelles.
- Le pilote SQLite est pur Go, mais le CMS complet dépend toujours de CGo via
  Wasmtime; choisir SQLite ne rend donc pas l’application complète indépendante
  de CGo.

Avant toute activation ou utilisation avec des données clients, il faudra
qualifier le verrouillage inter-processus, l’annulation des opérations par
contexte, la protection des fichiers sur chaque système pris en charge, la
récupération et sauvegarde des données, puis définir un contrat de configuration
et de migrations. Le prototype n’est pas activé par défaut.
