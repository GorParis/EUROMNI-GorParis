# Progression – validation du module minimal

## État actuel

- le runtime Wasmtime est déjà validé avec des fixtures internes
- le projet a documenté l’incompatibilité Go `wasip1` avec le linker actuel
- un module minimal de référence a été ajouté pour valider la couche `guest -> runtime -> host`

## Ce qui a été créé

- [pipeline/testdata/minimal-catalog/minimal_catalog.wat](../pipeline/testdata/minimal-catalog/minimal_catalog.wat) : module WAT de validation minimal
- [pipeline/testdata/minimal-catalog/README.md](../pipeline/testdata/minimal-catalog/README.md) : explication du motif et des étapes suivantes
- test de validation dans [internal/wasm/toolchain_spike_test.go](../internal/wasm/toolchain_spike_test.go)

## Objectif de la preuve

Le module minimal doit démontrer :

- imports autorisés strictement limités à `euromni.call`
- absence de WASI implicite
- exécution possible sur le runtime avec capability `catalog`
- blocage sans capability
- blocage si hash ou ABI est modifié

## Étapes validées

1. test d’imports du module Go WASI : confirmé incompatible (imports WASI supplémentaires)
2. test d’imports du module minimal : confirmé conforme (`euromni.call` seulement)
3. exécution runtime du module minimal : confirmée avec capability `catalog`, puis réponse ABI refusée sans capability
4. validateur d’archive PHP : confirmé pour refuser les chemins dangereux, fichiers non autorisés et paquets trop volumineux
5. parse du manifest PHP : confirmé pour les champs minimaux requis (`name`, `version`, `platform`, `shop`)
6. analyse statique de hooks PHP WooCommerce : confirmée pour repérer `add_action` / `add_filter` et dériver des capabilities attendues (`cart`, `catalog`)
7. analyse statique PrestaShop : confirmée sur `ps_emailalerts` et `ps_cashondelivery`, incluant `registerHook('...')`, la constante `HOOKS`, la classification `action` / `display` / `other` et l’association aux méthodes `hook…` réelles
8. analyse Shopify : types et cibles de Discount Function et extension checkout lus depuis leurs manifests TOML réels
9. analyse Magento : nom du module, événements, plugins et arguments DI lus depuis les fichiers XML réels de Japan Tax et gRPC
10. IR v2 → Wasm : appel catalogue unique validé contre l’allowlist, capability dérivée de `wasm.Mapping`, provenance versionnée et génération WAT/Wasm reproductible
11. lowering explicite : `woocommerce_product_query` abaissé vers `catalog.product.list`; les triggers sans règle sont refusés
12. exécution installée : Wasm généré chargé par `Runtime.LoadInstalled`, puis exécuté par `Runtime.Run` avec hash et capability `catalog` provenant de PostgreSQL
13. entrée IR dynamique : champ trigger `product_id` typé UUID validé, résolu en enveloppe ABI et transmis à `run(ptr,len)` par `Runtime.RunWithInput`
14. provenance IR : hash SHA-256 calculé sur les octets PHP analysés ; le lowering rejette un hash source différent, une version plateforme manquante ou un adaptateur qui ne correspond pas à la plateforme
15. manifeste de build v1 : lie provenance source, hash IR, ABI, capabilities dérivées et hash/taille Wasm ; sérialisation et vérification testées
16. publication de build : Wasm écrit d’abord dans le FileStore content-addressed, puis hash Wasm et manifeste JSONB enregistrés ensemble dans `modules`
17. réconciliation : hashes référencés lus dans PostgreSQL ; seuls les objets absents du registre et plus anciens que le délai de grâce peuvent être supprimés
normal, tu ne m'as demandé de mt18. ordonnanceur autonome ajouté sous `cmd/artifact-reconciler` : passage immédiat puis périodique, arrêt SIGINT/SIGTERM, retry après erreurs transitoires
19. politique provisoire : grâce 7 jours, cadence normale 24 heures, retry 5 minutes ; valeurs configurables et à réviser avant production

Les fixtures PrestaShop sont des snapshots des dépôts officiels GitHub, épinglés aux commits `359cd5c86c498964d9c38c97db9fcd13d73be3fe` et `d1bac20a8b5a4664759db5c6775be9b426ad4b62`. Elles valident l'analyse statique des fichiers PHP réels ; elles ne valident pas encore l'ingestion d'un package PrestaShop Addons ni la traduction en Wasm.

La référence de compatibilité est la documentation développeur PrestaShop [`9.x`](https://devdocs.prestashop-project.org/9/modules/concepts/hooks/), qui décrit l’enregistrement, les familles `action` / `display` et les méthodes handler. Les fixtures montrent une exception (`paymentOptions`) qui reste sans sémantique déduite. Aucune documentation développeur publique `10.x` n’a été trouvée au 13 octobre 2026 ; PS10 n’est donc pas déclaré compatible.

Les références Shopify et Magento sont épinglées et conservées dans [pipeline/fixtures/real-modules](../pipeline/fixtures/real-modules) avec leurs licences. `TestAnalyzeRealPlatformFixtures` les vérifie depuis le dépôt, sans réseau ni dépendance à `/tmp`. Les tests n’exécutent pas les apps, extensions ou modules ; ils valident uniquement la lecture de leurs manifests et configurations.

La tranche IR v2 compile les entrées statiques et sait résoudre une référence dynamique UUID pour `catalog.product.get`. Sa provenance comprend les hashes PHP et archive, plus versions plateforme/adaptateur. `PublishModuleBuild` stocke le Wasm puis enregistre hash et manifeste JSONB dans une mise à jour SQL atomique. Le binaire autonome `cmd/artifact-reconciler` lance la réconciliation immédiatement puis à chaque intervalle configuré ; ses défauts provisoires sont 7 jours de grâce et 24 heures de cadence, à revoir avant production. Son test de scheduler couvre le cycle/réessai ; le test d’intégration complet du chemin publication/runtime se lance avec `go test -tags=integration ./internal/pipeline -run TestIRGeneratedWasmRunsAsInstalledModule`. Seul le hook WooCommerce `woocommerce_product_query` est abaissé, vers une opération statique ; les autres plateformes et les conditions restent à traiter.

## Prochaines étapes prévisibles

1. vérifier le rejet avec hash ou ABI modifié
2. vérifier le rejet si module non installé ou module retiré du registre
3. étendre les schémas dynamiques aux données métier nécessaires et vérifier leur type
4. définir des règles explicites de lowering pour d’autres triggers et vérifier leur sémantique
5. revoir les valeurs provisoires en vue de la production et planifier la réconciliation dans le cycle du CMS
6. ajouter conditions et plusieurs opérations seulement après définition de leur contrat

## Remarque

Le module minimal n’est pas un plugin métier complet ; c’est une preuve de sécurité et de compatibilité. Les analyseurs des quatre plateformes sont des preuves de lecture statique ciblée, pas des modules traduits ni exécutables dans le runtime.
