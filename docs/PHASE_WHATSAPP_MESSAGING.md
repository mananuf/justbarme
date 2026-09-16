# Proposed Phase — WhatsApp messaging via Zavu

**Status:** Draft. Not approved, not started, not part of the original frozen `IMPLEMENTATION_PLAN.md` phase sequence (Phase 0–10) — like platform admin, signup, and Google OAuth before it, this is a deviation designed in-session because the original plan never anticipated it. Nothing in this document should be read as scheduled or committed.
**Depends on:** `internal/signup` (OTP mechanics), `internal/email` (Provider/retry/template pattern this mirrors), and — for the invitations sub-phase — the still-unbuilt invitations endpoint (`docs/IMPLEMENTATION_PLAN.md` Phase 2 explicitly deferred `POST /invitations`).
**Vendor:** [Zavu](https://docs.zavu.dev) — a unified messaging API (WhatsApp, SMS, voice, email, Telegram, Messenger) sitting in front of Meta's WhatsApp Business Platform.

---

## 1. Why WhatsApp, and why now

Nigeria has the highest WhatsApp adoption of any country in the world — roughly 95–98% of its ~107M internet users, 51M+ active users, and the continent's leading WhatsApp Business download count, concentrated specifically in informal/micro-business commerce. That last point matters more than the raw adoption number: this is not a generic "everyone uses WhatsApp" argument, it's evidence that justbarme's exact target audience (small Nigerian bar owners and staff) already runs business communication through WhatsApp specifically, in a way email demonstrably is not their default channel for.

Concretely, this phase exists to serve three needs already identified as gaps:

1. **OTP delivery that doesn't depend on a mailbox someone checks rarely.** Email OTP (`internal/signup`) works today, but for this audience it's plausibly the weaker channel, not the stronger one.
2. **A resend path when email genuinely fails or is inaccessible.** Today "Resend code" just re-sends the same email again. If the mailbox itself is the problem (wrong address typo'd, forgotten password, no access on this device), that button does nothing useful.
3. **Business/operational updates** (low stock, an expiring offline device lease, a daily summary) that don't exist as a notification concept at all yet.

Sources: [Brand Communicator — Nigeria WhatsApp users](https://brandcom.ng/2025/05/07/nigeria-boasts-51-million-active-whatsapp-users-ranks-10th-globally/), [Statista — WhatsApp usage by country](https://www.statista.com/statistics/291540/mobile-internet-user-whatsapp/), [Yazi — WhatsApp penetration Africa](https://www.askyazi.com/articles/whatsapp-penetration-across-africa-statistics-by-country), [Wapikit — WhatsApp Business statistics 2025](https://www.wapikit.com/blog/global-whatsapp-business-statistics-2025).

## 2. What Zavu is and why it, not raw Meta Cloud API

Zavu wraps WhatsApp (and SMS, voice, email, Telegram, Messenger) behind one REST API/TypeScript SDK, with:

- **Smart routing**: prefers WhatsApp over SMS when the recipient's 24-hour conversation window is open (~$0.01/msg vs ~$0.05/msg for SMS), and automatically fails over between *phone-number-based* channels (WhatsApp → SMS → Voice) on delivery failure — max 2 attempts, 5s delay, only when the sender supports both channels. **It does not fail over between a phone number and an email address** — those are different recipient identifiers, so an "email failed, try WhatsApp" flow is something justbarme has to orchestrate itself, not something Zavu does automatically.
- **Template management**: WhatsApp requires Meta pre-approval for any message sent outside a 24-hour reply window. Zavu manages this as its own `Template` concept (categories `AUTHENTICATION`/`UTILITY`/`MARKETING`, numbered or named variables, submission/approval workflow), converting to Meta's positional format on submission.
- **Webhooks**: one contract for delivery status (`message.queued/sent/delivered/failed`) and inbound events, HMAC-SHA256 signed (`X-Zavu-Signature`, format `t=<ts>,v2=<hash>` over `"{t}.{body}"`).

This is the same "keep the provider behind an interface" principle `internal/email` already follows for SMTP — Zavu is one more `Provider`-shaped dependency, not a special case.

## 3. Prerequisites (account/setup, not code)

1. **Meta Business Manager + WhatsApp Business Account (WABA)**, created through Zavu's dashboard (it walks through Meta's business-signup flow).
2. **A phone number**: buy one through Zavu, or bring one that can receive SMS/voice and is **not already registered on WhatsApp or WhatsApp Business**.
3. **A Zavu "sender"** — the routing identity bundling phone number + WABA + webhook config, same slot other channels (SMS, SES-backed email domain) would attach to.
4. **An `AUTHENTICATION`-category template for OTP.** Meta auto-generates the body (`"{{1}} is your verification code."`); pick Copy-Code or One-Tap (Android-only); these get approved fastest (1–4h vs 24–72h for other categories).
5. **API keys**: `zv_live_...` for production, `zv_test_...` for development — but note the test key only reaches a WhatsApp *sandbox* restricted to team members' numbers, real messages with restricted recipients, not a simulator. This is the same "can't be live-verified without real third-party credentials" situation Google OAuth is already in — see §8.
6. **A webhook endpoint** for delivery events, signature-verified.

## 4. Sub-phases

### N.1 — Foundation

New `internal/whatsapp` package, sibling to `internal/email`, same `Provider`-interface shape: a real `ZavuProvider` plus a `ConsoleProvider`-equivalent dev fallback (justbarme's own dev-fallback pattern, not Zavu's sandbox, since the sandbox's real-message-restricted-recipients behavior is worse for local dev than simply logging). Config: `JBM_ZAVU_API_KEY`, `JBM_ZAVU_SENDER_ID`, `JBM_ZAVU_OTP_TEMPLATE_ID`, `JBM_ZAVU_WEBHOOK_SECRET`. New `POST /api/v1/webhooks/zavu` (signature-verified, unauthenticated) for delivery status.

### N.2 — WhatsApp as an OTP channel

`signup_verifications` gains a `phone` column (parallel to `email`) and a `channel` column so `Verify` knows which channel's code is currently live. The frontend signup form adds an optional phone field and a channel choice ("Email" / "WhatsApp") before the code is sent — given §1's adoption numbers, the UI should give WhatsApp equal or greater visual weight, not bury it as a secondary option under an email-shaped default.

### N.3 — "Can't access email" resend-via-WhatsApp

The existing "Resend code" affordance in `SignUp.tsx` gains a second button — "Try WhatsApp instead" — that re-issues the same pending signup attempt through `internal/whatsapp` rather than `internal/email`, reusing `signup_verifications`' existing `attempts`/expiry semantics. Only offered if a phone number was collected.

### N.4 — Bar operational updates

`UTILITY`-category templates for things like a low-stock alert, a daily sales summary, or "your offline device access expires soon." Needs a notification-preferences model (which updates, which channel) that doesn't exist yet, and depends on the sale/inventory modules existing first (they don't — Phase 5–8 of `IMPLEMENTATION_PLAN.md`). Treat as its own sub-phase, not bundled with OTP work.

**Compliance note carried over from N.2/N.3**: keep operational updates (low-stock, lease-expiry) on a *different lawful basis and opt-out mechanism* than anything that reads as promotional (feature announcements, tips). See §5 — this split needs to exist from the first message sent, not retrofitted later.

### N.5 — Staff invitations via WhatsApp

Once the deferred invitations endpoint (`POST /invitations`) exists, an owner inviting a staff member by phone number gets the invite link delivered as a WhatsApp template message instead of assuming a business email the invitee may not have. No new architecture beyond N.1–N.3 — but see §5 for the indirect-collection privacy-notice requirement this specifically triggers.

## 5. NDPA 2023 compliance assessment

Run via this session's `ndpc` skill on 2026-09-16. Full detail (including section-by-section reasoning) is preserved in this session's project memory (`ndpa_whatsapp_messaging.md`); this is the summary that should travel with the phase itself. Labels: **[ACT]** = stated in the Nigeria Data Protection Act 2023, **[DELEGATED]** = the Act hands this to Commission regulation not yet examined, **[PRACTICE]** = professional judgment, not law.

```
Role:          Controller (justbarme). Zavu = processor. Meta/WhatsApp Business Platform = Zavu's sub-processor.
DCPMI:         [PRACTICE] Likely No at current single-pilot-bar scale — the quantitative headcount
               threshold is [DELEGATED] (NDPA s.65, unset by the Act itself), so re-check as user
               volume grows. Do not assume "No" is permanent.
Verdict:       GAPS, not a blocker — three things need doing before phone numbers are first collected.
```

- **[ACT] Applies regardless of hosting location.** s.2(2)(c)'s extraterritorial limb catches processing of a Nigerian data subject's personal data even where the controller/processor has no Nigerian presence — this is live from the first phone number collected, not a "check before scaling" item.
- **[ACT] Lawful basis, kept separate per purpose** (bundling purposes is flagged in the skill's own playbook as the commonest way to get this wrong):
  - *OTP via WhatsApp* → contract-necessity, s.25(1)(b)(i) — same honest basis email OTP already uses. Not consent: you can't meaningfully let someone "withdraw consent" to identity verification and keep the account.
  - *Bar operational updates* → contract-necessity **only for genuinely operational content**. Anything promotional-feeling needs a separate, opt-in, consent-based track (s.25(1)(a)) — direct marketing carries an **absolute** opt-out right under s.36(3)-(4), no balancing test, and treating operational and promotional messages as one basis/one toggle is the bundling error above in concrete form.
  - *Staff invitations* → the invitee's phone number is collected **indirectly** (from the inviting owner), which triggers s.27(2): the invite flow's landing page needs its own privacy notice, since neither s.27(2) exception (already has it / disproportionate effort) plausibly applies to a WhatsApp invite link. Whose basis covers contacting the invitee is a genuinely close call — leans toward the *inviting owner's own* contract-necessity rather than justbarme's legitimate interest (per the s.25(2)(b) incompatibility test) — **flagged for actual counsel review, not resolved here.**
- **[ACT]/[DELEGATED] Cross-border transfer (Part VIII) is the substantive gap.** No Commission adequacy determination exists for Zavu's or Meta's jurisdiction, and s.42(6) is explicit that silence never implies adequacy. The workable basis is s.43(1)(b) — necessary for performance of a contract with the data subject — but s.41(2) still creates a **standing documentation duty**: record the transfer basis and adequacy analysis, don't just decide once informally.
- **[PRACTICE] Not yet done — needed before building this**: actually read Zavu's DPA/Terms and Meta's WhatsApp Business data-processing terms against the s.29(1)(a)-(e) processor-obligations checklist. Standard vendor click-through terms usually don't satisfy this out of the box, and neither has been reviewed as part of this assessment.
- **[ACT] Retention**: WhatsApp OTP codes should follow the same short-lived, hashed, expiry-driven pattern `signup_verifications` already uses for email — no new risk there. Open items: Zavu's own message-log retention period (not checked), and justbarme's account-deletion flow (doesn't exist yet) needing to actually erase phone numbers per the s.34(2) standing erasure duty once an account closes.
- **[PRACTICE] DPIA**: not clearly mandated at pilot scale (no sensitive-data categories per the s.65 list, no automated decision-making, no systematic monitoring) but recommended anyway given this introduces a new cross-border sub-processor relationship handling authentication-security data.

**This is advisory output from an in-session skill, not a substitute for Nigerian counsel sign-off before anything here ships to real users.**

## 6. Open decisions before implementation starts

1. Default channel emphasis at signup (equal-weight choice vs. WhatsApp-first) — leaning WhatsApp-first per §1, not yet confirmed.
2. Whether to implement the operational-update notification-preferences model (N.4) now or defer it until the sale/inventory modules it depends on exist.
3. Zavu's and Meta's own DPA/Terms need an actual read-through against §5's processor-obligations checklist before any contract is signed.
4. The staff-invitation lawful-basis question (§5) needs a real answer from counsel, not just this document's lean.

## 7. What can't be live-verified without real credentials

Same situation as Google OAuth: this needs a real Meta Business Manager, an approved template, and a non-sandbox-restricted number, none of which exist in this dev environment. Whoever configures real Zavu/Meta credentials should manually exercise OTP-via-WhatsApp, the email-to-WhatsApp resend path, and (once built) the operational-update and invitation flows at least once before trusting any of this in production.
