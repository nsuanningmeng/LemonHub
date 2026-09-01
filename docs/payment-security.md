# Payment callback security

## Threat model

Payment callback endpoints are public. Attackers may replay callbacks, alter signed
fields, obtain a merchant signing key, or send a correctly signed success payload for
an order that was never created or paid at the gateway.

## Settlement boundary

An Epay callback is a trigger, not proof of payment. Before a pending order can settle,
the server must:

1. Verify the callback signature with credentials selected from the local order owner.
2. Bind `pid`, `out_trade_no`, `trade_no`, payment type, and the exact stored amount.
3. Query the configured gateway by the local `out_trade_no`.
4. Require a paid response whose merchant, order identifiers, payment type, and amount
   match the callback and local order.
5. Enter the existing short, idempotent database settlement transaction only after the
   network query succeeds.

Missing fields, an unreachable gateway, an unpaid/missing order, or any mismatch fails
closed: the local order remains pending and no quota or subscription is issued.

Recent pending top-up and subscription orders are retried by the existing scheduled
Epay reconciliation task. This recovers payments when a callback-time query fails
transiently, while keeping settlement subject to the same strict checks. Attempt times
are persisted and batches rotate fairly, so abandoned orders cannot permanently hide
an older paid order behind the reconciliation limit. A run with any per-order failure
is reported as failed while retaining its detailed result for operators.

Signed callbacks for the same pending order share a database-backed query cooldown
across application instances. That public-input cooldown is separate from the scheduled
reconciliation timestamp, so forged callbacks cannot push a paid order out of fair
automatic recovery. Each process also caps total concurrent gateway queries. Sub-sites
have both a shared cap and a smaller per-site cap, leaving reserved capacity for the
global merchant and subscriptions. These limits cover DNS/SSRF validation as well as
the HTTP request, preventing leaked tenant credentials from creating an unbounded
DNS/TLS/outbound-request fan-out or starving main-site settlement.

## Subscription checkout contract

- Every new external subscription order stores a versioned, private snapshot of the
  price and fulfillment terms before a gateway can accept payment. Settlement grants
  the duration, quota, reset policy, group changes, and wallet-overflow policy from
  that snapshot, not from a plan that an administrator may have edited later.
- Order creation locks and reloads the authoritative plan row before checking enabled
  state, price, terms, and purchase capacity. Process-local/Redis plan cache entries are
  display hints only and cannot authorize a checkout. The gateway request uses the same
  locked row that produced the stored snapshot.
- Legacy pending orders without a snapshot are accepted only when the current plan is
  provably older than the checkout and its exact price is unchanged. Ambiguous legacy
  orders fail closed and stay pending for manual investigation.
- A limited Epay plan reserves a purchase slot in the same database transaction that
  creates its pending order. Other gateways do not reserve indefinitely because their
  checkout URLs/sessions are not persisted; an abandoned row must not permanently block
  future checkout, balance purchase, or administrator assignment.
- Once an external gateway has authoritatively confirmed payment, the paid order is
  fulfilled idempotently when it either owns a durable reservation or still clears the
  purchase limit. Failed or expired checkout creation releases its pending row.
- Epay can regenerate the signed form for an existing pending reservation. It prefers
  the newly requested payment method, but safely reuses the order's original method when
  necessary rather than mutating a form that may already have reached the gateway.

## Multi-domain and merchant selection

- Main-site server-to-server notifications use the stable configured callback address.
- Browser return URLs use the trusted domain on which the purchase started.
- Main-site and subscription gateway queries use the global Epay configuration.
- Sub-site top-up callbacks and queries use the merchant configuration belonging to the
  local order's `SiteId`.
- Sub-site Epay methods shown to users and accepted by the payment endpoint come only
  from that sub-site merchant's `pay_config.pay_methods`, not the global merchant list.
- Callback `Host`, `Origin`, and submitted gateway addresses never select credentials.

## Known operational constraints

- The classic Epay query API sends the merchant key as a query parameter. `PayAddress`
  is therefore required to be an absolute HTTPS URL. Queries use strict certificate
  validation, a direct SSRF-protected connection, no environment proxy, and no redirects.
  The same DNS/IP/domain/port policy is checked before a payment form is returned, so a
  gateway the server cannot query is rejected before the customer can pay.
- Epay-compatible gateways that omit required query identity/amount fields cannot be
  used for automatic settlement; such orders remain pending for manual investigation.
- Rotating the global merchant configuration can make old pending orders unverifiable,
  because subscription orders do not currently snapshot a non-secret merchant version.
- Classic Epay signed forms have no standard server-enforceable expiry/cancel field. A
  pending limited-plan Epay quote is therefore reusable until it settles or is manually
  handled; silently expiring it could leave a customer able to pay an upstream form that
  the application would no longer honor.
- Stripe/Creem/Waffo checkout sessions do not currently persist a reusable reservation.
  Concurrent paid sessions for a limited plan are serialized at settlement; a losing
  paid session remains pending and requires refund/manual handling rather than exceeding
  the configured purchase limit.
- If a malicious gateway lies consistently in both callbacks and authenticated order
  queries, the application cannot independently prove settlement; use a trusted gateway
  or independent clearing evidence.
