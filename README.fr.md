<div align="center">

![LemonHub](/web/default/public/logo.png)

# LemonHub

**Passerelle d'API IA multi-locataires, avec franchise d'agents/revendeurs intégrée, système étendu de commissions de parrainage et suite de tickets de support — conçue sur new-api.**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <strong>Français</strong> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://github.com/nsuanningmeng/LemonHub/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/nsuanningmeng/LemonHub?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/nsuanningmeng/LemonHub/releases/latest">
    <img src="https://img.shields.io/github/v/release/nsuanningmeng/LemonHub?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://github.com/nsuanningmeng/LemonHub/pkgs/container/lemonhub">
    <img src="https://img.shields.io/badge/ghcr.io-lemonhub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#aperçu">Aperçu</a> •
  <a href="#en-quoi-lemonhub-diffère-de-new-api">Différences avec new-api</a> •
  <a href="#démarrage-rapide">Démarrage rapide</a> •
  <a href="#franchise-dagents--revendeurs">Franchise d'agents</a> •
  <a href="#déploiement">Déploiement</a>
</p>

</div>

## Aperçu

LemonHub est un fork de développement secondaire de [new-api](https://github.com/QuantumNous/new-api) (lui-même construit sur [One API](https://github.com/songquanpeng/one-api)). Il conserve l'intégralité de la passerelle new-api — une API unifiée devant plus de 40 fournisseurs d'IA (OpenAI, Claude, Gemini, Azure, AWS Bedrock, …) avec facturation, limitation de débit et tableau de bord d'administration — et y ajoute :

- une **couche multi-locataires avec franchise d'agents** (un seul déploiement dessert de nombreux sous-sites, chacun exploité par un revendeur disposant de son propre domaine, de son marchand de paiement et d'un portefeuille d'approvisionnement prépayé) ;
- un **système étendu de commissions de parrainage** (bonus à la première recharge, commission en pourcentage continue, taux par utilisateur, classement administrateur et mode promoteur réglé en espèces) ;
- une **suite de tickets de support et de campagnes e-mail** ;
- une **configuration client en un clic** (Connect Hub) ;
- et une série de modifications de **facturation, paiement, relais, sécurité et migration**.

Le cœur de relais / transfert de canaux / facturation est maintenu compatible avec l'amont afin que les fonctionnalités et correctifs de new-api fusionnent proprement ; les ajouts de LemonHub vivent autour de lui.

> [!IMPORTANT]
> - Ce projet est destiné uniquement à des scénarios licites et autorisés de passerelle d'API IA, d'authentification au niveau organisationnel, de gestion multi-modèles, d'analyse d'utilisation, de comptabilité analytique des coûts et de déploiement privé/revendeur.
> - Vous devez obtenir légalement les clés d'API, comptes, services de modèles et autorisations d'interface en amont, et respecter les conditions d'utilisation en amont ainsi que les lois et réglementations applicables.
> - Lorsque vous fournissez des services d'IA générative au public, accomplissez toutes les obligations d'enregistrement, de licence, de sécurité des contenus, de vérification d'identité réelle, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont requises par votre juridiction.

## En quoi LemonHub diffère de new-api

Tout ce qui suit est ajouté ou modifié par le fork au-dessus de new-api en amont, regroupé par domaine. Il s'agit de la liste cumulative depuis la première version de LemonHub, pas d'une seule release.

### 1. Sous-sites multi-locataires

- Un seul déploiement et une seule base de données desservent de nombreux sous-sites indépendants ; chaque requête entrante est routée vers un sous-site selon son `Host` (middleware de domaine).
- Personnalisation par site : nom, logo, avis, pied de page et texte d'accroche de la page d'accueil, tous configurables par sous-site.
- Isolation des données par site : chaque table porte un `site_id` ; les noms d'utilisateur sont uniques **par site** (`(site_id, username)`) ; les mots de passe, la 2FA et toutes les liaisons OAuth sont isolés par site ; l'inscription, la connexion et OAuth sont limités au site courant. L'accès inter-sites est couvert par des tests d'autorisation.
- Une page de gestion du site principal pour créer et administrer les sous-sites.

### 2. Franchise d'agents / revendeurs

- Deux rôles : l'**administrateur du site principal** (propriétaire de la plateforme — détient le déploiement, les canaux en amont et vend du quota en gros) et l'**administrateur de sous-site** (agent — exploite un sous-site via une console d'agent dédiée, adossée aux endpoints `SiteAdminAuth`).
- **Portefeuille d'approvisionnement** : chaque agent préfinance la plateforme dans un portefeuille tenu en milli-CNY entiers. Chaque recharge d'utilisateur final débite le portefeuille de manière atomique au prix de gros dans la même transaction ; le portefeuille ne devient jamais négatif et chaque variation écrit une entrée de registre. Le site principal peut créditer, ajuster, réconcilier, annuler et rembourser.
- **Taux de gros (remise) par agent** : `DiscountRate` est un entier en base 10000 (`10000` = prix affiché, `7000` = 70 %). Coût de gros = `face × DiscountRate / 10000` ; l'agent conserve la marge.
- **Encaissement des paiements par site** : chaque sous-site configure son **propre** marchand EasyPay (易支付). Les recharges des utilisateurs finaux affluent vers le marchand de l'agent ; la plateforme règle le portefeuille dans la même transaction de base de données (créditer l'utilisateur **et** débiter le portefeuille de l'agent), de façon idempotente, avec **dégradation automatique** — lorsque le portefeuille d'un agent est épuisé ou non configuré, la recharge en ligne disparaît de façon transparente pour ce sous-site, sans affecter le quota déjà émis ni les autres passerelles.
- **Codes de rédemption par site** : les agents génèrent et annulent des codes limités à leur propre site, avec isolation inter-sites et réconciliation.
- **Majoration de modèle personnalisée par site** (voie A) : un agent peut majorer des appels de modèles spécifiques sur son site ; le règlement de gros de la plateforme n'est jamais sous-coté.
- Une page d'accueil de franchise pour revendeurs, une page Contact configurable et des entrées de navigation/pied de page.

### 3. Système de commissions de parrainage

L'amont fournit un bonus d'invitation ponctuel. LemonHub le remplace par un système de commissions complet :

- La récompense de parrainage est créditée **après la première recharge réussie du filleul** (et non à l'inscription), à laquelle s'ajoute une **commission en pourcentage continue** sur chaque recharge effectuée par le filleul ; les deux passent par un registre idempotent par événement avec des API de statistiques.
- Une page de détail du parrainage destinée à l'utilisateur et un classement personnel des contributions.
- **Remplacement du taux de commission par utilisateur** qui prime sur le taux global (non défini hérite du global ; `0` désactive la commission pour ce parrain).
- Un **classement de parrainage à l'échelle du site pour l'administrateur** (cartes de synthèse + classement avec recherche, tri et pagination, derrière `AdminAuth`).
- **Mode promoteur réglé en espèces** : un indicateur par utilisateur pour les promoteurs payés en espèces hors plateforme. Lorsqu'il est activé, le bonus d'invitation de la plateforme est supprimé et la commission de recharge est enregistrée dans un registre comme somme due en espèces (non créditée au solde de la plateforme) ; un registre de versements en espèces suit le solde restant dû avec un règlement protégé contre les surpaiements et sûr en cas de concurrence. Les opérations relatives à la politique monétaire (marquer un promoteur, définir le taux de commission, enregistrer/consulter un règlement en espèces) sont réservées au compte root/propriétaire.

### 4. Tickets de support et campagnes e-mail

- Un espace de tickets pour les utilisateurs et une vue de gestion des tickets pour l'administrateur, avec priorité.
- Un outil de campagne e-mail / promotion de masse : migration de schéma, téléversement de pièces jointes, envoi groupé, limitation de débit, nettoyage et audit.
- Des tests de sécurité côté backend couvrant la gestion des téléversements, le XSS Markdown, l'injection d'en-têtes, l'autorisation et la priorité.

### 5. Connect Hub (configuration client en un clic)

- Configuration client en un clic pour Claude Code, Codex, Gemini CLI, Chatbox, Cherry Studio, VS Code et plus encore. La configuration générée cible **le domaine que l'utilisateur visite réellement**, ce qui la rend fonctionnelle sur plusieurs domaines.

### 6. Facturation et routage des tokens

- **Basculement prioritaire multi-groupes de tokens** : un token peut porter une liste ordonnée de groupes et basculer d'un groupe à l'autre ; **la facturation par paliers/expressions est calculée sur le groupe réellement utilisé**.
- L'éditeur de ratios du frontend par défaut met en évidence les modèles qui n'ont actuellement **aucun prix défini**, afin qu'ils ne soient pas oubliés silencieusement.

### 7. Fiabilité du paiement et du relais

- Les rappels EasyPay sont exemptés du traitement gzip et du limiteur de débit global et bénéficient d'un limiteur de secours dédié et permissif ; `notify_url` est ancré à un domaine stable.
- Les URL de rappel/retour de paiement et le `<title>` de la page au premier rendu suivent le domaine visité (de confiance), avec protection contre l'usurpation de Host ; un bug de scintillement du titre est corrigé.
- Des tests de concurrence/idempotence pour le règlement EasyPay.
- Les nouvelles tentatives de relais respectent les codes de statut 504/524 configurés ; les réponses `BadResponseBody` hors streaming deviennent réessayables ; lorsque tous les canaux sont épuisés, la véritable erreur en amont est renvoyée au lieu d'une erreur générique.
- Correctif d'abonnement : un abonnement expiré ramène correctement l'utilisateur à son groupe d'origine (`prev_user_group` est préservé lors du renouvellement).

### 8. Paramètres de performance des modèles

- Seuil de taux de réussite, liste blanche de codes d'erreur et gestion du cas « aucune donnée = 100 % ».

### 9. Renforcement de la sécurité et des migrations

- Renforcement SSRF pour les canaux personnalisés avancés.
- Sûreté des migrations de base de données sur les trois moteurs (SQLite / MySQL / PostgreSQL) : contrôle préalable fail-closed de la précision de `price_amount`, contrôle préalable de la précision des prix d'abonnement, contrôle préalable fail-stop en chemin rapide, et un remplissage des `site_id` NULL afin que les requêtes du site principal ne masquent jamais d'anciennes lignes.

### 10. Empaquetage et documentation

- Les images Docker sont publiées sur GitHub Container Registry (`ghcr.io/nsuanningmeng/lemonhub`), multi-arch (amd64 + arm64) ; `docker-compose` pointe par défaut vers l'image du fork.
- Un README multilingue et des guides pas à pas pour les sous-sites / agents.

> Nouveau dans le modèle d'agent ? Lisez le guide pas à pas : **[Guide des sous-sites / franchise d'agents (中文)](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

## Démarrage rapide

### Docker Compose (recommandé)

```bash
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# Review/edit the configuration (DB password, ServerAddress, etc.)
nano docker-compose.yml

# Start (the compose file already points at the LemonHub image)
docker compose up -d
```

<details>
<summary>Commande Docker simple</summary>

```bash
docker pull ghcr.io/nsuanningmeng/lemonhub:latest

# SQLite (default — mount /data to persist)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest

# MySQL / PostgreSQL (set SQL_DSN)
docker run --name lemonhub -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/lemonhub" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  ghcr.io/nsuanningmeng/lemonhub:latest
```

</details>

Après le déploiement, ouvrez `http://localhost:3000`. Le premier compte enregistré devient l'**administrateur root / de la plateforme (site principal)**.

> [!WARNING]
> Lorsque vous exploitez LemonHub comme service d'IA public ou destiné à la revente, accomplissez d'abord toutes les obligations requises d'enregistrement, de licence, de sécurité des contenus, de vérification d'identité réelle, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont.

## Franchise d'agents / revendeurs

LemonHub s'articule autour de deux rôles :

- **Administrateur du site principal (propriétaire de la plateforme)** — détient le déploiement et les canaux d'IA (clés en amont) et vend du quota en gros. Crée les sous-sites, alimente les portefeuilles des agents et fixe le taux de remise de chaque agent.
- **Administrateur de sous-site (agent / revendeur)** — exploite un sous-site sur son propre domaine, configure son propre marchand de paiement et son apparence, et sert ses propres utilisateurs finaux.

Le flux d'argent en un coup d'œil (exemple, remise `7000` = 70 %) :

```
End-user pays ¥100  ──►  Agent's OWN EasyPay merchant   (agent keeps ¥100)
        │
        ▼ (callback, one DB transaction, idempotent)
   User credited ¥100 of quota   +   Agent procurement wallet debited ¥70
                                          └─ ¥30 is the agent's margin
```

Le tutoriel complet — ce que chaque rôle doit préparer, et chaque étape — se trouve ici :
**[Guide des sous-sites / franchise d'agents (中文)](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

## Hérité de new-api

LemonHub conserve les capacités de passerelle de new-api, notamment :

- **Formats** : OpenAI Chat/Responses/Realtime, Claude Messages, Google Gemini, Rerank (Cohere/Jina), Image/Audio/Embedding, Midjourney-Proxy, Suno, Dify.
- **Conversion de format** : OpenAI ⇄ Claude Messages, OpenAI → Gemini, thinking-to-content, suffixes de reasoning-effort.
- **Routage intelligent** : canaux aléatoires pondérés, nouvelle tentative automatique en cas d'échec (codes de statut de reprise configurables), limitation de débit au niveau utilisateur.
- **Facturation** : comptabilité par requête / basée sur l'usage / sur les accès au cache, tarification par paliers et par expressions, recharge via EasyPay et Stripe.
- **Authentification** : JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, LinuxDO, Telegram, WeChat).
- **Interface** : tableau de bord moderne, multilingue (zh/en/fr/ja/vi…), tableau de bord de données, métriques de performance des modèles.

Pour les détails sur la passerelle/l'API, reportez-vous à la [documentation new-api](https://docs.newapi.pro) en amont.

## Déploiement

> [!TIP]
> Dernière image : `ghcr.io/nsuanningmeng/lemonhub:latest` (multi-arch : amd64 + arm64).

### Prérequis

| Composant | Exigence |
|---|---|
| BD locale | SQLite (Docker doit monter `/data`) |
| BD distante | MySQL ≥ 5.7.8 ou PostgreSQL ≥ 9.6 |
| Cache (recommandé) | Redis |
| Moteur | Docker / Docker Compose |

### Variables d'environnement courantes

| Variable | Description | Valeur par défaut |
|---|---|---|
| `SESSION_SECRET` | Secret de session (requis pour le multi-nœuds) | - |
| `CRYPTO_SECRET` | Secret de chiffrement (requis pour un Redis partagé) | - |
| `SQL_DSN` | Chaîne de connexion à la base de données (MySQL/PostgreSQL) | - |
| `REDIS_CONN_STRING` | Chaîne de connexion Redis | - |
| `TRUSTED_REDIRECT_DOMAINS` | Domaines de confiance séparés par des virgules pour la redirection de paiement / les rappels multi-domaines | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | Garde-fou généreux par IP pour les webhooks de notification de paiement (requêtes / fenêtre) | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | Fenêtre pour le paramètre ci-dessus (secondes) | `60` |
| `STREAMING_TIMEOUT` | Délai d'expiration sans réponse en streaming (secondes) | `300` |
| `MAX_REQUEST_BODY_MB` | Corps de requête maximal (Mo, après décompression) | `32` |

Les variables de limitation de débit et la plupart des variables de réglage retombent sur des valeurs par défaut raisonnables du code ; elles ne sont donc pas requises dans `.env`/compose. Voir `.env.example` pour les réglages optionnels documentés.

### Multi-nœuds

> [!WARNING]
> - Définissez `SESSION_SECRET`, sinon l'état de connexion est incohérent entre les nœuds.
> - Avec un Redis partagé, définissez `CRYPTO_SECRET`, sinon les données chiffrées ne peuvent pas être déchiffrées.

### Reprise et cache

- Reprise : `Paramètres → Paramètres d'exploitation → Fiabilité du routage` (nombre de tentatives en cas d'échec + plages de codes de statut pour la reprise automatique).
- Cache : `REDIS_CONN_STRING` (recommandé) ou `MEMORY_CACHE_ENABLED`.

## Conçu sur new-api

LemonHub est un fork sous licence AGPL. Crédit aux projets en amont :

| Projet | Rôle |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | Amont direct — la passerelle que LemonHub étend |
| [One API](https://github.com/songquanpeng/one-api) | Base originale du projet (MIT) |

LemonHub se synchronise régulièrement avec new-api en amont.

## Licence

Ce projet est sous licence [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE), héritant de la licence en amont.

Conformément aux conditions additionnelles de la section 7 de l'AGPLv3, les versions modifiées doivent conserver la mention d'attribution de l'auteur `Frontend design and development by New API contributors.` à l'emplacement légal / à propos / pied de page approprié, et doivent conserver un lien visible vers le projet original : <https://github.com/QuantumNous/new-api>.

Il s'agit d'un projet open source développé à partir de [One API](https://github.com/songquanpeng/one-api) (licence MIT).

## Aide et contributions

- Problèmes et demandes de fonctionnalités : [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- Guide des sous-sites : [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- Référence passerelle/API : [documentation new-api](https://docs.newapi.pro)

Toutes les contributions sont les bienvenues — rapports de bugs, fonctionnalités, documentation et code.

<div align="center">
<sub>LemonHub — une couche de franchise d'agents par-dessus <a href="https://github.com/QuantumNous/new-api">new-api</a>.</sub>
</div>
