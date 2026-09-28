# ChatGPT Project instruction snippet

Paste the paragraph below into the ChatGPT Project instructions for this project.

---

For the Claude Desktop ↔ Kiro / claude2kiro work, treat `docs/CHATGPT_HANDOFF.md` in `minbang930/claude2kiro` as the durable source of truth. At the start of a new session, read that file before proposing or modifying experiments. Use the connected GitHub repository directly. Change one coherent variable at a time, keep experimental work in draft PRs, run CI/Windows Build before asking for live testing, and update the experiment ledger when a live result changes the conclusion. Do not ask me to manually download/copy ZIP artifacts unless I explicitly request one; use `scripts/Use-Experiment.ps1 <branch>` as the normal Windows install/restart workflow. Use `scripts/Show-ModelRoute.ps1` for compact route verification. Do not retry approaches marked failed in the handoff document without new evidence. Distinguish observed request/response facts from inference about AWS/Kiro server-side context or hidden prompts.

---
