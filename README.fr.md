<div align="center">

![LemonHub](/web/default/public/logo.png)

# 🍋 LemonHub

**Passerelle d'API IA multi-locataires en marque blanche, avec système intégré de franchise d'agents / revendeurs**

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
  <a href="#-démarrage-rapide">Démarrage rapide</a> •
  <a href="#-ce-qui-distingue-lemonhub">Pourquoi LemonHub</a> •
  <a href="#-marque-blanche--franchise-dagents">Marque blanche</a> •
  <a href="#-déploiement">Déploiement</a> •
  <a href="#-conçu-sur-new-api">Conçu sur new-api</a>
</p>

</div>

## 📝 Description du projet

**LemonHub** est un fork de développement secondaire de [new-api](https://github.com/QuantumNous/new-api) (lui-même construit sur [One API](https://github.com/songquanpeng/one-api)). Il conserve toute la puissance de new-api — une passerelle unifiée devant plus de 40 fournisseurs d'IA (OpenAI, Claude, Gemini, Azure, AWS Bedrock, …) avec facturation, limitation de débit et tableau de bord d'administration — et y ajoute une **couche multi-locataires, en marque blanche et de franchise d'agents** :

> Un seul déploiement + une seule base de données peuvent desservir **de nombreux sous-sites à marque indépendante**, chacun détenu par un **agent (revendeur)** qui exploite son propre domaine, encaisse les paiements vers son **propre marchand de paiement** et achète du quota en gros à la plateforme via un **portefeuille d'approvisionnement prépayé**.

> [!IMPORTANT]
> - Ce projet est destiné uniquement à des scénarios licites et autorisés de passerelle d'API IA, d'authentification au niveau organisationnel, de gestion multi-modèles, d'analyse d'utilisation, de comptabilité analytique des coûts, et de déploiement privé/revendeur.
> - Vous devez obtenir légalement les clés d'API, comptes, services de modèles et autorisations d'interface en amont, et respecter les conditions d'utilisation en amont ainsi que les lois et réglementations applicables.
> - Lorsque vous fournissez des services d'IA générative au public, accomplissez toutes les obligations requises par votre juridiction en matière d'enregistrement, de licence, de sécurité des contenus, de vérification d'identité réelle, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont.

---

## ✨ Ce qui distingue LemonHub

En plus de tout ce qu'offre new-api, LemonHub ajoute :

| Capacité | Description |
|---|---|
| 🏢 **Sous-sites en marque blanche** | Un seul déploiement dessert de nombreux locataires à marque propre. Chaque sous-site possède son ou ses propres domaines, nom, logo, avis, pied de page et texte d'accroche de la page d'accueil. Les requêtes sont routées vers un sous-site selon leur `Host`. |
| 🤝 **Franchise d'agents / revendeurs** | Le propriétaire de la plateforme (site principal) intègre des **agents** ; chaque agent détient et auto-administre un sous-site via une **console d'agent** dédiée. |
| 💰 **Portefeuille d'approvisionnement (registre 整数厘)** | Chaque agent préfinance la plateforme dans un portefeuille tenu en *milli-CNY* entiers (厘). Chaque recharge utilisateur débite le portefeuille de manière atomique au prix de gros — il ne devient jamais négatif, et chaque variation écrit une entrée de registre. |
| 🏷️ **Taux de remise (gros) par agent** | `DiscountRate` est un entier en base 10000 (`10000` = prix affiché, `7000` = 70 %). Coût de gros = `face × DiscountRate / 10000` ; l'agent conserve la marge. |
| 💳 **Encaissement des paiements par site** | Chaque sous-site configure son **propre** marchand EasyPay (易支付). Les recharges des utilisateurs affluent vers le compte marchand propre à l'agent ; la plateforme règle le portefeuille dans la même transaction de base de données (créditer l'utilisateur **+** débiter le portefeuille de l'agent), de façon idempotente. |
| 🔻 **Dégradation automatique** | Lorsque le portefeuille d'un agent est épuisé (ou non configuré), la recharge en ligne disparaît de façon transparente pour ce sous-site — le quota déjà émis et les autres passerelles ne sont pas affectés. |
| 🔒 **Isolation des données par site** | Chaque table porte un `site_id` ; les noms d'utilisateur sont uniques **par site** (`(site_id, username)`) ; les mots de passe, la 2FA et toutes les liaisons OAuth sont isolés par site. |
| 🎟️ **Codes de rédemption par site** | Les agents génèrent/annulent des codes de rédemption limités à leur propre site, avec isolation inter-sites et réconciliation. |
| 🔌 **Connect Hub** | Configuration client en un clic (Claude Code / Codex / Gemini CLI / Chatbox / Cherry Studio / VS Code …) qui cible **le domaine que l'utilisateur visite réellement** — compatible multi-domaines. |
| 🌐 **Rappels et titres de paiement multi-domaines** | Les URL de notification/retour de paiement et le `<title>` du premier rendu suivent le domaine visité (de confiance), avec protection contre l'usurpation de Host. |

> 📖 **Nouveau dans le modèle d'agent ? Lisez le guide pas à pas :** **[Guide des sous-sites / franchise d'agents (中文)](./docs/subsite-guide.md)** · [English](./docs/subsite-guide.en.md)

---

## 🚀 Démarrage rapide

### Avec Docker Compose (recommandé)

```bash
# Clone the project
git clone https://github.com/nsuanningmeng/LemonHub.git
cd LemonHub

# Review/edit the configuration (DB password, ServerAddress, etc.)
nano docker-compose.yml

# Start (the compose file already points at the LemonHub image)
docker compose up -d
```

<details>
<summary><strong>Avec une commande Docker simple</strong></summary>

```bash
# Pull the latest image
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

🎉 Après le déploiement, ouvrez `http://localhost:3000`. Le premier compte enregistré devient l'**administrateur root / de la plateforme (site principal)**.

> [!WARNING]
> Lorsque vous exploitez LemonHub comme service d'IA public ou destiné à la revente, accomplissez d'abord toutes les obligations requises en matière d'enregistrement, de licence, de sécurité des contenus, de vérification d'identité réelle, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont.

---

## 🤝 Marque blanche / Franchise d'agents

LemonHub s'articule autour de deux rôles :

- **Administrateur du site principal (propriétaire de la plateforme / 主站站长)** — détient le déploiement, les canaux d'IA (clés en amont) et vend du quota en gros. Crée les sous-sites, alimente les portefeuilles des agents et fixe le taux de remise de chaque agent.
- **Administrateur de sous-site (agent / revendeur / 子站站长)** — exploite un sous-site à marque propre sur son propre domaine, configure son propre marchand de paiement et son image de marque, et sert ses propres utilisateurs finaux.

**Le flux d'argent en un coup d'œil** (exemple, remise `7000` = 70 %) :

```
End-user pays ¥100  ──►  Agent's OWN EasyPay merchant   (agent keeps ¥100)
        │
        ▼ (callback, one DB transaction, idempotent)
   User credited ¥100 of quota   +   Agent procurement wallet debited ¥70
                                          └─ ¥30 is the agent's margin
```

Le tutoriel complet, étape par étape — ce que chaque rôle doit préparer, et chaque clic — se trouve ici :

➡️ **[📘 Guide des sous-sites / franchise d'agents (中文，保姆级)](./docs/subsite-guide.md)** · **[English](./docs/subsite-guide.en.md)**

---

## 🤖 Prise en charge des modèles et fonctionnalités (héritée de new-api)

LemonHub hérite des capacités de passerelle de new-api, notamment :

- **Formats** : OpenAI Chat/Responses/Realtime, Claude Messages, Google Gemini, Rerank (Cohere/Jina), Image/Audio/Embedding, Midjourney-Proxy, Suno, Dify.
- **Conversion de format** : OpenAI ⇄ Claude Messages, OpenAI → Gemini, thinking-to-content, suffixes de reasoning-effort.
- **Routage intelligent** : canaux aléatoires pondérés, nouvelle tentative automatique en cas d'échec (codes de statut de reprise configurables), basculement prioritaire multi-groupes de tokens, limitation de débit au niveau utilisateur.
- **Facturation** : comptabilité par requête / basée sur l'usage / sur les accès au cache, tarification par paliers/expressions, recharge via EasyPay et Stripe.
- **Authentification** : JWT, WebAuthn/Passkeys, OAuth (GitHub, Discord, OIDC, LinuxDO, Telegram, WeChat).
- **Interface** : tableau de bord moderne, multilingue (zh/en/fr/ja/vi…), tableau de bord de données, métriques de performance des modèles.

> 📚 Pour les détails sur la passerelle/l'API, reportez-vous à la [documentation new-api](https://docs.newapi.pro) en amont.

---

## 🚢 Déploiement

> [!TIP]
> **Dernière image :** `ghcr.io/nsuanningmeng/lemonhub:latest` (multi-arch : amd64 + arm64).

### Prérequis

| Composant | Exigence |
|---|---|
| **BD locale** | SQLite (Docker doit monter `/data`) |
| **BD distante** | MySQL ≥ 5.7.8 ou PostgreSQL ≥ 9.6 |
| **Cache (recommandé)** | Redis |
| **Moteur** | Docker / Docker Compose |

### Variables d'environnement courantes

| Variable | Description | Valeur par défaut |
|---|---|---|
| `SESSION_SECRET` | Secret de session (**requis** pour le multi-nœuds) | - |
| `CRYPTO_SECRET` | Secret de chiffrement (**requis** pour un Redis partagé) | - |
| `SQL_DSN` | Chaîne de connexion à la base de données (MySQL/PostgreSQL) | - |
| `REDIS_CONN_STRING` | Chaîne de connexion Redis | - |
| `TRUSTED_REDIRECT_DOMAINS` | Domaines de confiance séparés par des virgules pour la redirection de paiement / les rappels multi-domaines | - |
| `PAYMENT_WEBHOOK_RATE_LIMIT` | Garde-fou généreux par IP pour les webhooks de notification de paiement (requêtes / fenêtre) | `1800` |
| `PAYMENT_WEBHOOK_RATE_LIMIT_DURATION` | Fenêtre pour le paramètre ci-dessus (secondes) | `60` |
| `STREAMING_TIMEOUT` | Délai d'expiration sans réponse en streaming (secondes) | `300` |
| `MAX_REQUEST_BODY_MB` | Corps de requête maximal (Mo, après décompression) | `32` |

> Les variables de limitation de débit et la plupart des variables de réglage retombent sur des valeurs par défaut raisonnables du code ; elles ne sont donc **pas** requises dans `.env`/compose. Voir `.env.example` pour les réglages optionnels documentés.

### Notes pour le multi-nœuds

> [!WARNING]
> - **Définissez** `SESSION_SECRET` — sinon l'état de connexion est incohérent entre les nœuds.
> - **Avec un Redis partagé, définissez** `CRYPTO_SECRET` — sinon les données chiffrées ne peuvent pas être déchiffrées.

### Reprise et cache

- **Reprise** : `Paramètres → Paramètres d'exploitation → Fiabilité du routage` (nombre de tentatives en cas d'échec + plages de codes de statut pour la reprise automatique).
- **Cache** : `REDIS_CONN_STRING` (recommandé) ou `MEMORY_CACHE_ENABLED`.

---

## 🧱 Conçu sur new-api

LemonHub est un fork sous licence AGPL. Un immense crédit aux projets en amont :

| Projet | Rôle |
|---|---|
| [new-api](https://github.com/QuantumNous/new-api) | Amont direct — la passerelle que LemonHub étend |
| [One API](https://github.com/songquanpeng/one-api) | Base originale du projet (MIT) |

LemonHub se synchronise régulièrement avec new-api en amont. Le cœur de relais / facturation / transfert de canaux est maintenu compatible octet pour octet afin que les fonctionnalités et correctifs en amont puissent être fusionnés proprement ; les ajouts de LemonHub vivent autour de lui (isolation des sous-sites, portefeuilles, paiement par site, image de marque).

---

## 📜 Licence

Ce projet est sous licence [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE), héritant de la licence en amont.

Conformément aux conditions additionnelles de la section 7 de l'AGPLv3, les versions modifiées doivent conserver la mention d'attribution de l'auteur `Frontend design and development by New API contributors.` à l'emplacement légal/à propos/pied de page approprié, et doivent conserver un lien visible vers le projet original : <https://github.com/QuantumNous/new-api>.

Il s'agit d'un projet open source développé à partir de [One API](https://github.com/songquanpeng/one-api) (licence MIT).

---

## 💬 Aide et contributions

- 🐛 Problèmes et demandes de fonctionnalités : [LemonHub Issues](https://github.com/nsuanningmeng/LemonHub/issues)
- 📘 Guide des sous-sites : [中文](./docs/subsite-guide.md) · [English](./docs/subsite-guide.en.md)
- 📚 Référence passerelle/API : [documentation new-api](https://docs.newapi.pro)

Toutes les contributions sont les bienvenues — rapports de bugs, fonctionnalités, documentation et code.

<div align="center">

Si LemonHub vous est utile, pensez à lui donner une ⭐️

<sub>🍋 LemonHub — une couche de marque blanche / franchise d'agents par-dessus <a href="https://github.com/QuantumNous/new-api">new-api</a>.</sub>

</div>
