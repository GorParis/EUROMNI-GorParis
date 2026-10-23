# Document d’architecture — EUROMNI

Pour garantir la sécurité, la vitesse et l’indépendance, l’architecture abandonne le modèle des serveurs monolithiques PHP pour une approche **Headless & Sandboxed**.

```
[ EXTENSIONS EXISTANTES (Marché Mondial) ]
           │  (ex: Plugin WooCommerce PHP, Module PrestaShop)
           ▼
┌────────────────────────────────────────────────────────┐
│ 1. PIPELINE D'ANALYSE ET DE COMPILATION               │
│    ├── Analyse statique des sources et IR validee     │
│    └── Generation WAT puis compilation en Wasm        │
└────────────────────────────────────────────────────────┘
           │
           ▼ (Code isolé et compilé)
┌────────────────────────────────────────────────────────┐
│ 2. CŒUR DU CMS SOUVERAIN (Moteur d’exécution en Go)    │
│                                                        │
│  ├── RUNTIME WASM (Wasmtime)                           │
│  │   └── [Module Traduit 1] [Module Traduit 2]         │
│  │       (Isolation totale : sécurité cyber garantie)  │
│  │                                                     │
│  ├── COUCHE API HAUTE PERFORMANCE (GraphQL / REST)     │
│  └── ACCÈS BASE DE DONNÉES (PostgreSQL, actuellement IONOS) │
└────────────────────────────────────────────────────────┘
           │
           ▼ (Livraison ultra-rapide des données)
┌────────────────────────────────────────────────────────┐
│ 3. FRONT-END DE LA BOUTIQUE                            │
│    └── Composants Web Universels (Shadow DOM isolés)   │
└────────────────────────────────────────────────────────┘
```

## 1. Pipeline de transpilation IA

- **Entrée** : plugins / modules PHP du marché (WooCommerce, PrestaShop, Magento, etc.).
- **Analyse** : syntaxe, appels API, hooks, accès données, effets de bord.
- **Sortie actuelle** : module Wasm généré depuis un sous-ensemble d'IR ; la
  traduction générale du code source et les bindings Go ne sont pas disponibles.
- **Modèle** : LLM (ex. DeepSeek) pour extraire la logique métier ; compilation déterministe ensuite.

Le code invité **n’a jamais** d’accès direct au processus hôte ni au PHP historique. Le dépôt contient des validateurs d’archives, des analyseurs statiques ciblés, une IR versionnée et une génération Wasm limitée à partir de cette IR ; il ne contient pas encore de traduction générique de code PHP ni d’ingestion HTTP authentifiée. La compilation Go `wasip1` testée importe des appels WASI en plus de `euromni.call` et ne convient pas au linker strict. La [matrice de portabilité](portability.md) sépare ce problème de toolchain de celui des hôtes Go/CGo du runtime.

## 2. Cœur CMS (Go)

| Composant | Rôle |
|-----------|------|
| Runtime Wasmtime | Charge et exécute les modules traduits, capacités minimales (capability-based). |
| API GraphQL **et** REST | Les deux surfaces dès le départ : même domaine, deux transports. |
| PostgreSQL | Données souveraines : catalogue, commandes, sessions, config modules. Schéma dans `docs/database.md`. Hébergeur indifférent (aujourd’hui IONOS ; OVH, autre cloud ou dédié conviennent). |
| SQLite | Prototype isolé mono-boutique pour évaluer une installation légère ; non branché au CMS et non destiné à la production. Voir [`docs/sqlite-prototype.md`](sqlite-prototype.md). |

Principes :

- Isolation totale des modules : pas d’accès brut au filesystem, au réseau ou à PostgreSQL.
- Le moteur Go est la seule couche de confiance (trust boundary).
- Pas de runtime PHP en production.
- Tout le métier e-commerce possible passe par l’ABI hôte (voir ci-dessous), jamais par des syscalls.

## ABI Wasm (hôte ↔ module)

Objectif : **tout le domaine e-commerce possible**, sans casser le sandbox.

Le module transpilé n’appelle pas l’OS. Il n’appelle que des **capabilities** exposées par le runtime Go. Si un plugin PHP faisait « tout », l’équivalent EUROMNI est « toutes les fonctions métier médiées », pas un accès root.

| Famille | Exemples de capabilities |
|---------|--------------------------|
| Catalogue | produits, variantes, stocks, taxonomie, recherche, médias |
| Panier / commande | panier, checkout, totaux, taxes, livraisons, commandes |
| Client | comptes, adresses, sessions, consentements |
| Paiement | initiation / confirmation via adaptateurs hôte (jamais de secrets dans Wasm) |
| Notifications | courriels métier rendus par le CMS, remis via un adaptateur de transport côté hôte |
| Contenu / CMS | pages, blocs, menus, i18n |
| Hooks | `before_*` / `after_*` sur les événements métier |
| Admin | CRUD config module, feature flags, jobs planifiés |
| Intégrations | HTTP sortant **uniquement** via allowlist + quotas (capability `http.fetch`) |
| Stockage module | KV / blobs **scoped** au module, pas le disque hôte |

Hors ABI (refusés par défaut) : shell, filesystem hôte, sockets bruts, lecture de secrets, SQL direct.

Le contrat d’ABI est versionné (`euromni.abi.v1`). Mapping host calls → tables : [`docs/abi.md`](abi.md).

### Service de courriel transactionnel

Le checkout et chaque transition de commande enregistrent leurs événements
dans une outbox transactionnelle ; un worker hôte les remet via SMTP
configurable. Le cycle partagé couvre paiement reçu, traitement, préparation,
expédition et livraison. Mailpit sert de transport local de développement.
Les identifiants et l’accès réseau ne sont jamais exposés aux modules Wasm.
La remise est au moins une fois ; la rétention, l’administration des échecs,
la supervision et la production d’un MTA propre restent à cadrer. Voir
[`email-service.md`](email-service.md).

## API headless (REST + GraphQL)

Les deux coexistent et parlent au **même** cœur de domaine :

- **REST** (`/v1/...`) : simple, cacheable, webhooks, intégrations.
- **GraphQL** (`/graphql`) : requêtes boutique (moins de round-trips, sélection de champs).

Pas deux modèles de données : un domaine Go, deux adaptateurs HTTP.

## Transport extensible

Le transport est un domaine natif extensible : le cœur gère services, règles
tarifaires, contraintes et snapshots de devis ; un module Wasm peut fournir une
règle métier via une capability typée, sans SQL, réseau ni secrets directs. Le
modèle, les phases de migration et le contrat de devis sont décrits dans
[`docs/transport-extensible.md`](transport-extensible.md).

Un premier pilote opt-in applique une règle Wasm embarquée au devis d’un panier
et la revalide au checkout. L’hôte garde l’autorité sur le tarif proposé et
enregistre l’identité du module dans le snapshot de commande. Ce pilote ciblé
ne constitue pas encore le stockage ni l’installation générique des artefacts
de modules.

## Marchés, locales et commerce transfrontalier

Le modèle de marchés UE distingue pays, langue, devise, taxes, transport et
moyens de paiement. Le français (`fr-FR`) reste la locale par défaut ; la langue
ne se déduit pas automatiquement du pays de livraison. La prise en charge
préparée des 27 États membres n’active pas à elle seule la vente ni des règles
fiscales ou des tarifs. Voir le cadrage et les étapes dans
[`docs/marches-localisation-europe.md`](marches-localisation-europe.md).

## 3. Front boutique

- Web Components (Custom Elements + Shadow DOM) pour isoler styles et DOM.
- Consomme REST et/ou GraphQL du cœur (pas de couplage à un thème PHP).
- Peut être hébergé séparément (CDN / edge).

## Décisions encore ouvertes

- Périmètre v1 du pipeline (un plugin pilote vs extracteur générique).
- Périmètre produit v1 (moteur seul, boutique, ou bout-en-bout).
- Où tourne Postgres (cloud / dédié) — le schéma ne dépend pas de ça. L’instance actuelle est chez IONOS.
- OS/architectures du runtime hôte et toolchain des guests ; Windows/amd64 et Linux/amd64 sont qualifiés dans les configurations CI documentées, sans déclaration générique de support pour d’autres versions ou environnements. Voir la [matrice de portabilité](portability.md).
- Hébergement mail de production et domaine expéditeur à choisir ; le worker SMTP ne constitue pas un MTA.
