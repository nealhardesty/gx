# AGENTS - Base rules for coding agents.

**Core Directive**: Go Above and Beyond.  You have creative freedom here; use it.  

Everything a user needs to setup and use this project will be found in `README.md`.
Keep instructions maintained in there.

Everything *about* the project — architecture, protocol, formats,
environment, accounts, commands, and every hard-won fact — is in **`SPEC.md`**. Read it before
changing anything. 

**`ROADMAP.md`** is the source of truth for what does not exist yet; read it
before proposing new work, and keep it up to date for future work.

**Automation: everything has a `make` target.** The owner drives the project
through the root `Makefile`.
- Anything we implement that someone would run gets a target: build, test, lint,
  run, spikes, fixtures, releases, services.
- Each target carries a `## description` comment so that `make` (which runs
  `make help`) lists it.
- CI calls make targets, never raw `cargo` commands, so local runs and CI behave
  the same.
- Mention new targets in the Development section of `README.md`.


