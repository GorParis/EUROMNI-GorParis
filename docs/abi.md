# Mapping ABI Wasm → SQL (`euromni.abi.v1`)

Le module Wasm n’exécute **aucune** requête. Il appelle un nom de fonction hôte. Le processus Go vérifie la capability (`shop_modules.capabilities`), puis lit / écrit Postgres.

Source code : [`internal/wasm/mapping.go`](../internal/wasm/mapping.go).  
Données exemple : [`migrations/0002_seed_example.sql`](../migrations/0002_seed_example.sql).

```
[module Wasm] --host call--> [Go / Wasmtime] --SQL--> [PostgreSQL]
```

## Boutique exemple

| Entité | ID fixe | Valeur |
|--------|---------|--------|
| Shop | `e0e00000-0001-4000-8000-000000000001` | `atelier-demo` |
| Client | `…000020` | `lea@client.example` |
| Produit | `…000040` | `cabas-lin` |
| Variante | `…000042` | SKU `CABAS-LIN-NAT`, 89,00 EUR |
| Commande | `…000060` | `AD-1001` (paid) |
| Module | `…000080` | `demo.loyalty` |

Exemple : `catalog.product.get` avec `product_id` = `e0e00000-0001-4000-8000-000000000040` → cabas démo.

## Contrat JSON (enveloppe)

Le guest n’envoie **que** cet objet. `shop_id`, `module_id` et les capabilities **ne sont pas** dans le JSON : le runtime Go les injecte depuis `shop_modules`.

Requête :

```json
{
  "abi": "euromni.abi.v1",
  "call": "catalog.product.get",
  "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
  "input": {
    "product_id": "e0e00000-0001-4000-8000-000000000040"
  }
}
```

- `abi` : littéral `euromni.abi.v1`
- `call` : nom allowlisté (voir mapping)
- `id` : UUID de corrélation optionnel
- `input` : objet, champs stricts (champs inconnus → `invalid`)
- taille max : 16 KiB
- montants : **string** (`"89.0000"`), jamais un float JSON

## Prévisualisation catalogue Selfnamed (prototype)

Le guest spécialisé reçoit une enveloppe `catalog.supplier.product.preview`
dont `input.product_json` contient le JSON fournisseur comme chaîne. Il exige
`id`, `name`, `price`, `volume`, `weight` et `isInStock`. L’entrée fournisseur
est limitée à 8 KiB ; `id` et `price` doivent être des nombres JSON (pas des
chaînes), `id` est un entier positif d’au plus 20 chiffres et `price` accepte
au plus 12 chiffres entiers et 4 décimales. Les poids/volumes négatifs ou hors
bornes sont refusés. Les champs image sont ignorés et aucun téléchargement
n’est effectué.

Le mapping Rust `no_std` du guest produit le brouillon, le nom, la description,
le prix décimal normalisé, le statut de stock sans quantité, le poids en
grammes, le volume et les tags. Devise, locale et unité du volume restent `null`
et sont listées dans `unresolved`. Le guest renvoie ce DTO par
`catalog.supplier.product.preview.result`. Cet appel hôte valide strictement
les champs et invariants du DTO puis le renvoie dans `data` ; il ne lit ni
n’écrit aucune table. La capability `catalog` est requise, mais aucun accès SQL,
réseau ou filesystem n’est accordé. Les images, catégories, publication
persistante et synchronisation réelle restent hors périmètre.

Le handler Go de `catalog.supplier.product.preview` reste une implémentation
nativement testable et un oracle de caractérisation ; le test Wasmtime du guest
Rust n’utilise pas ce handler pour calculer sa sortie. Cela ne traduit ni
n’exécute le PHP du plugin Selfnamed.

## Liaison Wasmtime

Un module chargé par le runtime est compilé, puis lancé via un export sans
argument choisi par l’hôte. Le linker n’expose qu’un import, sans WASI ni
accès système :

```wat
(import "euromni" "call"
  (func $euromni_call
    (param i32 i32 i32 i32)
    (result i32)))
(memory (export "memory") 1)
```

Les paramètres de `euromni.call` sont `input_ptr`, `input_len`, `output_ptr` et
`output_capacity`, tous des `i32` dans la mémoire linéaire exportée sous le nom
`memory`. L’entrée est une enveloppe JSON v1 ; la sortie est une réponse JSON
v1. Le résultat est le nombre d’octets écrits, ou `-1` si une plage mémoire est
invalide ou si la sortie ne tient pas dans le tampon.

Limites actuelles par exécution : enveloppe de 16 KiB, réponse de 1 MiB,
mémoire de 64 MiB et 10 millions d’unités de fuel Wasmtime. L’hôte revalide
l’enveloppe, l’appel allowlisté, la capability et le contexte boutique avant
toute opération. Aucun autre import n’est lié ; une dépendance WASI ou système
fait échouer l’instanciation.

## Contrat du pilote Wasm de tarification transport

Le pilote `demo.shipping.free-over-100` est un module embarqué distinct du
chargeur générique `shop_modules`. Il est compilé depuis
[`internal/wasm/free_shipping_threshold.wat`](../internal/wasm/free_shipping_threshold.wat)
et exécuté sans import hôte, via son export `run`. Son entrée JSON contient
`subtotal_units` et `base_units`, deux entiers non négatifs exprimés en unités
de 1/10 000. La règle renvoie le tarif de base sous 100 unités de la devise,
sinon zéro. Le contrat métier est `euromni.shipping.v1` ; l’enveloppe de
l’appel utilise l’ABI `euromni.abi.v1` et le nom `shipping.rate.quote`.

L’activation est opt-in avec `EUROMNI_SHIPPING_WASM_POLICY=free-over-100`.
L’hôte transmet uniquement les deux montants, vérifie l’identité et le hash du
module, puis recalcule le résultat autorisé avant d’accepter le devis. Le
snapshot de module est incorporé au devis et conservé sur la commande.
L’activation n’affecte que les devis de panier et les commandes, pas la liste
de tarifs sans panier. Le module ne lit aucune donnée et n’autorise aucune
opération hôte ; ce contrat est un pilote ciblé, pas une extension de
l’allowlist générique décrite ci-dessous.

## Chargement et autorisation

`Runtime.LoadInstalled` ne charge qu’une association `shop_modules` au statut
`installed`, pour la boutique et le module demandés. Le chargeur vérifie
`modules.abi_version`, exige un SHA-256 enregistré et le compare à l’artefact,
puis refuse toute capability inconnue. L’artefact est fourni au chargeur ; son
stockage durable et son téléchargement ne sont pas encore définis.

`Runtime.Run` relit le statut, la version ABI, le hash et les capabilities dans
PostgreSQL à chaque invocation. Une capability retirée s’applique donc à
l’appel suivant ; un module désactivé est refusé ; un hash modifié impose un
rechargement et une nouvelle vérification avec `Runtime.ReloadInstalled`. Le
remplacement attend la fin des exécutions en cours. Le contexte de confiance
n’est pas accepté depuis le guest ni conservé comme source d’autorité après
chargement.

Succès :

```json
{
  "ok": true,
  "abi": "euromni.abi.v1",
  "call": "catalog.product.get",
  "id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
  "data": {
    "id": "e0e00000-0001-4000-8000-000000000040",
    "slug": "cabas-lin",
    "status": "active",
    "translations": [{"locale": "fr-FR", "name": "Cabas en lin", "description": "Cabas quotidien, anse cuir."}],
    "media": [{"uri": "https://cdn.example/atelier-demo/cabas-lin.jpg", "mime_type": "image/jpeg", "alt": "Cabas en lin naturel"}],
    "variants": [{
      "id": "e0e00000-0001-4000-8000-000000000042",
      "sku": "CABAS-LIN-NAT",
      "options": {"color": "naturel", "size": "unique"},
      "prices": [{"currency": "EUR", "amount": "89.0000", "compare_at": "110.0000"}],
      "inventory": {"quantity": 24, "track": true}
    }]
  }
}
```

Erreur :

```json
{
  "ok": false,
  "abi": "euromni.abi.v1",
  "call": "catalog.product.get",
  "error": { "code": "denied", "message": "capability missing" }
}
```

Codes : `invalid`, `too_large`, `denied`, `not_found`, `unimplemented`, `internal`.  
`internal` ne contient jamais le message SQL / OS.

## Règles de sécurité (non négociables)

- Pas de SQL, pas de DSN, pas d’HTTP ABI public (le contrat n’est pas une API REST anonyme).
- `shop_id` uniquement depuis le contexte hôte ; mauvais shop → `not_found` (pas d’oracle d’existence).
- Capability `catalog` obligatoire pour les calls catalogue, dont
  `catalog.product.get`, `catalog.supplier.product.preview` et
  `catalog.supplier.product.preview.result`.
- UUID uniquement (rejet des payloads type injection).
- Requêtes Postgres paramétrées (`$1`, `$2`) ; `status = 'active'` seulement (pas de brouillons).
- Timeout hôte 2 s ; champs JSON inconnus refusés.
- `http.fetch` et le paiement restent hors de ce call.

## Table de mapping

| Capability | Appels hôte | Tables |
|------------|-------------|--------|
| `catalog` | `catalog.product.list/get/upsert`, `catalog.category.list`, `catalog.variant.get`, `catalog.inventory.adjust` | `products`, `product_translations`, `product_categories`, `categories`, `category_translations`, `variants`, `prices`, `inventory_items`, `media`, `product_media` |
| `catalog` | `catalog.supplier.product.preview`, `catalog.supplier.product.preview.result` | *(aucune table : projection guest validée et non persistée)* |
| `cart` | `cart.get`, `cart.add_line`, `cart.update_line`, `cart.clear` | `carts`, `cart_lines` (+ lecture `variants`, `prices`) |
| `order` | `order.list/get/place/set_status`, `order.shipment.upsert` | `orders`, `order_lines`, `payments`, `shipments`, `inventory_items`, `domain_events` |
| `customer` | `customer.get/upsert`, `customer.address.list`, `customer.consent.set`, `session.get` | `customers`, `addresses`, `customer_consents`, `sessions` |
| `payment` | `payment.initiate`, `payment.capture` | `payments`, `orders` — secrets PSP uniquement côté Go |
| `content` | `content.page.get/upsert`, `content.menu.get` | `pages`, `page_translations`, `menus`, `menu_items` |
| `hooks` | `hooks.emit`, `hooks.consume` | `domain_events` |
| `admin` | `admin.shop.get`, `admin.module.config.get/set` | `shops`, `shop_locales`, `shop_modules` |
| `http.fetch` | `http.fetch` | *(aucune table)* allowlist + quotas dans le hôte |
| `module.kv` | `kv.get/put/delete` | `module_kv` (clé déjà scopée `shop_id` + `module_id`) |

## Interdit (pas de call)

`SELECT`/`INSERT` bruts, filesystem, sockets, lecture `users.password_hash`, credentials PSP.

## Scope obligatoire

Chaque call hôte reçoit implicitement `shop_id` (et `module_id` pour le KV). Un module `demo.loyalty` ne peut pas lire le KV d’un autre module ni une autre boutique.
