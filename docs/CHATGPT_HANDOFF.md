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
| #13 | `experiment/fixed-opus-5` | direct `claude-opus-5` control | Claude Code, while backend response still reported `claude-haiku-4.5` |
| #14 | `experiment/log-requested-sent-served` | instrumentation only: log incoming model and model actually sent; keep response model logging | live log showed `incoming=claude-opus-5 sent=claude-opus-5`, followed ~0.5s later by backend-reported `claude-haiku-4.5`; however a concurrent Haiku request was also in flight, so the response line is not yet uniquely paired |
| #18 | `experiment/correlate-model-route` | instrumentation only: add the same `[session:request]` key to sent-model and response-model logs | live log paired Haiku request `[9361248f:000003]` with backend-reported `claude-haiku-4.5`; concurrent Opus request `[6bb8d99d:000004]` was sent as `claude-opus-5`, but no response-model line for that key appeared in the captured output. Therefore the prior ~0.5s Haiku response line did not belong to the Opus request |

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
7. The concurrent Opus request `[6bb8d99d:000004]` was sent as `claude-opus-5`, but the captured output contained no `Backend response model(s)` line with that key. The backend-reported model for that exact Opus request therefore remains unresolved.

The current unresolved question is now narrower: what model ID, if any, does the backend report for the exact request that the proxy sent as `claude-opus-5`?

PR #18 (`experiment/correlate-model-route`) has ruled out the earlier false temporal pairing with the concurrent Haiku request. The next step is to determine whether the Opus-correlated response arrives later, omits `assistantResponseEvent.modelId`, or terminates through an error/cancellation path.

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
