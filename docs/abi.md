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
- Capability `catalog` obligatoire pour `catalog.product.get`.
- UUID uniquement (rejet des payloads type injection).
- Requêtes Postgres paramétrées (`$1`, `$2`) ; `status = 'active'` seulement (pas de brouillons).
- Timeout hôte 2 s ; champs JSON inconnus refusés.
- `http.fetch` et le paiement restent hors de ce call.

## Table de mapping

| Capability | Appels hôte | Tables |
|------------|-------------|--------|
| `catalog` | `catalog.product.list/get/upsert`, `catalog.category.list`, `catalog.variant.get`, `catalog.inventory.adjust` | `products`, `product_translations`, `product_categories`, `categories`, `category_translations`, `variants`, `prices`, `inventory_items`, `media`, `product_media` |
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
