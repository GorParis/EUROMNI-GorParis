# Courriels transactionnels

## Première tranche

Le checkout SQL enregistre un événement `order_confirmation` dans
`email_outbox`, dans la même transaction que la commande et la consommation du
panier. Un rollback du checkout annule donc aussi le courriel. L’événement
contient un snapshot minimal du destinataire et du récapitulatif de commande ;
aucune adresse de livraison, donnée de paiement ou donnée de carte n’est
incluse.

Le processus séparé `cmd/mail-worker` réclame les messages avec un bail,
produit une version texte et une version HTML localisées en français ou anglais,
puis les remet au serveur SMTP. Les erreurs temporaires sont retentées avec un
délai exponentiel à partir de 5 secondes. Après huit tentatives, ou en cas de
message invalide, l’événement passe à `failed` et nécessite une intervention
opérationnelle. Le bail dure 45 secondes ; le timeout SMTP est de 15 secondes.

La remise est **au moins une fois**, pas exactement une fois : si le serveur
accepte le message mais que le worker s’arrête avant d’enregistrer `sent`, une
reprise après expiration du bail peut produire un doublon. La clé d’idempotence
est persistée et incluse indirectement dans un `Message-ID` stable, mais SMTP
ne garantit pas la déduplication. Les lignes restent dans l’outbox après succès
ou échec ; aucune politique de rétention ou interface de rejeu n’est encore
définie.

Sans worker SMTP configuré, les courriels SQL s’accumulent en attente ; le
checkout ne bloque pas sur la disponibilité du transport. En mode mémoire,
aucun événement durable n’est créé. Le worker exige une base SQL et une
configuration SMTP complète.

## Essai local avec Mailpit

Lancer la pile sous un nom de projet dédié pour isoler son volume PostgreSQL de
toute base locale persistante. Préparer d’abord un secret de base dans un fichier
`.env` privé et ignoré par Git, puis lancer :

```sh
podman compose -p euromni-mail-test --profile cms --profile mail \
  up -d postgres cms mailpit mail-worker
podman compose -p euromni-mail-test port mailpit 8025
```

Ouvrir l’adresse loopback affichée par `port` pour consulter les messages. Le
worker utilise le SMTP privé du réseau Compose (`mailpit:1025`), sans
authentification ni TLS ; cette configuration n’est acceptée qu’en
`development`. L’interface Mailpit n’est publiée que sur loopback. Configurer
un transporteur dans la boutique de test, puis passer une commande dans la
vitrine pour observer sa confirmation. Ne pas lancer ce scénario avec le nom de
projet de la stack persistante.

## Transport SMTP

Configuration du worker par variables d’environnement :

| Variable | Requise | Description |
|---|---:|---|
| `DATABASE_URL` | oui | Connexion PostgreSQL ou MariaDB migrée |
| `EUROMNI_MAIL_SMTP_ADDR` | oui | Hôte et port SMTP |
| `EUROMNI_MAIL_FROM` | oui | Adresse expéditeur |
| `EUROMNI_MAIL_TLS_MODE` | non | `starttls` par défaut, `implicit` pour TLS implicite, `none` uniquement en développement |
| `EUROMNI_MAIL_SMTP_USERNAME` | non | Identifiant SMTP ; doit être fourni avec le mot de passe |
| `EUROMNI_MAIL_SMTP_PASSWORD` | non | Secret SMTP ; requis avec l’identifiant |
| `EUROMNI_MAIL_POLL_INTERVAL` | non | `2s` par défaut, entre `250ms` et `1m` |
| `EUROMNI_ENVIRONMENT` | selon mode | Doit valoir `development` pour `TLS_MODE=none` |

Le mode STARTTLS, utilisé par défaut, exige que le serveur annonce STARTTLS et
valide son certificat TLS avec le magasin de confiance système. Le mode
`implicit` démarre directement en TLS. L’authentification ne peut être utilisée
qu’avec TLS.

La configuration de production n’est pas encore opérationnelle : aucun domaine,
serveur SMTP réel ou expéditeur n’a été choisi. L’hébergement d’un MTA
auto-administré est distinct du worker et n’est pas fourni par EUROMNI. Avant
un envoi public, choisir l’hôte et l’identité d’expéditeur, configurer SPF,
DKIM, DMARC et le DNS inverse selon le fournisseur, protéger les secrets,
valider la délivrabilité, ainsi que définir la rétention, les rebonds, les
plaintes, les limites d’envoi et la supervision.
