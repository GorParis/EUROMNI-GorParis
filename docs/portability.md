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
| Runtime Go + Wasmtime | `go.mod` cible Go 1.23 et depend de `wasmtime-go/v36`. Le binding requiert CGo. | Aucune combinaison OS/architecture n'est encore declaree validee par une matrice de tests. |
| PostgreSQL | Compose utilise PostgreSQL 16 pour le developpement et les tests locaux. | La version seule ne valide ni une offre hebergee ni un profil de production. |
| FileStore | Le CMS et le reconciliateur partagent le meme stockage local par SHA-256. POSIX utilise des permissions de repertoire privees ; Windows configure un DACL NTFS protege. | Les garanties de permissions, atomicite et durabilite doivent etre verifiees sur chaque systeme de fichiers retenu. |

Les cibles ci-dessous sont des lignes de validation, pas des engagements de
support :

| OS / architecture | Runtime Wasmtime natif | FileStore | Statut |
|---|---|---|---|
| Linux / amd64 | A tester avec CGo actif | A tester | Non valide |
| Linux / arm64 | A tester avec CGo actif | A tester | Non valide |
| Windows / amd64 | Tests Go, Wasmtime, integration PostgreSQL et build natif passes avec CGo actif | Tests FileStore passes avec DACL prive | Runtime local et paquet distribue valides le 2026-10-17 ; choix des cibles de production reste ouvert |
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
constitue pas un test du runtime Wasmtime natif sur Linux ni du FileStore Linux.
Linux/macOS et les autres architectures restent non valides pour ces surfaces.
QP-10 reste en cours jusqu'a la decision des cibles de production et la
validation de chaque surface native retenue.

Le responsable produit/technique doit choisir les cibles de deploiement avant
que cette matrice puisse etre reduite a une liste de support. Les cibles non
choisies peuvent etre retirees du perimetre ; aucune n'est implicitement
supportee.

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

Ces decisions ferment QP-01 et QP-10 uniquement apres preuves executees sur
les cibles approuvees. Voir le [plan directeur qualite](plan-directeur-qualite.md)
et le [contrat du pipeline](../pipeline/README.md).
