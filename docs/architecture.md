# Document d’architecture — EUROMNI

Pour garantir la sécurité, la vitesse et l’indépendance, l’architecture abandonne le modèle des serveurs monolithiques PHP pour une approche **Headless & Sandboxed**.

```
[ EXTENSIONS EXISTANTES (Marché Mondial) ]
           │  (ex: Plugin WooCommerce PHP, Module PrestaShop)
           ▼
┌────────────────────────────────────────────────────────┐
│ 1. PIPELINE DE TRANSPILATION IA (DeepSeek / LLM)       │
│    ├── Analyse syntaxique et extraction de la logique  │
│    └── Transpilation vers Go / WebAssembly (Wasm)      │
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
- **Sortie** : module Wasm (et éventuellement bindings Go) exécutable dans le runtime isolé.
- **Modèle** : LLM (ex. DeepSeek) pour extraire la logique métier ; compilation déterministe ensuite.

Le code transpilé **n’a jamais** d’accès direct au processus hôte ni au PHP historique. Le contrat détaillé d’ingestion, d’analyse, de traduction, de validation et de publication est décrit dans [`pipeline/README.md`](../pipeline/README.md). Le pipeline source PHP n’est pas encore implémenté ; seul le stockage local d’artefacts et le runtime Wasmtime/ABI sont disponibles. La compatibilité entre le compilateur Go/Wasm prévu et l’import custom `euromni.call` doit être démontrée avant le traducteur.

## 2. Cœur CMS (Go)

| Composant | Rôle |
|-----------|------|
| Runtime Wasmtime | Charge et exécute les modules traduits, capacités minimales (capability-based). |
| API GraphQL **et** REST | Les deux surfaces dès le départ : même domaine, deux transports. |
| PostgreSQL | Données souveraines : catalogue, commandes, sessions, config modules. Schéma dans `docs/database.md`. Hébergeur indifférent (aujourd’hui IONOS ; OVH, autre cloud ou dédié conviennent). |

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
| Contenu / CMS | pages, blocs, menus, i18n |
| Hooks | `before_*` / `after_*` sur les événements métier |
| Admin | CRUD config module, feature flags, jobs planifiés |
| Intégrations | HTTP sortant **uniquement** via allowlist + quotas (capability `http.fetch`) |
| Stockage module | KV / blobs **scoped** au module, pas le disque hôte |

Hors ABI (refusés par défaut) : shell, filesystem hôte, sockets bruts, lecture de secrets, SQL direct.

Le contrat d’ABI est versionné (`euromni.abi.v1`). Mapping host calls → tables : [`docs/abi.md`](abi.md).

## API headless (REST + GraphQL)

Les deux coexistent et parlent au **même** cœur de domaine :

- **REST** (`/v1/...`) : simple, cacheable, webhooks, intégrations.
- **GraphQL** (`/graphql`) : requêtes boutique (moins de round-trips, sélection de champs).

Pas deux modèles de données : un domaine Go, deux adaptateurs HTTP.

## 3. Front boutique

- Web Components (Custom Elements + Shadow DOM) pour isoler styles et DOM.
- Consomme REST et/ou GraphQL du cœur (pas de couplage à un thème PHP).
- Peut être hébergé séparément (CDN / edge).

## Décisions encore ouvertes

- Périmètre v1 du pipeline (un plugin pilote vs extracteur générique).
- Périmètre produit v1 (moteur seul, boutique, ou bout-en-bout).
- Où tourne Postgres (cloud / dédié) — le schéma ne dépend pas de ça. L’instance actuelle est chez IONOS.
