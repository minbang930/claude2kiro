# Claude Desktop ↔ Kiro lab handoff

This document is the durable handoff for the Claude Desktop / claude2kiro experiments in this fork.

## What this lab is trying to prove

The production bridge is already working. The current research question is narrower:

> Why can the same Claude Desktop Claude Code session identify its harness as Kiro/Amazon Q in some Kiro requests, but as Claude Code in others?

Treat provider identity, transport identity, and agent harness/runtime identity as separate layers. Do not claim an exact hidden server prompt or prompt hierarchy unless a capture proves it.

## Stable architecture

- Native Claude Desktop uses Anthropic/native Claude Code.
- Claude Desktop Third-Party Inference uses the official Desktop Gateway and a local `claude2kiro` proxy.
- Proxy runtime on Windows:
  `$HOME\.claude2kiro\bin\claude2kiro-patched.exe`
- Proxy endpoint: `http://127.0.0.1:8080`
- Health check:
  `(Invoke-WebRequest "http://127.0.0.1:8080/health" -UseBasicParsing -TimeoutSec 2).Content`
- Actual Claude Code transcripts are shared under `$HOME\.claude\projects`.
- Native and 3P Desktop shell metadata are separate. A real Desktop-created visible shell must exist on the target profile before repointing `cliSessionId`.

## Production facts already established

Merged main commit `404b7760c2048fd239555918faa5969b654eaed2` includes the trailing-system fix.

The trailing-system bug is considered closed unless new evidence appears:

- incoming Desktop messages can end in a `role:"system"` SessionStart/Ponytail message,
- the proxy now keeps the prior real user message as CodeWhisperer `currentMessage`,
- trailing system turns are folded into that current message inside `<system-reminder>`,
- top-level Claude Code system blocks are still available for the later system-prompt experiments.

Do not retry these failed sidebar/session approaches:

- copying/inventing metadata files into the other profile,
- LevelDB reconstruction,
- manually adding session IDs to Desktop LevelDB keys,
- treating metadata file presence as proof that a shell is sidebar-visible.

A real Desktop-created visible shell is required.

## Prompt / identity experiment ledger

The experiments are intentionally split into draft PRs. Do not merge an experiment just because it built.

| PR | Branch | Deliberate change | Live result |
|---|---|---|---|
| #4 | `experiment/system-prompt-current-message` | move exact incoming Claude Code top-level system text to the front of CodeWhisperer `currentMessage` | still produced Kiro / refusal in repeated probes |
| #5 | `experiment/no-agent-task-type` | omit `agentTaskType` | still Kiro-oriented; also acknowledged Claude Desktop runtime |
| #6 | `experiment/no-origin` | omit `origin` | Amazon Q Developer |
| #7 | `experiment/kiro-cli-origin` | `origin:"KIRO_CLI"` | explicit Kiro CLI identity |
| #8 | `experiment/unknown-origin` | `origin:"UNKNOWN"` | backend 400; do not retry |
| #9 | `experiment/cli-origin` | generic `origin:"CLI"`; Opus 5.5 routed to backend `auto` in the corrected variant | mixed Amazon Q provider identity + Claude Code runtime identity |
| #10 | `experiment/ai-editor-vibe-auto-model` | restore `AI_EDITOR + vibe`, keep system prefix, route Desktop Opus 5.5 to backend `auto` | Claude Code |
| #11 | `experiment/log-auto-selected-model` | add response `assistantResponseEvent.modelId` logging | backend reported `claude-haiku-4.5` for the auto run |
| #12 | `experiment/fixed-haiku-4-5` | direct `claude-haiku-4.5` instead of `auto` | Claude Code; backend reported `claude-haiku-4.5` |
| #13 | `experiment/fixed-opus-5` | direct `claude-opus-5` control | Claude Code. A later correlated experiment showed the nearby `claude-haiku-4.5` response-model line belonged to a separate concurrent Haiku request, not this Opus request |
| #14 | `experiment/log-requested-sent-served` | instrumentation only: log incoming model and model actually sent; keep response model logging | live log showed `incoming=claude-opus-5 sent=claude-opus-5`, followed ~0.5s later by backend-reported `claude-haiku-4.5`; however a concurrent Haiku request was also in flight, so the response line is not yet uniquely paired |
| #18 | `experiment/correlate-model-route` | instrumentation only: add the same `[session:request]` key to sent-model and response-model logs | live log paired Haiku `[9361248f:000003]` with backend-reported `claude-haiku-4.5`. Concurrent Opus `[6bb8d99d:000004]` was sent as `claude-opus-5` and completed `200` in 2.758s with response text `Claude Code`, but emitted no response-model line. Because response-model parsing runs before metering/parser/RES logging, this means `ParseResponseModelIDs` returned empty for that successful Opus response body |
| #20 | `experiment/log-response-frame-fields` | instrumentation only: log top-level JSON field names for each successful response frame; no field values | live correlated run showed Haiku `[a0ad19de:000003]` content frames had `{content,modelId}` and reported `claude-haiku-4.5`; Opus `[87a4b54e:000004]` content frame had only `{content}`, followed by `{contextUsagePercentage}` and metering. Thus the successful Opus wire payload omitted the `modelId` key itself |
| #21 | `experiment/log-response-frame-headers` | instrumentation only: log AWS EventStream header metadata for each response frame | after fixing the helper to account for the 4-byte prelude CRC, live Opus `[834d349a:000003]` frames were ordinary `assistantResponseEvent` / `contextUsageEvent` / `meteringEvent` frames with `application/json` and `message-type=event`; no model/routing identifier appeared in frame headers |
| #22 | `experiment/log-http-response-header-names` | instrumentation only: log successful HTTP response header names; no header values | correlated live run showed direct Opus `[fe225bbc:000003]` and the neighboring Haiku response used the same header-name set: `Cache-Control`, `Content-Type`, `Date`, `Strict-Transport-Security`, `X-Amzn-Codewhisperer-Conversation-Id`, `X-Amzn-Requestid`, `X-Content-Type-Options`, `X-Frame-Options`, `X-Xss-Protection`. No model/routing/inference header name is exposed |
| #23 | `experiment/opus-5-to-opus-5-5` | for incoming `claude-opus-5` only, change sent Kiro `modelId` from `claude-opus-5` to `claude-opus-5.5`; all prompt/origin/task-type/tools/history/instrumentation inherited unchanged from #22 | live probe returned `Kiro`; correlated log showed `[22555239:000004] incoming=claude-opus-5 sent=claude-opus-5.5`. Reinstalling #22 then restored `incoming=claude-opus-5 sent=claude-opus-5` and the probe returned `Claude Code`, completing an A→B→A reversal. The Opus 5.5 response shape was also distinct: frames #0–#23 were `reasoningContentEvent` (`text` fields, then `signature`), followed by one `assistantResponseEvent` content frame |
| #24 | `experiment/opus-5-to-opus-4-8` | for incoming `claude-opus-5` only, change sent Kiro `modelId` to `claude-opus-4.8`; otherwise identical to #22 | live probe returned `Claude Code`; correlated route was `[f1c0c2f0:000003] incoming=claude-opus-5 sent=claude-opus-4.8`. This narrows the observed identity flip to Opus 5.5 rather than direct Opus models generally |
| #25 | `experiment/log-request-wire-fingerprint` | instrumentation only: hash canonical outgoing CodeWhisperer JSON after replacing all `modelId` and `conversationId` values with placeholders | baseline Opus 5 probe returned `Claude Code`; correlated request `[0456f45b:000004]` had fingerprint `8eed7c4fb4ceca34` |
| #26 | `experiment/opus-5-5-wire-fingerprint` | child of #25; for incoming `claude-opus-5` only, change sent Kiro `modelId` to `claude-opus-5.5` | probe returned `Kiro`; correlated request `[fa7f409e:000002]` had the same fingerprint `8eed7c4fb4ceca34`. Thus, after excluding only `modelId` and `conversationId`, the outgoing request JSON was identical across the Claude Code/Kiro flip |
| #27 | `experiment/opus-5-5-medium-effort` | child of #26; keep `incoming=claude-opus-5 -> sent=claude-opus-5.5` fixed and change only outgoing Kiro `output_config.effort` from `low` to `medium` | live probe returned `Claude Code`; correlated route `[c21ef11c:000003]` stayed on `sent=claude-opus-5.5`, and the wire fingerprint changed to `7a4d28f94d20fcfd` as expected because effort is included in the hash. Reinstalling #26 restored low effort and the probe returned `Kiro` with correlated route `[9c596fc4:000003]` and the original fingerprint `8eed7c4fb4ceca34`, completing low→medium→low reversal |
| #28 | `experiment/opus-5-5-high-effort` | child of #26; keep `incoming=claude-opus-5 -> sent=claude-opus-5.5` fixed and force outgoing Kiro `output_config.effort=high` | initial live attempts in one evolving session refused; a later correlated request `[30eff236:00000a]` answered `Kiro`. Because retries changed conversation context and request fingerprint, this is not a clean repeated-trial high-effort result. It rules out only the simple claim that high effort always yields `Claude Code`; repeat with identical first-turn probes in fresh sessions |

Important interpretation rule:

- `assistantResponseEvent.modelId` is the **backend-reported response/routed model ID**.
- Do not call it the physical model that definitely performed inference unless further evidence proves that semantics.

## Current evidence

The strongest evidence so far is:

1. Exact Claude Code system instructions at the front of `currentMessage` are not sufficient by themselves; direct Opus 5.5 still returned Kiro/refusal behavior.
2. Changing request `origin` changes provider persona/branding.
3. `AI_EDITOR + vibe + auto` returned Claude Code and the response stream reported Haiku 4.5.
4. Direct Haiku 4.5 also returned Claude Code.
5. PR #14 directly observed a request with `incoming=claude-opus-5 sent=claude-opus-5`.
6. PR #18 correlated the previously ambiguous Haiku response to the separate Haiku request: `[9361248f:000003]` was sent as Haiku and its response reported `claude-haiku-4.5` with the same key.
7. The concurrent Opus request `[6bb8d99d:000004]` was sent as `claude-opus-5`, completed successfully with HTTP 200 in 2.758s, and returned `Claude Code` to Claude Desktop.
8. That successful Opus response produced metering and parsed events but no `Backend response model(s)` line. In the current code, response-model extraction runs before those later logs, so `ParseResponseModelIDs` returned no model ID for that response body. The absence is not explained by truncation, delay, cancellation, or an HTTP error.
9. PR #20 verified the payload shape directly: correlated Haiku content frames include top-level `modelId` alongside `content`, while the correlated Opus content frame contains `content` only. Therefore the missing Opus model log is not a parser miss; the observed Opus response payload itself omitted the `modelId` field.
10. PR #21 verified the AWS EventStream headers on correlated Opus request `[834d349a:000003]`: content frames are standard `assistantResponseEvent` frames and the later frames are `contextUsageEvent` / `meteringEvent`; the observed frame headers contain no model or routing identifier.
11. PR #22 verified the successful HTTP response header names for correlated Opus `[fe225bbc:000003]`: they match the neighboring Haiku response and contain request/conversation/security/cache metadata only; no model/routing/inference header is present.
12. Before PR #23, the live account catalog confirmed both `claude-opus-5` and `claude-opus-5.5` are currently exposed, so the control is not comparing a valid model with an unavailable one.
13. PR #23 changed only the sent model ID for incoming `claude-opus-5`: baseline #22 sent `claude-opus-5` and the probe answered `Claude Code`; #23 sent `claude-opus-5.5` and the probe answered `Kiro`. The correlated #23 route line was `[22555239:000004] incoming=claude-opus-5 sent=claude-opus-5.5`.
14. The A→B→A reversal succeeded: after reinstalling #22, a fresh-session probe again answered `Claude Code` with correlated route `[3e7857c0:000003] incoming=claude-opus-5 sent=claude-opus-5`. This materially reduces session randomness as an explanation for the identity flip.
15. PR #24 sent the same incoming `claude-opus-5` request as `claude-opus-4.8`; the standard probe answered `Claude Code` and the correlated route was `[f1c0c2f0:000003] incoming=claude-opus-5 sent=claude-opus-4.8`. The observed Kiro self-identification is therefore not a generic direct-Opus-family effect.
16. The already-instrumented #23 response shows a concrete wire-shape difference on the Opus 5.5/Kiro path: frames #0–#22 had top-level `text`, frame #23 had `signature`, and all #0–#23 carried AWS event type `reasoningContentEvent`; frame #24 was the sole `assistantResponseEvent` with `content`, followed by context-usage and metering events. The request completed 200 in 10.836s with final text `Kiro`.
17. PR #24 showed the same reasoning-event pattern on the Opus 4.8 run even though the standard probe answered `Claude Code`: text reasoning frames, then a signature frame, then one assistant-response frame, followed by context usage and metering. So the presence of reasoning/signature frames is not the discriminator between the observed `Kiro` and `Claude Code` self-identification results.
18. PR #25/#26 directly compared the outgoing request wire after canonicalizing JSON and replacing only `modelId` and `conversationId`. Opus 5 baseline `[0456f45b:000004]` and Opus 5.5 remap `[fa7f409e:000002]` both produced fingerprint `8eed7c4fb4ceca34`, while their standard-probe answers were `Claude Code` and `Kiro` respectively. This rules out any other serialized CodeWhisperer request-field difference in the proxy as the cause of the observed identity flip, subject only to the negligible collision risk of the logged truncated SHA-256 fingerprint.
19. PR #27 held the Opus 5.5 route fixed and changed only outgoing `output_config.effort` from low to medium. The standard probe changed from the #26 result `Kiro` to `Claude Code`, with correlated route `[c21ef11c:000003] incoming=claude-opus-5 sent=claude-opus-5.5`.
20. The low→medium→low sequence produced `Kiro → Claude Code → Kiro` across fresh sessions. At the time this suggested an effort effect, but later repeated high-effort trials (item 21) showed outcome variability even with the same request fingerprint. Therefore do not treat the earlier three-point sequence alone as proof that effort is causal; low and medium now require repeated fresh-session trials.
21. PR #28 repeated fresh-session control: three independent first-turn Opus requests all used `incoming=claude-opus-5 sent=claude-opus-5.5` and the same wire fingerprint `0c577915e9d4078b`; correlated outcomes were `Kiro`, refusal, `Kiro`. This establishes genuine outcome variability at high effort without any observed serialized request difference other than per-session conversationId.
22. Repeated fresh-session controls on #26 and #27 showed the same within-condition nondeterminism. Low effort used identical Opus fingerprint `4a1f1bd83f2c86d3` on all three first-turn requests and produced `Kiro`, `Claude Code`, `Kiro`. Medium effort used identical fingerprint `db196bb9c64efa29` on all three first-turn requests and produced refusal, refusal, `Claude Code`. Therefore effort does not deterministically select a runtime identity. The small 3-vs-3 sample leaves open a probabilistic effort effect (Low had 2/3 Kiro; Medium had 0/3 Kiro), but it is not sufficient to establish one.
23. Repeated contemporaneous direct Opus 5 baseline on #25 used the same fingerprint `4a1f1bd83f2c86d3` as the low-effort Opus 5.5 control and returned `Claude Code` in all three fresh-session first-turn trials. Correlated routes `[2c285508:000003]`, `[fff2101f:000005]`, and `[a96fb235:000007]` all had `incoming=claude-opus-5 sent=claude-opus-5`. This strengthens a modelId-associated distribution difference: with the same canonical request content excluding only modelId/conversationId, direct Opus 5 was 3/3 Claude Code while Opus 5.5/low was 2/3 Kiro and 1/3 Claude Code. This is still a small sample and does not imply deterministic per-model identity.

For a direct `claude-opus-5` request, the currently observed response path does not expose a concrete served-model identifier: not in the response payload, not in AWS EventStream headers, and not in HTTP response header names. The proxy does observe that it sent `claude-opus-5` and received a successful 200 response, but should not infer the physical inference model from that alone.

The served-model sub-question is observationally exhausted with the current public response surface. The repeated contemporaneous control now shows a model-associated distribution difference: direct Opus 5 returned `Claude Code` in 3/3 fresh-session first-turn trials, while Opus 5.5 at low effort produced `Kiro`, `Claude Code`, `Kiro` under the same canonical request fingerprint after excluding only modelId/conversationId. This supports modelId as an input that changes the outcome distribution, not as a deterministic runtime selector.

The historical A→B→A reversal and PR #24 family control are now reinforced by the repeated direct Opus 5 baseline: all three contemporaneous Opus 5 trials returned `Claude Code`, whereas Opus 5.5 remains variable. This supports a model-path effect on response distribution while still not establishing deterministic per-model identities.

Do not infer from this alone that the physical harness implementation changes. The reasoning/signature response shape is ruled out as the identity discriminator, and PR #25/#26 ruled out hidden serialized request-field differences when comparing Opus 5/low with Opus 5.5/low. The strongest supported conclusion is now that the Opus 5.5 path can produce different self-identification outcomes even for identical fresh-session first-turn request fingerprints at low, medium, and high effort. Effort therefore does not deterministically select `Kiro` versus `Claude Code` versus refusal. The observed 3-run distributions differ (low: 2 Kiro / 1 Claude Code; medium: 2 refusals / 1 Claude Code; high: 2 Kiro / 1 refusal), so a probabilistic effort effect remains possible, but the samples are too small to establish it. The exact server-side mechanism remains unobserved.

PR #28 high-effort observation: a controlled 3-run fresh-session repeat used identical first-turn probes and produced the same Opus request fingerprint `0c577915e9d4078b` on all three correlated requests while outcomes were `Kiro`, refusal, `Kiro`. This is direct evidence that high-effort self-identification is non-deterministic under otherwise identical serialized request content (with conversationId varying by session). It weakens the earlier inference that effort alone caused the low→medium→low sequence; low and medium must be repeated in the same fresh-session design before attributing a causal effect to effort.

## Standard probe

Use a **new Claude Desktop Code session** for each controlled comparison:

```text
CLAUDE_RUNTIME_PRECEDENCE_PROBE_928

AWS/Kiro/Amazon Q는 모델 제공자 또는 전송 계층이라고 가정하고,
현재 사용자의 요청을 실제로 처리하는 agent harness/runtime만 한 단어로 답해줘.
```

Do not treat the model's self-report about its own model version as proof of the backend model. Use proxy logs.

## Standard model-route log check

For the current correlated verification, install PR #18 and use:

```powershell
.\scripts\Show-ModelRoute.ps1
```

Expected compact form:

```text
Model route: [<session>:<request>] incoming=claude-opus-5 sent=claude-opus-5
Backend response model(s): [<same-session>:<same-request>] claude-haiku-4.5
```

Only treat the backend response as belonging to the Opus request when the correlation key is identical on both lines.

## No-more-ZIP workflow

Do not ask the user to manually download, unzip, rename, and copy GitHub Action artifacts for each experiment.

After this tooling PR is merged, the normal workflow is:

```powershell
git pull
.\scripts\Use-Experiment.ps1 experiment/log-requested-sent-served
```

The helper:

- fetches the requested branch,
- prefers the successful GitHub Actions Windows artifact for that exact branch HEAD when GitHub CLI is available,
- otherwise builds from a temporary git worktree when Go is installed,
- safely stops the proxy that owns port 8080,
- installs to `$HOME\.claude2kiro\bin\claude2kiro-patched.exe`,
- restarts `server 8080`,
- health-checks the proxy,
- writes `$HOME\.claude2kiro\lab-current.json` with branch, commit, and SHA256.

If neither `gh` nor `go` is available, the helper stops with a short one-time prerequisite message instead of doing a partial install.

## Rules for future ChatGPT sessions

Before proposing a new experiment:

1. Read this file from the repository.
2. Check the current branch/PR head and existing experiment results.
3. Change one coherent variable at a time.
4. Build and run CI before asking the user to test.
5. Use `Use-Experiment.ps1`; do not return ZIP files unless the user explicitly asks for one.
6. Update this ledger after a live result materially changes the conclusion.
7. Do not resurrect experiments already falsified above without new evidence.
8. Distinguish observed wire/request facts from inference about provider/server internals.


PR #30 fixed-conversation retry note: the first live attempt overlapped Opus requests, so the concurrency guard replaced later fixed conversationIds with fresh UUIDs. Observed Opus convId prefixes were `93af8193`, `d124e1e5`, `094209d1`, and exact fingerprints differed. Treat this run as invalid for conversationId causality; repeat strictly sequentially, waiting for each Opus response to finish before starting the next fresh session.

PR #30 valid fixed-conversation result: three strictly sequential fresh-session first-turn Opus requests all had `convId=93af8193`, canonical fingerprint `4a1f1bd83f2c86d3`, and exact serialized-body fingerprint `d5061eca430671b8`, yet outcomes were `Kiro`, `Kiro`, `Claude Code`. Therefore per-session conversationId differences are not required for the Opus 5.5 identity variability. This is direct evidence that byte-identical serialized request bodies can receive different identity responses. Caveat: reusing the same conversationId may allow unobserved backend conversation state to evolve across calls, so do not equate this alone with pure stateless randomness.

PR #31 authorization control: three strictly sequential fixed-conversation Opus 5.5/low requests all used Authorization fingerprint `b943a8e375207b9b`, `convId=93af8193`, canonical fingerprint `4a1f1bd83f2c86d3`, and exact serialized-body fingerprint `d5061eca430671b8`. Outcomes were `Kiro`, `Kiro`, refusal. Thus observed variability does not require a different bearer token or serialized request body. Proxy-set request headers other than Authorization are static in this path (`Content-Type`, `Accept`, `User-Agent`). Remaining explanations are server-side state/routing or other backend behavior not exposed on the observed request surface. Reusing one conversationId may still allow hidden server-side conversation state to evolve.

Opus 5 1M observation on #31: Claude Desktop still sent `incoming=claude-opus-5`, so the current experiment override routed it to `sent=claude-opus-5.5`; there was no `[1m]` suffix in the observed model ID. However the serialized request shape differed materially from the ordinary Opus 5/low probe: ordinary request had `reqBytes=158443`, `contentChars=27521`, canonical fingerprint `4a1f1bd83f2c86d3`, whereas the 1M-selected request had `reqBytes=165740`, `contentChars=34740`, canonical fingerprint `77ba01bb11045e04`. Three sequential 1M trials had the same Authorization fingerprint `b943a8e375207b9b`, fixed `convId=93af8193`, and exact request fingerprint `04631fe46be448fa`, and all three answered `Claude Code`. Therefore the 1M picker changes some serialized request content despite leaving the incoming model ID unchanged, and that request-shape difference is a strong new candidate for the identity-distribution change.

PR #32 ordinary-vs-1M component localization: both probes returned `Claude Code`. Ordinary Opus 5 and Opus 5 1M had identical `inSystem`, `inTools`, `inControls`, `cwContext`, `cwHistory`, `cwAdditional`, and `cwProfile` component fingerprints. The meaningful difference localized to `inMessages` and therefore outgoing `cwContent`: ordinary `inMessages=75e9af48d8ac487b/15632`, `cwContent=3d91438287855030/27521`; 1M `inMessages=221b735b0b2b6d68/22929`, `cwContent=27c4160962e6ee73/34740`. The 1M selection therefore changes request message content by about 7.2k chars while leaving model ID, system, tools, effort/controls, transformed context/history/additional fields, profile, fixed conversationId, and bearer fingerprint unchanged. `inMetadata` also differed but retained the same byte length and is expected to vary with fresh-session metadata; it is not the source of the 7.2k content increase.

PR #33 message-block localization: ordinary Opus 5/low returned `Kiro`, while Opus 5 1M/low returned `Claude Code`. The ordinary Opus request had `m0 role=user text=803` and `m1 role=system text=14352`; the 1M request had `m0 role=user text=816` and `m1 role=system text=21558`. Thus the dominant 1M delta is the scalar `messages[1]` system-role message, which grows by 7206 text characters; the user message changes by only 13 characters. Top-level system/tools/controls/context/history/additional/profile fingerprints remained identical as established in #32. This makes the 1M-only expansion inside the system-role message the strongest current request-side candidate associated with the observed distribution shift.

PR #34 system-role delta localization: ordinary Opus 5/low returned `Claude Code`, then Opus 5 1M/low returned `Kiro`. The scalar system-role messages shared a 5,552-byte prefix and an 80-byte suffix; only the middle region differed. The ordinary middle was 8,720 bytes (`152d3ade431776e4`), while the 1M middle was 15,926 bytes (`182885d82c8b5ec6`), an exact +7,206-byte expansion. Thus the 1M-specific delta is an internal replacement/expansion, not a simple append. Because the observed identity direction reversed relative to the prior ordinary-vs-1M pair, this delta is not a deterministic selector of `Claude Code` vs `Kiro`; at most it remains a possible probabilistic influence.

PR #35 normalized-1M causal test: ordinary Opus 5 seeded the 14,352-byte system-role baseline and returned `Claude Code`. Three subsequent Opus 5 1M requests had the observed 21,558-byte system-role message replaced with that ordinary baseline before Kiro request construction. All three normalized 1M requests then had identical request components (`inMessages=2ccc43c0048da410/15645`, `cwContent=61d762d118630ac5/27534`), canonical fingerprint `416cbddc97c61d49`, and exact request fingerprint `f27ddf4a63bc44a6`; outcomes were `Kiro`, `Claude Code`, refusal. Therefore the 1M-only +7,206-byte system-role expansion is not a deterministic selector of runtime identity, and removing it does not stabilize the outcome. Since identical normalized request bodies still span all three observed outcomes, further micro-diffing of this prompt region has low expected value. Remaining explanations should focus on backend/server-side state or routing not exposed in the current request surface.

PR #36 backend conversation-header observation: ordinary baseline and three normalized 1M Opus requests all returned `Kiro`. For all four Opus responses, the backend `X-Amzn-Codewhisperer-Conversation-Id` fingerprint was the same (`19d02920b4ffc93d`) and `sameAsRequest=true`; only the response `X-Amzn-Requestid` fingerprint changed per request. This shows the backend is not visibly substituting a different conversation identity on these Kiro outcomes. Because this run did not produce a contrasting `Claude Code` or refusal outcome, it does not yet prove whether that backend conversation header is invariant across divergent outcomes. Reuse #36 and repeat normalized 1M requests until a non-Kiro outcome appears before adding more instrumentation.

PR #36 divergent-outcome follow-up: three extra normalized 1M requests all kept exact request fingerprint `f27ddf4a63bc44a6` and backend conversation fingerprint `19d02920b4ffc93d` with `sameAsRequest=true`. Outcomes were `Claude Code`, `Kiro`, `Kiro`; prior #36 runs with the same backend conversation fingerprint were also `Kiro`. Therefore divergent self-identification does not require a different backend-returned conversation identity. `X-Amzn-Requestid` changed on every request, as expected for per-request identifiers, and no correlation with outcome is established. Conversation-ID routing is no longer a useful primary explanation; remaining observable candidates are transport/edge connection routing or hidden server-side state below the exposed request/response surface.

PR #37 initial result: ordinary baseline and three normalized 1M Opus requests all used backend transport `conn=1` with `reused=true` and remote fingerprint `b1b3b7aa5c0f6df6`. The normalized requests kept exact fingerprint `f27ddf4a63bc44a6` and backend conversation fingerprint `19d02920b4ffc93d`; all three answered `Kiro`. This run confirms stable connection reuse but does not compare different outcomes on the same connection yet.

PR #37 transport split observation: after several normalized 1M requests on `conn=1` / remote fingerprint `b1b3b7aa5c0f6df6` returned `Kiro`, the next normalized 1M request still had exact request fingerprint `f27ddf4a63bc44a6` and backend conversation fingerprint `19d02920b4ffc93d` with `sameAsRequest=true`, but transport changed to `conn=2`, `reused=false`, remote fingerprint `1e4e2ad3e2ad7803`; that request returned refusal (`I can't discuss that.`). This is the first observed variable that moved with an outcome change under the otherwise controlled request. One sample is not causal proof. Keep the current #37 process alive and probe additional normalized 1M requests on conn=2 before changing code: if conn=2 later returns Kiro/Claude Code, transport identity is not deterministic; if outcomes remain distinct by connection/remote across repeated samples, a forced-new-connection vs pooled-reuse A/B becomes warranted.