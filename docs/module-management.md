# Gestion des modules EUROMNI

## Première tranche

L’administration doit permettre de distinguer le registre des builds connus de
leur installation dans la boutique active. Elle affiche les métadonnées
disponibles pour chaque build : nom, écosystème source, version source,
adaptateur, version d’ABI, hash Wasm, capacités accordées et état pour la
boutique. La provenance et les capacités viennent du manifeste de build
persisté ; elles ne sont pas saisies par l’interface.

Cette première tranche est volontairement limitée à la gestion de
`ps_newproducts`, dont l’ingestion, le build Wasm et le comportement catalogue
ont déjà été validés. L’installation n’accepte ni archive ni code depuis
l’administration : elle active uniquement le build PrestaShop déjà publié et
dont l’artefact content-addressed privé existe et passe son contrôle
d’intégrité. Cette intégration du registre est actuellement prise en charge sur
PostgreSQL. Les rôles `owner` et `admin` peuvent installer ou désinstaller.

La désinstallation est réversible : elle fait passer l’installation de la
boutique à l’état `disabled`, sans effacer le registre, la provenance, la
configuration, les données KV ni l’artefact. Une réinstallation réactive le
même build et conserve les réglages existants. Les artefacts sont collectés
séparément par la réconciliation lorsqu’ils ne sont plus référencés et que le
délai de grâce est passé.

Les autres entrées du registre restent visibles en lecture seule. Si un build
n’a pas de manifeste ou d’artefact, l’interface indique que ses métadonnées ou
son cycle de vie ne sont pas disponibles ; elle ne déduit pas un écosystème et
ne propose pas d’action générique.

## Surfaces livrées dans la première tranche

- `GET /v1/admin/modules` retourne le registre et l’état d’installation de la
  boutique, protégé pour `owner` et `admin`.
- `POST /v1/admin/modules/ps_newproducts/installation` vérifie le build et
  relit l’artefact privé avant activation.
- `DELETE /v1/admin/modules/ps_newproducts/installation` désactive uniquement
  l’installation de la boutique.
- `POST /v1/admin/modules/ps_newproducts/build` accepte le SHA-256 d’un build
  immuable disponible et l’active uniquement pour la boutique de la session.
  Avant de modifier le pin, le CMS vérifie le manifeste, les capacités et
  l’intégrité de l’artefact privé. Le changement de build ne modifie pas les
  autres boutiques ; une erreur laisse le pin courant inchangé.
- L’écran d’administration présente la provenance, l’ABI, le hash Wasm et les
  capacités, liste les builds conservés et propose une activation explicite
  uniquement pour les builds pris en charge et récupérables.

Les paramètres existants du widget restent accessibles après installation ;
une désinstallation masque ces paramètres sans les supprimer. L’inventaire
restitue les autres modules en lecture seule, même si leur build ne permet pas
encore une action d’installation.

## Limites et évolutions

Cette gestion n’est pas un gestionnaire d’extensions PHP. Le code PHP amont
n’est ni installé ni exécuté par le CMS ; seule la traduction Wasm explicitement
prise en charge est exécutée. La première tranche ne propose ni catalogue
distant, ni upload arbitraire, ni installation multi-écosystème, ni suppression
physique. Toute nouvelle action de cycle de vie doit vérifier l’identité et le
manifeste du build, ses capacités, l’intégrité de l’artefact et les conséquences
sur les données persistantes.

Les suites envisageables sont l’ajout contrôlé d’autres builds validés, un
historique des opérations et une politique de suppression définitive tenant
compte des installations de toutes les boutiques, des données KV et des
références aux artefacts.
