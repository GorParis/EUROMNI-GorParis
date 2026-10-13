# Changelog - EUROMNI dasegole

## 2026-10-12

- Validation de l’incompatibilité Go WASI avec le linker actuel : les imports `wasi_snapshot_preview1` sont présents en plus de `euromni.call`.
- Ajout d’un module minimal de validation de catalogue, sans WASI, avec un seul import autorisé : `euromni.call`.
- Ajout d’un test de runtime montrant qu’un module minimal fonctionne avec capability `catalog` et est refusé sans capability.
- Documentation du protocole de preuve et de la trajectoire de reprise dans les fichiers de pipeline et de documentation.
- La preuve est poussée sur GitHub et constitue la base de reprise avant l’analyse de modules PHP Woo/Presta.
