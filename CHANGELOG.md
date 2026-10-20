# Changelog - EUROMNI dasegole

## 2026-10-20

- Ajout du lowering ciblé PrestaShop `ps_newproducts` vers `catalog.product.newest`, des entrées IR entières et du test PostgreSQL lowering → Wasm → publication → exécution ; la fixture PHP tierce n’est jamais exécutée et la publication dans la stack CMS persistante reste à faire.
- Le runtime lit maintenant `days` et `limit` dans la configuration persistée du module installé, côté hôte et sans les exposer au guest ; le test PostgreSQL vérifie qu’une modification de configuration s’applique sans recharger le Wasm.
- Ajout des endpoints admin protégés et du formulaire owner/admin pour modifier les réglages `ps_newproducts`, avec validation des bornes, isolation par boutique et conservation des autres clés de configuration.

## 2026-10-17

- Ajout d'un spike Rust `no_std` vers `wasm32-unknown-unknown` : import exclusivement `euromni.call`, exports requis et exécution ABI vérifiés dans Wasmtime.
- Vérification locale de deux builds Cargo verrouillés en répertoires propres, identiques octet à octet ; CI générale Linux et matrice native Windows/Linux configurées avec Rust 1.99.1 épinglé.

## 2026-10-12

- Validation de l’incompatibilité Go WASI avec le linker actuel : les imports `wasi_snapshot_preview1` sont présents en plus de `euromni.call`.
- Ajout d’un module minimal de validation de catalogue, sans WASI, avec un seul import autorisé : `euromni.call`.
- Ajout d’un test de runtime montrant qu’un module minimal fonctionne avec capability `catalog` et est refusé sans capability.
- Documentation du protocole de preuve et de la trajectoire de reprise dans les fichiers de pipeline et de documentation.
- La preuve est poussée sur GitHub et constitue la base de reprise avant l’analyse de modules PHP Woo/Presta.
