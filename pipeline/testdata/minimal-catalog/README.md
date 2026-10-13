# Module de validation minimal pour EUROMNI

Ce module de test est conçu pour vérifier le cœur du runtime sans dépendre d’une toolchain locale ni d’un plugin PHP/Presta/Woo réel.

## Objectif

Prouver que le runtime Wasmtime accepte un guest minimal, conforme à l’ABI `euromni.abi.v1`, sans imports système implicites.

## Contrat attendu

- import autorisé : `euromni.call`
- aucun import `wasi_snapshot_preview1`
- export de mémoire : `memory`
- export fonctionnel : `run`
- appel hôte : `catalog.product.get` ou `catalog.product.list`
- capability hôte : `catalog`

## Fichier de référence

Le module minimal est dans :

- `minimal_catalog.wat`

## Validation

Commande de preuve :

```sh
go test -tags=wasmtoolchain -v ./internal/wasm -run TestMinimalCatalogGuestRequiresOnlyEuromniCall
```

## Prochaines étapes

1. vérifier le module en runtime avec un contexte `catalog` autorisé
2. vérifier le rejet sans capability
3. vérifier le rejet après hash ou ABI modifié
4. ensuite seulement explorer des sources PHP Woo/Presta pour générer des modules de production
