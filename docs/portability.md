# Portabilite du runtime et de la toolchain

Ce document distingue la portabilite des modules Wasm de celle du processus
serveur EUROMNI. Une cible n'est supportee qu'apres execution des controles
indiques ci-dessous ; la presence d'une image ou d'une dependance ne constitue
pas une preuve de support.

## Frontieres de portabilite

1. **Guest Wasm** : le module publie est un artefact `wasm32` et ne doit
   importer que `euromni.call`. Il ne doit pas dependre de WASI, de CGo, du
   systeme de fichiers ou du reseau de l'hote.
2. **Runtime hote** : le CMS et le runtime Wasmtime sont des binaires Go natifs.
   Le binding `wasmtime-go/v36` utilise CGo ; le binaire hote doit donc etre
   construit et execute avec une combinaison OS/architecture et une toolchain C
   effectivement validees.
3. **Worker de build** : il peut etre distinct de l'hote de production. Seules
   les proprietes verifiables du module produit (imports, ABI, hash et
   reproductibilite) conditionnent sa publication. Docker n'est pas une
   dependance de production.

`CGO_ENABLED=0` dans une commande de compilation du guest ne signifie pas que
le serveur Go EUROMNI peut etre construit sans CGo.

## Etat et matrice initiale

| Surface | Etat constate | Support declare |
|---|---|---|
| Guest Wasm | Le WAT minimal et l'IR generent un module utilisant `euromni.call`. La fixture minimale est le seul contrat de guest actuellement prouve. | Format cible du pipeline, pas une declaration de support pour un compilateur de code source arbitraire. |
| Go `wasip1` vers Wasm | Le spike consigne avec Go 1.26.8 retrouve `euromni.call` et des imports `wasi_snapshot_preview1` ; cette configuration ne respecte pas le linker strict. | Rejete pour cette configuration precise ; les autres compilateurs et versions restent a evaluer. |
| Runtime Go + Wasmtime | `go.mod` cible Go 1.23 et depend de `wasmtime-go/v36`. Le binding requiert CGo. | Windows/amd64 et Linux/amd64 sont les cibles runtime qualifiees ; tests natifs, PostgreSQL 16, packaging et smoke tests CI sont requis sur les deux. |
| PostgreSQL | Compose utilise PostgreSQL 16 pour le developpement et les tests locaux. | La version seule ne valide ni une offre hebergee ni un profil de production. |
| FileStore | Le CMS et le reconciliateur partagent le meme stockage local par SHA-256. POSIX utilise des permissions de repertoire privees ; Windows configure un DACL NTFS protege. | Les garanties de permissions, atomicite et durabilite doivent etre verifiees sur chaque systeme de fichiers retenu. |

Les cibles ci-dessous sont des lignes de validation, pas des engagements de
support :

| OS / architecture | Runtime Wasmtime natif | FileStore | Statut |
|---|---|---|---|
| Linux / amd64 | Tests natifs, Wasmtime et PostgreSQL integres au workflow de distribution | Integration et FileStore passes sur runner natif | Qualifie par le run 39032334862 ; revalide a chaque workflow de distribution |
| Linux / arm64 | A tester avec CGo actif | A tester | Non valide |
| Windows / amd64 | Tests Go, Wasmtime, integration PostgreSQL et build natif passes avec CGo actif | Tests FileStore passes avec DACL prive | Qualifie par le run 39032334862 ; revalide a chaque workflow de distribution |
| macOS / amd64 et arm64 | A tester avec CGo actif | A tester | Non valide |

### Preuves executees le 2026-10-17

Environnement local : Go 1.27.2, `GOOS=windows`, `GOARCH=amd64`,
`CGO_ENABLED=1`, compilateur C `gcc`.

- `go test ./...` : passe.
- `go vet ./...` : passe.
- `go test -tags=integration ./...` : passe avec PostgreSQL 16 embarque pour
  les suites d'integration.
- `go build ./cmd/...` : passe ; les commandes natives se compilent avec CGo.
- `go test ./internal/pipeline` : passe apres ajout de l'inspection des
  imports des artefacts Wasm statique et dynamique generes depuis l'IR.
- `go test -tags=wasmtoolchain ./internal/wasm -run 'TestMinimalCatalogGuest(RequiresOnlyEuromniCall|RuntimeExecution)$'` :
  passe ; le guest minimal n'importe que `euromni.call` et n'obtient pas la
  donnee du catalogue sans la capability `catalog`.

Le run natif [CMS standalone distribution #39017221737](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39017221737)
a egalement extrait et lance les paquets CMS amd64 sur Ubuntu 22.04 et Windows
Server 2022 ; `/health`, `/admin/` et `/v1/products` ont repondu HTTP 200. Le
paquet Windows du run initial a ete installe contre PostgreSQL 16.15. Ces
resultats valident les distributions CMS, mais le smoke test sans base ne
constituait pas un test du runtime Wasmtime natif sur Linux ni du FileStore
Linux ; ces surfaces ont ensuite ete couvertes par le run de qualification
ci-dessous. Linux/arm64, macOS et les autres architectures restent non valides.

Le responsable produit a retenu Windows/amd64 et Linux/amd64 comme cibles a
qualifier. Le workflow `cms-distribution.yml` execute la suite d'integration
PostgreSQL, les builds CGo et les smoke tests des archives sur les deux runners.
Le run corrige
[39032334862](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032334862)
a reussi ses deux jobs : le client PostgreSQL 16 a ete installe depuis PGDG sur
Ubuntu 22.04, les integrations ont passe sur les deux OS, les distributions
ont ete construites et extraites, et les smoke tests HTTP ont repondu avec
succes. Les deux artifacts Windows/Linux ont ete publies par le run.

Le run precedent
[39032009205](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032009205)
avait reussi completement le job Windows, mais le job Linux s'etait arrete avant
les integrations faute du paquet `postgresql-client-16` dans les depots Ubuntu
actifs. Le workflow ajoute maintenant le depot officiel PGDG. La CI generale
sur ce correctif a egalement reussi
([39032300222](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032300222)).

La matrice technique Windows/amd64 et Linux/amd64 est validee sur les runners
natifs et les filesystems locaux testes. Cela ne qualifie pas Linux/arm64,
macOS, les filesystems reseau ni tous les environnements de production ;
aucune garantie n'est etendue au-dela des surfaces explicitement testees.
La qualification concerne les configurations de runners et les filesystems
locaux testes ; elle ne certifie pas toutes les versions de Windows/Linux,
toolchains C ou environnements de production.

Un pilote local Windows sans privileges administrateur est documente dans
[`local-postgresql.md`](local-postgresql.md). Il utilise PostgreSQL 16.9 issu
de la fixture embarquee de developpement ; ce n'est pas une installation
serveur de production et il n'inclut pas `pg_dump`/`pg_restore`.

Sur Windows, les bits de permission POSIX exposes par `os.FileMode` ne
representent pas l'ACL NTFS et `os.Chmod(0600)` ne protege pas les artefacts.
Le FileStore cree donc son repertoire racine avec un DACL explicite autorisant
le compte courant, SYSTEM et les administrateurs, protege contre l'heritage ; il
refuse une racine existante dont le proprietaire ou les entrees ACL ne
correspondent pas. Les sous-repertoires et fichiers heritent cette ACL. Cette
validation a ete executee sur Windows/amd64 ; les garanties des partages reseau
et des autres systemes de fichiers ne sont pas declarees.

## Toolchain invitee actuelle

`CompileIRToWasm` traduit l'IR valide en WAT puis appelle
`wasmtime.Wat2Wasm`. Cette voie compile l'IR actuellement supporte ; elle ne
constitue pas un compilateur PHP generique ni une preuve qu'un binaire Go
arbitraire peut etre cible vers un guest importe uniquement par EUROMNI.

Le test `TestGoWASIP1ToolchainImportSet` est une preuve de diagnostic pour une
configuration precise, pas un test de compatibilite generale. Ne pas retirer
les imports WASI de la politique du runtime pour faire passer ce guest. Une
nouvelle toolchain candidate devra fournir une fixture qui appelle l'ABI et
dont la liste complete d'imports est inspectee avant execution.

### Candidate Rust `wasm32-unknown-unknown`

Une preuve expérimentale compile `pipeline/testdata/rust-euromni/minimal_catalog.rs`
avec Rust `1.99.1`, `--target wasm32-unknown-unknown`, `#![no_std]` et un import
explicite `euromni.call`. Le test inspecte l'ensemble des imports Wasm, exige
les exports `memory` et `run`, puis exécute l'appel au travers de Wasmtime.
Le projet Cargo embarque son lockfile, n'a aucune dépendance externe et se
construit avec `--frozen`. Le test compile deux fois les mêmes sources dans des
répertoires de sortie distincts et compare les octets Wasm. L'absence de WASI
est une propriété vérifiée sur l'artefact généré, pas une garantie tirée du
seul nom de target.

Preuve locale exécutée sur Windows/amd64 : Rust `1.99.1
(58cf8f9c3 2026-10-13)`, Cargo en mode `--frozen`. Deux répertoires de build
vierges produisent des artefacts identiques, avec import unique et appel ABI
exécuté dans Wasmtime. Commandes reproductibles :

```powershell
rustup toolchain install 1.99.1 --profile minimal --target wasm32-unknown-unknown
$env:RUSTUP_TOOLCHAIN = "1.99.1"
go test -v -tags=wasmtoolchain -count=1 ./internal/wasm -run '^TestRustUnknownUnknownGuestImportsOnlyEuromniCallAndExecutes$'
```

La CI générale a réussi sur Linux amd64 (run
[39030961826](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39030961826)).
Le premier run matriciel
[39031153173](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39031153173)
a exécuté avec succès le test Rust sur les deux runners, mais les intégrations
complètes ont révélé deux problèmes de runner : `pg_dump` 14 face au PostgreSQL
16 embarqué sous Ubuntu, et une course de première extraction du cache PostgreSQL
sur Windows quand plusieurs packages s'exécutent en parallèle. Le workflow
installe maintenant `postgresql-client-16` depuis le dépôt officiel PGDG sous
Linux et utilise `go test -p 1` pour les intégrations. Le run de confirmation
[39032009205](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032009205)
a échoué sous Ubuntu avant les intégrations : Ubuntu 22.04 ne fournit pas
`postgresql-client-16` dans ses dépôts activés par défaut. Le workflow ajoute
désormais le dépôt PGDG. Le run de qualification corrigé
[39032334862](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032334862)
a passe ses deux jobs natifs, y compris le test Rust sur Windows et Linux. Le
workflow CI ordinaire est maintenant configure pour executer aussi la preuve
de guest sur ces deux cibles. Le test impose Rust 1.99.1 depuis
`rust-toolchain.toml`, Cargo `--frozen`, deux repertoires de sortie propres,
une egalite octet a octet et
journalise les versions ainsi que le SHA-256 du guest. Le workflow de
distribution produit un fichier de sommes SHA-256 adjacent a chaque archive ;
il detecte une corruption accidentelle mais ne constitue pas une attestation
signee de provenance.

Le 2026-10-21, le test de reproductibilite a ete execute localement sur
Linux/amd64 avec Go 1.26.10, Rust 1.99.1 et Cargo 1.99.1 : deux builds propres
identiques, SHA-256
`4df40e34e013652f30cfd8a3a262279b7554d1075bbe8bb62d02cf67166328d7`, imports
verifies et execution Wasmtime reussie.

La CI ordinaire
[39580872372](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39580872372)
passe `go vet`, `go test ./...` et le test Rust/Wasm sur Linux/amd64 et
Windows/amd64. Le workflow de distribution
[39581659670](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39581659670),
sur le commit `6610399`, passe les integrations PostgreSQL 16, les builds CGo
et smoke tests HTTP des distributions Linux et Windows. Les deux artefacts
publies sont `euromni-cms-linux-amd64` et `euromni-cms-windows-amd64` ; chacun
inclut l'archive et son fichier `.sha256`. Les artefacts sont retenus 30 jours
par GitHub Actions. Cette preuve qualifie les runners et la configuration de
build testees, pas toutes les installations de production.

La validation locale de la restauration complete reste limitee par les
executables `pg_dump`/`pg_restore` disponibles en version 18.6, alors que le
serveur de test embarque est PostgreSQL 16. La reussite de l'integration avec
le client 16 est attestee par le runner Linux du workflow de distribution,
pas par cette machine locale.

Cette qualification reste un prototype de guest minimal, pas une toolchain de
traduction de modules metier. QP-01 reste en cours jusqu'a ce qu'un processus
de build/promotion consomme une source/version explicite, valide les imports et
l'ABI, conserve le manifeste de provenance et fournisse un artefact auditable.
Ni le ZIP PHP ni le compilateur Wasm actuel ne sont remplaces par cette fixture.

## Criteres de validation d'une cible hote

Pour chaque OS/architecture retenu :

1. Consigner les versions de Go, du compilateur C, de Wasmtime et de
   PostgreSQL, ainsi que `GOOS`, `GOARCH` et `CGO_ENABLED`.
2. Construire le CMS et le reconciliateur avec CGo actif sur la cible, puis
   executer `go test ./...` et `go vet ./...`.
3. Executer `go test -tags=integration ./...` sur PostgreSQL 16, et verifier
   sur le systeme de fichiers cible la publication, la lecture verifiant le
   hash, la reconciliation, les permissions et le comportement apres
   interruption.
4. Executer le test Wasm minimal et le parcours IR → publication → chargement
   Wasmtime. Inspecter les imports et confirmer qu'ils se limitent a
   `euromni.call`.
5. Compiler deux fois le meme input avec la meme toolchain epinglee et comparer
   les hashes Wasm. Enregistrer toute exception a la reproductibilite avant
   d'accepter la toolchain.

Les commandes de test exactes et les preuves doivent etre liees a la cible
testee. Un test non execute ou une cible non disponible reste non validee.

## Decisions restant a prendre

- Les OS et architectures pris en charge en production.
- Le compilateur C et la chaine de distribution des binaires CGo pour chaque
  cible.
- La toolchain qui traduira les fonctions metier vers des guests sans imports
  implicites ; Go `wasip1` avec la configuration testee n'est pas compatible.
- Les systemes de fichiers ou services de stockage pris en charge pour les
  artefacts et leurs garanties de durabilite.

QP-01 reste ouvert jusqu'a la qualification du processus de build/promotion
des guests metier. La qualification QP-10 s'applique uniquement aux runners
Windows/amd64 et Linux/amd64 ainsi qu'aux filesystems locaux testes ; toute
extension du support de production demande des preuves supplementaires. Voir
le [plan directeur qualite](plan-directeur-qualite.md) et le
[contrat du pipeline](../pipeline/README.md).
