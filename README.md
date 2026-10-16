# HitPipe
A centralized webhook handler

## Project OS — agent onboarding

Codex, Gemini CLI, Hermes and Antigravity load the short workspace adapters (`AGENTS.md`, `GEMINI.md`, `.hermes.md` and `.agents/rules/project-os.md`). They point to the **single canonical** [Agent Master Prompt](https://github.com/hitpipe/project-os/blob/main/AGENT_MASTER_PROMPT.md) in [`hitpipe/project-os`](https://github.com/hitpipe/project-os). A link is not loaded instructions: the agent must actually read the file.

For a Task, send `Project OS /start HIT-<number>`. The agent must obtain an official [Task bootstrap](https://github.com/hitpipe/project-os/blob/main/onboarding/README.md) via an **authorized** integration/operator export, then follow its exact pinned protocol. Adapters do not grant Project OS credentials, participant access, Start approval or Review authority. Without a valid bootstrap or context, execution is blocked; an offline draft is not execution.

For ordinary ChatGPT/Gemini chats, follow the [manual chat template](https://github.com/hitpipe/project-os/blob/main/onboarding/templates/chat.md), providing the full Master Prompt text and any authorized Task materials. Product coding conventions remain local to this repository.
